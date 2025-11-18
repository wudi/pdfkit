package writer

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"crypto/rand"
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/hex"
	"fmt"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type impl struct{ interceptors []Interceptor }

func (w *impl) SerializeObject(ref raw.ObjectRef, obj raw.Object) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%d %d obj\n", ref.Num, ref.Gen))
	switch o := obj.(type) {
	case *raw.DictObj:
		buf.WriteString("<<")
		keys := make([]string, 0, len(o.KV))
		for k := range o.KV {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("/" + k + " ")
			buf.Write(serializePrimitive(o.KV[k]))
		}
		buf.WriteString(">>\n")
	case *raw.ArrayObj, raw.NameObj, raw.NumberObj, raw.BoolObj, raw.NullObj, raw.StringObj, raw.HexStringObj, *raw.StreamObj, raw.RefObj:
		buf.Write(serializePrimitive(o))
		buf.WriteString("\n")
	default:
		buf.WriteString("null\n")
	}
	buf.WriteString("endobj\n")
	return buf.Bytes(), nil
}

func (w *impl) Write(ctx Context, doc *semantic.Document, out WriterAt, cfg Config) error {
	if doc.Encrypted && !(doc.Permissions.Modify || doc.Permissions.Assemble) {
		return fmt.Errorf("document permissions forbid modification")
	}
	version := pdfVersion(cfg)
	incr := incrementalContext(doc, out, cfg)
	// Build raw objects from semantic (minimal subset: catalog, pages, page, fonts, content streams)
	objects := make(map[raw.ObjectRef]raw.Object)
	objNum := incr.startObjNum
	nextRef := func() raw.ObjectRef {
		ref := raw.ObjectRef{Num: objNum, Gen: 0}
		objNum++
		return ref
	}
	catalogRef := nextRef()
	pagesRef := nextRef()
	pageRefs := make([]raw.ObjectRef, 0, len(doc.Pages))

	// Fonts (shared across pages by BaseFont)
	fontRefs := map[string]raw.ObjectRef{}
	xobjectRefs := map[string]raw.ObjectRef{}
	patternRefs := map[string]raw.ObjectRef{}
	shadingRefs := map[string]raw.ObjectRef{}
	ensureFont := func(base string) raw.ObjectRef {
		if base == "" {
			base = "Helvetica"
		}
		if ref, ok := fontRefs[base]; ok {
			return ref
		}
		ref := nextRef()
		fontDict := raw.Dict()
		fontDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Font"))
		fontDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Type1"))
		fontDict.Set(raw.NameLiteral("BaseFont"), raw.NameLiteral(base))
		objects[ref] = fontDict
		fontRefs[base] = ref
		return ref
	}
	ensureXObject := func(name string, xo semantic.XObject) raw.ObjectRef {
		key := xoKey(name, xo)
		if ref, ok := xobjectRefs[key]; ok {
			return ref
		}
		ref := nextRef()
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("XObject"))
		sub := xo.Subtype
		if sub == "" {
			sub = "Image"
		}
		dict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(sub))
		if sub == "Image" {
			if xo.Width > 0 {
				dict.Set(raw.NameLiteral("Width"), raw.NumberInt(int64(xo.Width)))
			}
			if xo.Height > 0 {
				dict.Set(raw.NameLiteral("Height"), raw.NumberInt(int64(xo.Height)))
			}
			color := xo.ColorSpace.Name
			if color == "" {
				color = "DeviceRGB"
			}
			dict.Set(raw.NameLiteral("ColorSpace"), raw.NameLiteral(color))
			if xo.BitsPerComponent > 0 {
				dict.Set(raw.NameLiteral("BitsPerComponent"), raw.NumberInt(int64(xo.BitsPerComponent)))
			}
		}
		if sub == "Form" && cropSet(xo.BBox) {
			dict.Set(raw.NameLiteral("BBox"), rectArray(xo.BBox))
		}
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(xo.Data))))
		objects[ref] = raw.NewStream(dict, xo.Data)
		xobjectRefs[key] = ref
		return ref
	}
	ensurePattern := func(name string, p semantic.Pattern) raw.ObjectRef {
		key := patternKey(name, p)
		if ref, ok := patternRefs[key]; ok {
			return ref
		}
		ref := nextRef()
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pattern"))
		pt := p.PatternType
		if pt == 0 {
			pt = 1
		}
		dict.Set(raw.NameLiteral("PatternType"), raw.NumberInt(int64(pt)))
		paint := p.PaintType
		if paint == 0 {
			paint = 1
		}
		dict.Set(raw.NameLiteral("PaintType"), raw.NumberInt(int64(paint)))
		tiling := p.TilingType
		if tiling == 0 {
			tiling = 1
		}
		dict.Set(raw.NameLiteral("TilingType"), raw.NumberInt(int64(tiling)))
		if cropSet(p.BBox) {
			dict.Set(raw.NameLiteral("BBox"), rectArray(p.BBox))
		}
		if p.XStep > 0 {
			dict.Set(raw.NameLiteral("XStep"), raw.NumberFloat(p.XStep))
		}
		if p.YStep > 0 {
			dict.Set(raw.NameLiteral("YStep"), raw.NumberFloat(p.YStep))
		}
		content := p.Content
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(content))))
		objects[ref] = raw.NewStream(dict, content)
		patternRefs[key] = ref
		return ref
	}
	ensureShading := func(name string, s semantic.Shading) raw.ObjectRef {
		key := shadingKey(name, s)
		if ref, ok := shadingRefs[key]; ok {
			return ref
		}
		ref := nextRef()
		dict := raw.Dict()
		stype := s.ShadingType
		if stype == 0 {
			stype = 2
		}
		dict.Set(raw.NameLiteral("ShadingType"), raw.NumberInt(int64(stype)))
		cs := s.ColorSpace.Name
		if cs == "" {
			cs = "DeviceRGB"
		}
		dict.Set(raw.NameLiteral("ColorSpace"), raw.NameLiteral(cs))
		if len(s.Coords) > 0 {
			arr := raw.NewArray()
			for _, c := range s.Coords {
				arr.Append(raw.NumberFloat(c))
			}
			dict.Set(raw.NameLiteral("Coords"), arr)
		}
		if len(s.Domain) > 0 {
			arr := raw.NewArray()
			for _, d := range s.Domain {
				arr.Append(raw.NumberFloat(d))
			}
			dict.Set(raw.NameLiteral("Domain"), arr)
		}
		content := s.Function
		if len(content) > 0 {
			dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(content))))
			objects[ref] = raw.NewStream(dict, content)
		} else {
			objects[ref] = dict
		}
		shadingRefs[key] = ref
		return ref
	}

	// Document info dictionary
	var infoRef *raw.ObjectRef
	if doc.Info != nil {
		infoDict := raw.Dict()
		if doc.Info.Title != "" {
			infoDict.Set(raw.NameLiteral("Title"), raw.Str([]byte(doc.Info.Title)))
		}
		if doc.Info.Author != "" {
			infoDict.Set(raw.NameLiteral("Author"), raw.Str([]byte(doc.Info.Author)))
		}
		if doc.Info.Subject != "" {
			infoDict.Set(raw.NameLiteral("Subject"), raw.Str([]byte(doc.Info.Subject)))
		}
		if doc.Info.Creator != "" {
			infoDict.Set(raw.NameLiteral("Creator"), raw.Str([]byte(doc.Info.Creator)))
		}
		if doc.Info.Producer != "" {
			infoDict.Set(raw.NameLiteral("Producer"), raw.Str([]byte(doc.Info.Producer)))
		}
		if len(doc.Info.Keywords) > 0 {
			infoDict.Set(raw.NameLiteral("Keywords"), raw.Str([]byte(strings.Join(doc.Info.Keywords, ","))))
		}
		if infoDict.Len() > 0 {
			ref := nextRef()
			infoRef = &ref
			objects[ref] = infoDict
		}
	}

	// XMP metadata stream reference
	var metadataRef *raw.ObjectRef
	if doc.Metadata != nil && len(doc.Metadata.Raw) > 0 {
		ref := nextRef()
		metadataRef = &ref
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Metadata"))
		dict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("XML"))
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(doc.Metadata.Raw))))
		objects[ref] = raw.NewStream(dict, doc.Metadata.Raw)
	}

	// OutputIntents
	var outputIntentRefs []raw.ObjectRef
	for _, oi := range doc.OutputIntents {
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("OutputIntent"))
		if oi.S != "" {
			dict.Set(raw.NameLiteral("S"), raw.NameLiteral(oi.S))
		} else {
			dict.Set(raw.NameLiteral("S"), raw.NameLiteral("GTS_PDFA1"))
		}
		if oi.OutputConditionIdentifier != "" {
			dict.Set(raw.NameLiteral("OutputConditionIdentifier"), raw.Str([]byte(oi.OutputConditionIdentifier)))
		} else {
			dict.Set(raw.NameLiteral("OutputConditionIdentifier"), raw.Str([]byte("Custom")))
		}
		if oi.Info != "" {
			dict.Set(raw.NameLiteral("Info"), raw.Str([]byte(oi.Info)))
		}
		var profileRef *raw.ObjectRef
		if len(oi.DestOutputProfile) > 0 {
			pr := nextRef()
			profileRef = &pr
			pd := raw.Dict()
			pd.Set(raw.NameLiteral("N"), raw.NumberInt(3))
			pd.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(oi.DestOutputProfile))))
			objects[pr] = raw.NewStream(pd, oi.DestOutputProfile)
		}
		if profileRef != nil {
			dict.Set(raw.NameLiteral("DestOutputProfile"), raw.Ref(profileRef.Num, profileRef.Gen))
		}
		ref := nextRef()
		objects[ref] = dict
		outputIntentRefs = append(outputIntentRefs, ref)
	}

	// Page content streams
	contentRefs := []raw.ObjectRef{}
	annotationRefs := make([][]raw.ObjectRef, len(doc.Pages))
	for _, p := range doc.Pages {
		contentData := []byte{}
		for _, cs := range p.Contents {
			contentData = append(contentData, serializeContentStream(cs)...)
		}
		streamData := contentData
		contentRef := nextRef()
		dict := raw.Dict()
		switch filter := pickContentFilter(cfg); filter {
		case FilterFlate:
			data, err := flateEncode(streamData, cfg.Compression)
			if err != nil {
				return err
			}
			streamData = data
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("FlateDecode"))
		case FilterASCIIHex:
			streamData = asciiHexEncode(streamData)
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("ASCIIHexDecode"))
		case FilterASCII85:
			streamData = ascii85Encode(streamData)
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("ASCII85Decode"))
		case FilterRunLength:
			streamData = runLengthEncode(streamData)
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("RunLengthDecode"))
		case FilterLZW:
			data, err := lzwEncode(streamData)
			if err != nil {
				return err
			}
			streamData = data
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("LZWDecode"))
		}
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(streamData))))
		objects[contentRef] = raw.NewStream(dict, streamData)
		contentRefs = append(contentRefs, contentRef)
	}
	// Pages
	unionFonts := raw.Dict()
	unionExtGStates := raw.Dict()
	unionColorSpaces := raw.Dict()
	unionXObjects := raw.Dict()
	unionPatterns := raw.Dict()
	unionShadings := raw.Dict()
	procSet := raw.NewArray(raw.NameLiteral("PDF"), raw.NameLiteral("Text"))
	for i, p := range doc.Pages {
		ref := nextRef()
		pageRefs = append(pageRefs, ref)
		p.MediaBox = semantic.Rectangle{0, 0, p.MediaBox.URX, p.MediaBox.URY}
		pageDict := raw.Dict()
		pageDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
		pageDict.Set(raw.NameLiteral("Parent"), raw.Ref(pagesRef.Num, pagesRef.Gen))
		// MediaBox
		pageDict.Set(raw.NameLiteral("MediaBox"), rectArray(p.MediaBox))
		if cropSet(p.CropBox) {
			pageDict.Set(raw.NameLiteral("CropBox"), rectArray(p.CropBox))
		}
		if cropSet(p.TrimBox) {
			pageDict.Set(raw.NameLiteral("TrimBox"), rectArray(p.TrimBox))
		}
		if cropSet(p.BleedBox) {
			pageDict.Set(raw.NameLiteral("BleedBox"), rectArray(p.BleedBox))
		}
		if cropSet(p.ArtBox) {
			pageDict.Set(raw.NameLiteral("ArtBox"), rectArray(p.ArtBox))
		}
		if rot := normalizeRotation(p.Rotate); rot != 0 {
			pageDict.Set(raw.NameLiteral("Rotate"), raw.NumberInt(int64(rot)))
		}
		if p.UserUnit > 0 {
			pageDict.Set(raw.NameLiteral("UserUnit"), raw.NumberFloat(p.UserUnit))
		}
		// Resources
		resDict := raw.Dict()
		fontResDict := raw.Dict()
		if p.Resources != nil && len(p.Resources.Fonts) > 0 {
			for name, font := range p.Resources.Fonts {
				fRef := ensureFont(font.BaseFont)
				fontResDict.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
				if _, ok := unionFonts.KV[name]; !ok {
					unionFonts.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
				}
			}
		} else {
			fRef := ensureFont("Helvetica")
			fontResDict.Set(raw.NameLiteral("F1"), raw.Ref(fRef.Num, fRef.Gen))
			if _, ok := unionFonts.KV["F1"]; !ok {
				unionFonts.Set(raw.NameLiteral("F1"), raw.Ref(fRef.Num, fRef.Gen))
			}
		}
		resDict.Set(raw.NameLiteral("Font"), fontResDict)
		if p.Resources != nil && len(p.Resources.ExtGStates) > 0 {
			gsDict := raw.Dict()
			for name, gs := range p.Resources.ExtGStates {
				entry := raw.Dict()
				if gs.LineWidth != nil {
					entry.Set(raw.NameLiteral("LW"), raw.NumberFloat(*gs.LineWidth))
				}
				if gs.StrokeAlpha != nil {
					entry.Set(raw.NameLiteral("CA"), raw.NumberFloat(*gs.StrokeAlpha))
				}
				if gs.FillAlpha != nil {
					entry.Set(raw.NameLiteral("ca"), raw.NumberFloat(*gs.FillAlpha))
				}
				gsDict.Set(raw.NameLiteral(name), entry)
				if _, ok := unionExtGStates.KV[name]; !ok {
					unionExtGStates.Set(raw.NameLiteral(name), entry)
				}
			}
			if gsDict.Len() > 0 {
				resDict.Set(raw.NameLiteral("ExtGState"), gsDict)
			}
		}
		if p.Resources != nil && len(p.Resources.ColorSpaces) > 0 {
			csDict := raw.Dict()
			for name, cs := range p.Resources.ColorSpaces {
				val := cs.Name
				if val == "" {
					val = "DeviceRGB"
				}
				csDict.Set(raw.NameLiteral(name), raw.NameLiteral(val))
				if _, ok := unionColorSpaces.KV[name]; !ok {
					unionColorSpaces.Set(raw.NameLiteral(name), raw.NameLiteral(val))
				}
			}
			if csDict.Len() > 0 {
				resDict.Set(raw.NameLiteral("ColorSpace"), csDict)
			}
		}
		if p.Resources != nil && len(p.Resources.XObjects) > 0 {
			xDict := raw.Dict()
			for name, xo := range p.Resources.XObjects {
				xref := ensureXObject(name, xo)
				xDict.Set(raw.NameLiteral(name), raw.Ref(xref.Num, xref.Gen))
				if _, ok := unionXObjects.KV[name]; !ok {
					unionXObjects.Set(raw.NameLiteral(name), raw.Ref(xref.Num, xref.Gen))
				}
			}
			if xDict.Len() > 0 {
				resDict.Set(raw.NameLiteral("XObject"), xDict)
			}
		}
		if p.Resources != nil && len(p.Resources.Patterns) > 0 {
			patDict := raw.Dict()
			for name, pat := range p.Resources.Patterns {
				pRef := ensurePattern(name, pat)
				patDict.Set(raw.NameLiteral(name), raw.Ref(pRef.Num, pRef.Gen))
				if _, ok := unionPatterns.KV[name]; !ok {
					unionPatterns.Set(raw.NameLiteral(name), raw.Ref(pRef.Num, pRef.Gen))
				}
			}
			if patDict.Len() > 0 {
				resDict.Set(raw.NameLiteral("Pattern"), patDict)
			}
		}
		if p.Resources != nil && len(p.Resources.Shadings) > 0 {
			shDict := raw.Dict()
			for name, sh := range p.Resources.Shadings {
				shRef := ensureShading(name, sh)
				shDict.Set(raw.NameLiteral(name), raw.Ref(shRef.Num, shRef.Gen))
				if _, ok := unionShadings.KV[name]; !ok {
					unionShadings.Set(raw.NameLiteral(name), raw.Ref(shRef.Num, shRef.Gen))
				}
			}
			if shDict.Len() > 0 {
				resDict.Set(raw.NameLiteral("Shading"), shDict)
			}
		}
		if procSet.Len() > 0 {
			resDict.Set(raw.NameLiteral("ProcSet"), procSet)
		}
		pageDict.Set(raw.NameLiteral("Resources"), resDict)
		// Contents
		contentRef := contentRefs[i]
		pageDict.Set(raw.NameLiteral("Contents"), raw.Ref(contentRef.Num, contentRef.Gen))

		// Annotations
		if len(p.Annotations) > 0 {
			annotArr := raw.NewArray()
			for _, a := range p.Annotations {
				aRef := nextRef()
				aDict := raw.Dict()
				aDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Annot"))
				subtype := a.Subtype
				if subtype == "" {
					subtype = "Link"
				}
				aDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(subtype))
				rect := a.Rect
				if !cropSet(rect) {
					// fall back to crop/media box coordinates
					if cropSet(p.CropBox) {
						rect = p.CropBox
					} else {
						rect = p.MediaBox
					}
				}
				aDict.Set(raw.NameLiteral("Rect"), rectArray(rect))
				if a.URI != "" {
					action := raw.Dict()
					action.Set(raw.NameLiteral("S"), raw.NameLiteral("URI"))
					action.Set(raw.NameLiteral("URI"), raw.Str([]byte(a.URI)))
					aDict.Set(raw.NameLiteral("A"), action)
				} else if a.Contents != "" {
					aDict.Set(raw.NameLiteral("Contents"), raw.Str([]byte(a.Contents)))
				}
				if len(a.Appearance) > 0 {
					apRef := nextRef()
					apDict := raw.Dict()
					apDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(a.Appearance))))
					apStream := raw.NewStream(apDict, a.Appearance)
					objects[apRef] = apStream
					ap := raw.Dict()
					ap.Set(raw.NameLiteral("N"), raw.Ref(apRef.Num, apRef.Gen))
					aDict.Set(raw.NameLiteral("AP"), ap)
				}
				aDict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(0)))
				objects[aRef] = aDict
				annotArr.Append(raw.Ref(aRef.Num, aRef.Gen))
				annotationRefs[i] = append(annotationRefs[i], aRef)
			}
			pageDict.Set(raw.NameLiteral("Annots"), annotArr)
		}
		objects[ref] = pageDict
	}
	// Pages tree
	kidsArr := raw.NewArray()
	for _, r := range pageRefs {
		kidsArr.Append(raw.Ref(r.Num, r.Gen))
	}
	pagesDict := raw.Dict()
	pagesDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pagesDict.Set(raw.NameLiteral("Count"), raw.NumberInt(int64(len(pageRefs))))
	pagesDict.Set(raw.NameLiteral("Kids"), kidsArr)
	if unionFonts.Len() > 0 {
		pagesRes := raw.Dict()
		pagesRes.Set(raw.NameLiteral("Font"), unionFonts)
		if unionExtGStates.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("ExtGState"), unionExtGStates)
		}
		if unionColorSpaces.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("ColorSpace"), unionColorSpaces)
		}
		if unionXObjects.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("XObject"), unionXObjects)
		}
		if unionPatterns.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("Pattern"), unionPatterns)
		}
		if unionShadings.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("Shading"), unionShadings)
		}
		if procSet.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("ProcSet"), procSet)
		}
		pagesDict.Set(raw.NameLiteral("Resources"), pagesRes)
	}
	objects[pagesRef] = pagesDict
	// Catalog
	catalogDict := raw.Dict()
	catalogDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	catalogDict.Set(raw.NameLiteral("Pages"), raw.Ref(pagesRef.Num, pagesRef.Gen))
	if doc.StructTree != nil {
		structRef := nextRef()
		structDict := raw.Dict()
		structDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("StructTreeRoot"))
		if len(doc.StructTree.RoleMap) > 0 {
			roleDict := raw.Dict()
			for k, v := range doc.StructTree.RoleMap {
				roleDict.Set(raw.NameLiteral(k), raw.NameLiteral(v))
			}
			structDict.Set(raw.NameLiteral("RoleMap"), roleDict)
		}
		objects[structRef] = structDict
		catalogDict.Set(raw.NameLiteral("StructTreeRoot"), raw.Ref(structRef.Num, structRef.Gen))
	}
	if metadataRef != nil {
		catalogDict.Set(raw.NameLiteral("Metadata"), raw.Ref(metadataRef.Num, metadataRef.Gen))
	}
	if doc.Info != nil && doc.Info.Title != "" {
		vp := raw.Dict()
		vp.Set(raw.NameLiteral("DisplayDocTitle"), raw.Bool(true))
		catalogDict.Set(raw.NameLiteral("ViewerPreferences"), vp)
	}
	if len(doc.PageLabels) > 0 {
		nums := raw.NewArray()
		indices := make([]int, 0, len(doc.PageLabels))
		for idx := range doc.PageLabels {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			nums.Append(raw.NumberInt(int64(idx)))
			entry := raw.Dict()
			entry.Set(raw.NameLiteral("P"), raw.Str([]byte(doc.PageLabels[idx])))
			nums.Append(entry)
		}
		labels := raw.Dict()
		labels.Set(raw.NameLiteral("Nums"), nums)
		catalogDict.Set(raw.NameLiteral("PageLabels"), labels)
	}
	if len(doc.Outlines) > 0 {
		outlineRef := nextRef()
		outlineDict := raw.Dict()
		outlineDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Outlines"))
		objects[outlineRef] = outlineDict

		first, last, total := w.buildOutlines(doc.Outlines, outlineRef, pageRefs, objects, nextRef, outlineRef)
		outlineDict.Set(raw.NameLiteral("First"), raw.Ref(first.Num, first.Gen))
		outlineDict.Set(raw.NameLiteral("Last"), raw.Ref(last.Num, last.Gen))
		outlineDict.Set(raw.NameLiteral("Count"), raw.NumberInt(total))
		catalogDict.Set(raw.NameLiteral("Outlines"), raw.Ref(outlineRef.Num, outlineRef.Gen))
		catalogDict.Set(raw.NameLiteral("PageMode"), raw.NameLiteral("UseOutlines"))
	}
	if doc.AcroForm != nil {
		formRef := nextRef()
		formDict := raw.Dict()
		fieldsArr := raw.NewArray()
		for _, f := range doc.AcroForm.Fields {
			fieldRef := nextRef()
			fd := raw.Dict()
			if f.Type != "" {
				fd.Set(raw.NameLiteral("FT"), raw.NameLiteral(f.Type))
			} else {
				fd.Set(raw.NameLiteral("FT"), raw.NameLiteral("Tx"))
			}
			if f.Name != "" {
				fd.Set(raw.NameLiteral("T"), raw.Str([]byte(f.Name)))
			}
			if f.Value != "" {
				fd.Set(raw.NameLiteral("V"), raw.Str([]byte(f.Value)))
			}
			objects[fieldRef] = fd
			fieldsArr.Append(raw.Ref(fieldRef.Num, fieldRef.Gen))
		}
		formDict.Set(raw.NameLiteral("Fields"), fieldsArr)
		if doc.AcroForm.NeedAppearances {
			formDict.Set(raw.NameLiteral("NeedAppearances"), raw.Bool(true))
		}
		objects[formRef] = formDict
		catalogDict.Set(raw.NameLiteral("AcroForm"), raw.Ref(formRef.Num, formRef.Gen))
	}
	if len(outputIntentRefs) > 0 {
		arr := raw.NewArray()
		for _, ref := range outputIntentRefs {
			arr.Append(raw.Ref(ref.Num, ref.Gen))
		}
		catalogDict.Set(raw.NameLiteral("OutputIntents"), arr)
	}
	objects[catalogRef] = catalogDict

	// Serialize
	var buf bytes.Buffer
	initialOffset := int64(0)
	if len(incr.base) > 0 {
		initialOffset = int64(len(incr.base))
		if !bytes.HasSuffix(incr.base, []byte("\n")) {
			buf.WriteByte('\n')
		}
	} else {
		buf.WriteString("%PDF-" + version + "\n%\xE2\xE3\xCF\xD3\n")
	}
	offsets := make(map[int]int64)
	for k, v := range incr.prevOffsets {
		offsets[k] = v
	}

	ordered := make([]raw.ObjectRef, 0, len(objects))
	for ref := range objects {
		ordered = append(ordered, ref)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Num < ordered[j].Num })
	for _, ref := range ordered {
		offset := initialOffset + int64(buf.Len())
		serialized, _ := w.SerializeObject(ref, objects[ref])
		buf.Write(serialized)
		offsets[ref.Num] = offset
	}
	// XRef
	if cfg.XRefStreams {
		xrefRef := nextRef()
		maxObjNum := xrefRef.Num
		if len(offsets) > 0 {
			for n := range offsets {
				if n > maxObjNum {
					maxObjNum = n
				}
			}
		}
		xrefOffset := initialOffset + int64(buf.Len())
		offsets[xrefRef.Num] = xrefOffset
		size := maxInt(maxObjNum, incr.prevMaxObj) + 1

		trailer := buildTrailer(size, catalogRef, infoRef, doc, cfg, incr.prevOffset)
		trailer.Set(raw.NameLiteral("Type"), raw.NameLiteral("XRef"))
		trailer.Set(raw.NameLiteral("W"), raw.NewArray(raw.NumberInt(1), raw.NumberInt(4), raw.NumberInt(1)))
		indexArr, entries := xrefStreamIndexAndEntries(offsets)
		trailer.Set(raw.NameLiteral("Index"), indexArr)
		trailer.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(entries))))

		xrefStream := raw.NewStream(trailer, entries)
		serialized, _ := w.SerializeObject(xrefRef, xrefStream)
		buf.Write(serialized)

		buf.WriteString("startxref\n")
		buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))
	} else {
		xrefOffset := initialOffset + int64(buf.Len())
		maxObjNum := ordered[len(ordered)-1].Num
		size := maxInt(maxObjNum, incr.prevMaxObj) + 1
		buf.WriteString("xref\n0 ")
		buf.WriteString(fmt.Sprintf("%d\n", size))
		buf.WriteString("0000000000 65535 f \n")
		for i := 1; i <= maxObjNum; i++ {
			if off, ok := offsets[i]; ok {
				buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
			} else {
				buf.WriteString("0000000000 65535 f \n")
			}
		}
		trailer := buildTrailer(size, catalogRef, infoRef, doc, cfg, incr.prevOffset)
		buf.WriteString("trailer\n")
		buf.Write(serializePrimitive(trailer))
		buf.WriteString("\nstartxref\n")
		buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))
	}

	_, err := out.Write(buf.Bytes())
	return err
}

func (w *impl) buildOutlines(items []semantic.OutlineItem, parent raw.ObjectRef, pageRefs []raw.ObjectRef, objects map[raw.ObjectRef]raw.Object, nextRef func() raw.ObjectRef, root raw.ObjectRef) (first, last raw.ObjectRef, count int64) {
	if len(items) == 0 {
		return first, last, 0
	}
	refs := make([]raw.ObjectRef, len(items))
	for i := range items {
		refs[i] = nextRef()
	}
	for i, item := range items {
		count++
		d := raw.Dict()
		d.Set(raw.NameLiteral("Title"), raw.Str([]byte(item.Title)))
		if item.PageIndex >= 0 && item.PageIndex < len(pageRefs) {
			pref := pageRefs[item.PageIndex]
			dest := raw.NewArray(raw.Ref(pref.Num, pref.Gen), raw.NameLiteral("Fit"))
			d.Set(raw.NameLiteral("Dest"), dest)
		}
		d.Set(raw.NameLiteral("Parent"), raw.Ref(parent.Num, parent.Gen))
		if i > 0 {
			d.Set(raw.NameLiteral("Prev"), raw.Ref(refs[i-1].Num, refs[i-1].Gen))
		}
		if i < len(refs)-1 {
			d.Set(raw.NameLiteral("Next"), raw.Ref(refs[i+1].Num, refs[i+1].Gen))
		}
		if len(item.Children) > 0 {
			firstChild, lastChild, childCount := w.buildOutlines(item.Children, refs[i], pageRefs, objects, nextRef, root)
			d.Set(raw.NameLiteral("First"), raw.Ref(firstChild.Num, firstChild.Gen))
			d.Set(raw.NameLiteral("Last"), raw.Ref(lastChild.Num, lastChild.Gen))
			d.Set(raw.NameLiteral("Count"), raw.NumberInt(childCount))
			count += childCount
		}
		objects[refs[i]] = d
	}
	first = refs[0]
	last = refs[len(refs)-1]
	return first, last, count
}

func serializePrimitive(o raw.Object) []byte {
	switch v := o.(type) {
	case raw.NameObj:
		return []byte("/" + v.Value())
	case raw.NumberObj:
		if v.IsInteger() {
			return []byte(fmt.Sprintf("%d", v.Int()))
		}
		return []byte(fmt.Sprintf("%f", v.Float()))
	case raw.BoolObj:
		if v.Value() {
			return []byte("true")
		}
		return []byte("false")
	case raw.NullObj:
		return []byte("null")
	case raw.String:
		if v.IsHex() {
			dst := make([]byte, hex.EncodedLen(len(v.Value())))
			hex.Encode(dst, v.Value())
			return []byte("<" + strings.ToUpper(string(dst)) + ">")
		}
		return escapeLiteralString(v.Value())
	case *raw.ArrayObj:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, it := range v.Items {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.Write(serializePrimitive(it))
		}
		b.WriteByte(']')
		return b.Bytes()
	case *raw.DictObj:
		var b bytes.Buffer
		b.WriteString("<<")
		keys := make([]string, 0, len(v.KV))
		for k := range v.KV {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("/" + k + " ")
			b.Write(serializePrimitive(v.KV[k]))
		}
		b.WriteString(">>")
		return b.Bytes()
	case *raw.StreamObj:
		var b bytes.Buffer
		d := serializePrimitive(v.Dict)
		b.Write(d)
		b.WriteString("stream\n")
		b.Write(v.Data)
		b.WriteString("\nendstream")
		return b.Bytes()
	case raw.RefObj:
		return []byte(fmt.Sprintf("%d %d R", v.Ref().Num, v.Ref().Gen))
	default:
		return []byte("null")
	}
}

func pdfVersion(cfg Config) string {
	if cfg.Version == "" {
		return string(PDF17)
	}
	return string(cfg.Version)
}

func fileID(doc *semantic.Document, cfg Config) [2][]byte {
	seed := deterministicIDSeed(doc, cfg)
	if cfg.Deterministic {
		return [2][]byte{seed, seed}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		id = seed
	}
	idB := make([]byte, len(id))
	copy(idB, id)
	return [2][]byte{id, idB}
}

func deterministicIDSeed(doc *semantic.Document, cfg Config) []byte {
	h := sha256.New()
	h.Write([]byte(pdfVersion(cfg)))
	if doc.Info != nil {
		h.Write([]byte(doc.Info.Title))
		h.Write([]byte(doc.Info.Author))
		h.Write([]byte(doc.Info.Subject))
		h.Write([]byte(doc.Info.Creator))
		h.Write([]byte(doc.Info.Producer))
		if len(doc.Info.Keywords) > 0 {
			h.Write([]byte(strings.Join(doc.Info.Keywords, ",")))
		}
	}
	if doc.Metadata != nil {
		h.Write(doc.Metadata.Raw)
	}
	h.Write([]byte(fmt.Sprintf("%d", len(doc.Pages))))
	for _, p := range doc.Pages {
		h.Write([]byte(fmt.Sprintf("%f-%f-%f-%f-%d", p.MediaBox.LLX, p.MediaBox.LLY, p.MediaBox.URX, p.MediaBox.URY, p.Rotate)))
	}
	sum := h.Sum(nil)
	if len(sum) >= 16 {
		return sum[:16]
	}
	buf := make([]byte, 16)
	copy(buf, sum)
	return buf
}

func buildTrailer(size int, catalogRef raw.ObjectRef, infoRef *raw.ObjectRef, doc *semantic.Document, cfg Config, prev int64) *raw.DictObj {
	trailer := raw.Dict()
	trailer.Set(raw.NameLiteral("Size"), raw.NumberInt(int64(size)))
	trailer.Set(raw.NameLiteral("Root"), raw.Ref(catalogRef.Num, catalogRef.Gen))
	if infoRef != nil {
		trailer.Set(raw.NameLiteral("Info"), raw.Ref(infoRef.Num, infoRef.Gen))
	}
	fileIDs := fileID(doc, cfg)
	idArr := raw.NewArray(
		raw.Str([]byte(hex.EncodeToString(fileIDs[0]))),
		raw.Str([]byte(hex.EncodeToString(fileIDs[1]))),
	)
	trailer.Set(raw.NameLiteral("ID"), idArr)
	if prev > 0 && cfg.Incremental {
		trailer.Set(raw.NameLiteral("Prev"), raw.NumberInt(prev))
	}
	return trailer
}

func xrefStreamIndexAndEntries(offsets map[int]int64) (*raw.ArrayObj, []byte) {
	offCopy := make(map[int]int64, len(offsets)+1)
	for k, v := range offsets {
		offCopy[k] = v
	}
	if _, ok := offCopy[0]; !ok {
		offCopy[0] = 0
	}
	keys := make([]int, 0, len(offCopy))
	for k := range offCopy {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	indexArr := raw.NewArray()
	var entries []byte
	segStart := -1
	prev := -1
	for _, k := range keys {
		if segStart == -1 {
			segStart = k
			prev = k
		} else if k != prev+1 {
			indexArr.Append(raw.NumberInt(int64(segStart)))
			indexArr.Append(raw.NumberInt(int64(prev - segStart + 1)))
			segStart = k
		}
		prev = k

		off := offCopy[k]
		typ := 0
		gen := 0
		if k == 0 {
			gen = 65535
		}
		if off > 0 {
			typ = 1
		}
		entries = appendXRefStreamEntry(entries, typ, off, gen)
	}
	if segStart != -1 {
		indexArr.Append(raw.NumberInt(int64(segStart)))
		indexArr.Append(raw.NumberInt(int64(prev - segStart + 1)))
	}
	return indexArr, entries
}

func appendXRefStreamEntry(buf []byte, typ int, field2 int64, gen int) []byte {
	buf = append(buf, byte(typ))
	offset := uint32(field2)
	buf = append(buf, byte(offset>>24), byte(offset>>16), byte(offset>>8), byte(offset))
	buf = append(buf, byte(gen))
	return buf
}

// escapeLiteralString escapes characters that need backslash protection in literal strings.
func escapeLiteralString(rawBytes []byte) []byte {
	var b bytes.Buffer
	b.WriteByte('(')
	for _, ch := range rawBytes {
		switch ch {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteByte(ch)
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		default:
			if ch < 0x20 || ch >= 0x80 {
				fmt.Fprintf(&b, "\\%03o", ch)
			} else {
				b.WriteByte(ch)
			}
		}
	}
	b.WriteByte(')')
	return b.Bytes()
}

type incrementalInfo struct {
	base        []byte
	prevOffset  int64
	startObjNum int
	prevMaxObj  int
	prevOffsets map[int]int64
}

func incrementalContext(doc *semantic.Document, out WriterAt, cfg Config) incrementalInfo {
	info := incrementalInfo{startObjNum: 1}
	if !cfg.Incremental {
		return info
	}
	reader, ok := out.(interface{ Bytes() []byte })
	if !ok {
		return info
	}
	info.base = append([]byte(nil), reader.Bytes()...)
	info.prevOffsets = scanObjectOffsets(info.base)
	info.prevOffset = lastStartXRef(info.base)
	info.prevMaxObj = maxObjNumFromBytes(info.base)
	info.startObjNum = info.prevMaxObj + 1
	if dec := doc.Decoded(); dec != nil && dec.Raw != nil {
		for ref := range dec.Raw.Objects {
			if ref.Num >= info.startObjNum {
				info.startObjNum = ref.Num + 1
			}
		}
	}
	if info.startObjNum < 1 {
		info.startObjNum = 1
	}
	return info
}

func lastStartXRef(data []byte) int64 {
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	matches := re.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return 0
	}
	// use last occurrence
	m := matches[len(matches)-1]
	off, err := strconv.ParseInt(string(m[1]), 10, 64)
	if err != nil {
		return 0
	}
	return off
}

func maxObjNumFromBytes(data []byte) int {
	re := regexp.MustCompile(`\s(\d+)\s+0\s+obj`)
	matches := re.FindAllSubmatch(data, -1)
	max := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		num, err := strconv.Atoi(string(m[1]))
		if err != nil {
			continue
		}
		if num > max {
			max = num
		}
	}
	return max
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func xoKey(name string, xo semantic.XObject) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(xo.Subtype))
	h.Write([]byte(fmt.Sprintf("%d-%d-%d", xo.Width, xo.Height, xo.BitsPerComponent)))
	h.Write([]byte(xo.ColorSpace.Name))
	h.Write([]byte(fmt.Sprintf("%f-%f-%f-%f", xo.BBox.LLX, xo.BBox.LLY, xo.BBox.URX, xo.BBox.URY)))
	h.Write(xo.Data)
	return hex.EncodeToString(h.Sum(nil))
}

func patternKey(name string, p semantic.Pattern) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(fmt.Sprintf("%d-%d-%d", p.PatternType, p.PaintType, p.TilingType)))
	h.Write([]byte(fmt.Sprintf("%f-%f-%f-%f-%f-%f", p.BBox.LLX, p.BBox.LLY, p.BBox.URX, p.BBox.URY, p.XStep, p.YStep)))
	h.Write(p.Content)
	return hex.EncodeToString(h.Sum(nil))
}

func shadingKey(name string, s semantic.Shading) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(fmt.Sprintf("%d-%s", s.ShadingType, s.ColorSpace.Name)))
	for _, c := range s.Coords {
		h.Write([]byte(fmt.Sprintf("%f", c)))
	}
	for _, d := range s.Domain {
		h.Write([]byte(fmt.Sprintf("%f", d)))
	}
	h.Write(s.Function)
	return hex.EncodeToString(h.Sum(nil))
}

func scanObjectOffsets(data []byte) map[int]int64 {
	re := regexp.MustCompile(`(?m)^(\d+)\s+0\s+obj`)
	matches := re.FindAllStringSubmatchIndex(string(data), -1)
	offsets := make(map[int]int64, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		num, err := strconv.Atoi(string(data[m[2]:m[3]]))
		if err != nil {
			continue
		}
		if _, exists := offsets[num]; exists {
			continue
		}
		offsets[num] = int64(m[0])
	}
	return offsets
}

func serializeContentStream(cs semantic.ContentStream) []byte {
	if len(cs.RawBytes) > 0 {
		return cs.RawBytes
	}
	if len(cs.Operations) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, op := range cs.Operations {
		for i, operand := range op.Operands {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(serializeOperand(operand))
		}
		if len(op.Operands) > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(op.Operator)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func serializeOperand(op semantic.Operand) []byte {
	switch v := op.(type) {
	case semantic.NumberOperand:
		// %g keeps minimal form while preserving integer vs float readability.
		return []byte(fmt.Sprintf("%g", v.Value))
	case semantic.NameOperand:
		return []byte("/" + v.Value)
	case semantic.StringOperand:
		return escapeLiteralString(v.Value)
	case semantic.ArrayOperand:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, it := range v.Values {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(serializeOperand(it))
		}
		buf.WriteByte(']')
		return buf.Bytes()
	case semantic.DictOperand:
		var buf bytes.Buffer
		buf.WriteString("<<")
		keys := make([]string, 0, len(v.Values))
		for k := range v.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("/" + k + " ")
			buf.Write(serializeOperand(v.Values[k]))
		}
		buf.WriteString(">>")
		return buf.Bytes()
	default:
		return []byte("null")
	}
}

func pickContentFilter(cfg Config) ContentFilter {
	if cfg.ContentFilter != FilterNone {
		return cfg.ContentFilter
	}
	if cfg.Compression > 0 {
		return FilterFlate
	}
	return FilterNone
}

func flateEncode(data []byte, level int) ([]byte, error) {
	if level < flate.NoCompression || level > flate.BestCompression {
		level = flate.DefaultCompression
	}
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, level)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func asciiHexEncode(data []byte) []byte {
	dst := make([]byte, hex.EncodedLen(len(data))+1)
	hex.Encode(dst[:len(dst)-1], data)
	for i := 0; i < len(dst)-1; i++ {
		if dst[i] >= 'a' && dst[i] <= 'f' {
			dst[i] -= 32 // upper-case
		}
	}
	dst[len(dst)-1] = '>'
	return dst
}

func ascii85Encode(data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("<~")
	enc := make([]byte, ascii85.MaxEncodedLen(len(data)))
	n := ascii85.Encode(enc, data)
	buf.Write(enc[:n])
	buf.WriteString("~>")
	return buf.Bytes()
}

// runLengthEncode implements PDF RunLength filter encoding.
func runLengthEncode(data []byte) []byte {
	if len(data) == 0 {
		return []byte{128} // EOD marker
	}
	var out bytes.Buffer
	for i := 0; i < len(data); {
		// Detect run
		runLen := 1
		for i+runLen < len(data) && data[i+runLen] == data[i] && runLen < 128 {
			runLen++
		}
		if runLen > 1 {
			out.WriteByte(byte(257 - runLen))
			out.WriteByte(data[i])
			i += runLen
			continue
		}
		// Literal sequence
		litStart := i
		for i < len(data) && (i+1 >= len(data) || data[i] != data[i+1]) && i-litStart < 127 {
			i++
			if i < len(data) && i+1 < len(data) && data[i] == data[i+1] {
				break
			}
		}
		out.WriteByte(byte(i - litStart - 1))
		out.Write(data[litStart:i])
	}
	out.WriteByte(128) // EOD
	return out.Bytes()
}

func lzwEncode(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, lzw.LSB, 8)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func rectArray(r semantic.Rectangle) *raw.ArrayObj {
	return raw.NewArray(
		raw.NumberFloat(r.LLX),
		raw.NumberFloat(r.LLY),
		raw.NumberFloat(r.URX),
		raw.NumberFloat(r.URY),
	)
}

func cropSet(r semantic.Rectangle) bool {
	return r.URX > r.LLX && r.URY > r.LLY
}

func normalizeRotation(rot int) int {
	if rot == 0 {
		return 0
	}
	rot %= 360
	if rot < 0 {
		rot += 360
	}
	return rot
}
