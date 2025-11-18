package writer

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/hex"
	"fmt"
	"math"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/observability"
	"pdflib/security"
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

func (w *impl) Write(ctx Context, doc *semantic.Document, out WriterAt, cfg Config) (err error) {
	tracer := tracerFromConfig(cfg)
	goctx, writeSpan := tracer.StartSpan(contextFrom(ctx), "writer.write")
	writeSpan.SetTag("pages", len(doc.Pages))
	writeSpan.SetTag("xref_streams", cfg.XRefStreams)
	writeSpan.SetTag("incremental", cfg.Incremental)
	logger := loggerFromConfig(cfg)
	logger.Info("writer.write.start", observability.Int("pages", len(doc.Pages)))
	defer func() {
		if err != nil {
			writeSpan.SetError(err)
			logger.Error("writer.write.error", observability.Error("err", err))
		} else {
			logger.Info("writer.write.finish", observability.Int("pages", len(doc.Pages)))
		}
		writeSpan.Finish()
	}()

	version := pdfVersion(cfg)
	incr := incrementalContext(doc, out, cfg)
	idPair := fileID(doc, cfg)
	// Build raw objects from semantic (minimal subset: catalog, pages, page, fonts, content streams)
	objects := make(map[raw.ObjectRef]raw.Object)
	objNum := incr.startObjNum
	nextRef := func() raw.ObjectRef {
		ref := raw.ObjectRef{Num: objNum, Gen: 0}
		objNum++
		return ref
	}
	_, buildSpan := tracer.StartSpan(goctx, "writer.build_objects")
	defer buildSpan.Finish()
	catalogRef := nextRef()
	pagesRef := nextRef()
	pageRefs := make([]raw.ObjectRef, 0, len(doc.Pages))

	// Fonts (shared across pages by BaseFont)
	fontRefs := map[string]raw.ObjectRef{}
	xobjectRefs := map[string]raw.ObjectRef{}
	patternRefs := map[string]raw.ObjectRef{}
	shadingRefs := map[string]raw.ObjectRef{}
	ensureFont := func(font *semantic.Font) raw.ObjectRef {
		base := "Helvetica"
		encoding := ""
		subtype := "Type1"
		if font != nil {
			if font.BaseFont != "" {
				base = font.BaseFont
			}
			encoding = font.Encoding
			if font.Subtype != "" {
				subtype = font.Subtype
			}
		}
		key := fontKey(base, encoding, subtype, font)
		if ref, ok := fontRefs[key]; ok {
			return ref
		}
		ref := nextRef()
		fontDict := raw.Dict()
		fontDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Font"))
		fontDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(subtype))
		fontDict.Set(raw.NameLiteral("BaseFont"), raw.NameLiteral(base))

		if subtype == "Type0" {
			desc := font.DescendantFont
			if encoding == "" {
				encoding = "Identity-H"
			}
			fontDict.Set(raw.NameLiteral("Encoding"), raw.NameLiteral(encoding))

			descRef := nextRef()
			descDict := raw.Dict()
			descDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Font"))
			descSubtype := "CIDFontType2"
			if desc != nil && desc.Subtype != "" {
				descSubtype = desc.Subtype
			}
			descDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(descSubtype))
			descBase := base
			if desc != nil && desc.BaseFont != "" {
				descBase = desc.BaseFont
			}
			descDict.Set(raw.NameLiteral("BaseFont"), raw.NameLiteral(descBase))

			var csi *semantic.CIDSystemInfo
			if font.CIDSystemInfo != nil {
				csi = font.CIDSystemInfo
			} else if desc != nil {
				csi = &desc.CIDSystemInfo
			}
			if csi != nil {
				cs := raw.Dict()
				reg := csi.Registry
				if reg == "" {
					reg = "Adobe"
				}
				ord := csi.Ordering
				if ord == "" {
					ord = "Identity"
				}
				cs.Set(raw.NameLiteral("Registry"), raw.Str([]byte(reg)))
				cs.Set(raw.NameLiteral("Ordering"), raw.Str([]byte(ord)))
				cs.Set(raw.NameLiteral("Supplement"), raw.NumberInt(int64(csi.Supplement)))
				descDict.Set(raw.NameLiteral("CIDSystemInfo"), cs)
			}
			dw := 1000
			if desc != nil && desc.DW > 0 {
				dw = desc.DW
			}
			descDict.Set(raw.NameLiteral("DW"), raw.NumberInt(int64(dw)))
			widths := map[int]int{}
			if desc != nil && len(desc.W) > 0 {
				widths = desc.W
			} else if font != nil && len(font.Widths) > 0 {
				widths = font.Widths
			}
			if len(widths) > 0 {
				descDict.Set(raw.NameLiteral("W"), encodeCIDWidths(widths))
			}
			objects[descRef] = descDict
			fontDict.Set(raw.NameLiteral("DescendantFonts"), raw.NewArray(raw.Ref(descRef.Num, descRef.Gen)))
		} else {
			if encoding != "" {
				fontDict.Set(raw.NameLiteral("Encoding"), raw.NameLiteral(encoding))
			}
			if font != nil && len(font.Widths) > 0 {
				first, last, widthsArr := encodeWidths(font.Widths)
				fontDict.Set(raw.NameLiteral("FirstChar"), raw.NumberInt(int64(first)))
				fontDict.Set(raw.NameLiteral("LastChar"), raw.NumberInt(int64(last)))
				fontDict.Set(raw.NameLiteral("Widths"), widthsArr)
			}
		}
		objects[ref] = fontDict
		fontRefs[key] = ref
		return ref
	}
	var ensureXObject func(name string, xo semantic.XObject) raw.ObjectRef
	ensureXObject = func(name string, xo semantic.XObject) raw.ObjectRef {
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
			if xo.Interpolate {
				dict.Set(raw.NameLiteral("Interpolate"), raw.Bool(true))
			}
		}
		if sub == "Form" && cropSet(xo.BBox) {
			dict.Set(raw.NameLiteral("BBox"), rectArray(xo.BBox))
		}
		if xo.SMask != nil {
			maskName := fmt.Sprintf("%s:SMask", name)
			maskRef := ensureXObject(maskName, *xo.SMask)
			dict.Set(raw.NameLiteral("SMask"), raw.Ref(maskRef.Num, maskRef.Gen))
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

	// Encrypt dictionary (Standard handler)
	var encryptRef *raw.ObjectRef
	var encryptionHandler security.Handler
	if doc.Encrypted {
		_, encSpan := tracer.StartSpan(goctx, "writer.encrypt")
		encSpan.SetTag("metadata_encrypted", doc.MetadataEncrypted)
		ref := nextRef()
		encryptRef = &ref
		enc, _, err := security.BuildStandardEncryption(doc.UserPassword, doc.OwnerPassword, doc.Permissions, idPair[0], doc.MetadataEncrypted)
		if err != nil {
			encSpan.SetError(err)
			encSpan.Finish()
			return err
		}
		handler, err := (&security.HandlerBuilder{}).WithEncryptDict(enc).WithFileID(idPair[0]).Build()
		if err != nil {
			encSpan.SetError(err)
			encSpan.Finish()
			return err
		}
		if err := handler.Authenticate(doc.UserPassword); err != nil {
			encSpan.SetError(err)
			encSpan.Finish()
			return err
		}
		encryptionHandler = handler
		objects[ref] = enc
		encSpan.Finish()
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
		case FilterJPX:
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("JPXDecode"))
		case FilterJBIG2:
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("JBIG2Decode"))
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
	procEntries := map[string]bool{"PDF": true, "Text": true}
	procSet := raw.NewArray(raw.NameLiteral("PDF"), raw.NameLiteral("Text"))
	addProc := func(name string) {
		if !procEntries[name] {
			procEntries[name] = true
			procSet.Append(raw.NameLiteral(name))
		}
	}
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
				fRef := ensureFont(font)
				fontResDict.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
				unionFonts.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
			}
		} else {
			fRef := ensureFont(nil)
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
				if xo.Subtype == "Image" || xo.Subtype == "" {
					if xo.ColorSpace.Name == "DeviceGray" {
						addProc("ImageB")
					} else {
						addProc("ImageC")
					}
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
				if a.Flags != 0 {
					aDict.Set(raw.NameLiteral("F"), raw.NumberInt(int64(a.Flags)))
				}
				if len(a.Border) == 3 {
					aDict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberFloat(a.Border[0]), raw.NumberFloat(a.Border[1]), raw.NumberFloat(a.Border[2])))
				} else {
					aDict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(0)))
				}
				if len(a.Color) > 0 {
					colArr := raw.NewArray()
					for _, c := range a.Color {
						colArr.Append(raw.NumberFloat(c))
					}
					aDict.Set(raw.NameLiteral("C"), colArr)
				}
				if a.AppearanceState != "" {
					aDict.Set(raw.NameLiteral("AS"), raw.NameLiteral(a.AppearanceState))
				}
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
	if doc.Lang != "" {
		catalogDict.Set(raw.NameLiteral("Lang"), raw.Str([]byte(doc.Lang)))
	}
	if doc.Marked || doc.StructTree != nil {
		mark := raw.Dict()
		mark.Set(raw.NameLiteral("Marked"), raw.Bool(true))
		catalogDict.Set(raw.NameLiteral("MarkInfo"), mark)
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
	if len(doc.Articles) > 0 {
		threadArr := raw.NewArray()
		for _, art := range doc.Articles {
			if len(art.Beads) == 0 {
				continue
			}
			threadRef := nextRef()
			threadArr.Append(raw.Ref(threadRef.Num, threadRef.Gen))
			threadDict := raw.Dict()
			threadDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Thread"))
			if art.Title != "" {
				threadDict.Set(raw.NameLiteral("T"), raw.Str([]byte(art.Title)))
			}
			beadRefs := make([]raw.ObjectRef, len(art.Beads))
			for i := range art.Beads {
				beadRefs[i] = nextRef()
			}
			for i, bead := range art.Beads {
				bd := raw.Dict()
				bd.Set(raw.NameLiteral("Type"), raw.NameLiteral("Bead"))
				if bead.PageIndex >= 0 && bead.PageIndex < len(pageRefs) {
					pref := pageRefs[bead.PageIndex]
					bd.Set(raw.NameLiteral("P"), raw.Ref(pref.Num, pref.Gen))
				}
				if cropSet(bead.Rect) {
					bd.Set(raw.NameLiteral("R"), rectArray(bead.Rect))
				}
				if i > 0 {
					prev := beadRefs[i-1]
					bd.Set(raw.NameLiteral("V"), raw.Ref(prev.Num, prev.Gen))
				}
				if i < len(beadRefs)-1 {
					nextBead := beadRefs[i+1]
					bd.Set(raw.NameLiteral("N"), raw.Ref(nextBead.Num, nextBead.Gen))
				}
				objects[beadRefs[i]] = bd
			}
			threadDict.Set(raw.NameLiteral("K"), raw.Ref(beadRefs[0].Num, beadRefs[0].Gen))
			objects[threadRef] = threadDict
		}
		if threadArr.Len() > 0 {
			catalogDict.Set(raw.NameLiteral("Threads"), threadArr)
		}
	}
	if doc.AcroForm != nil {
		formRef := nextRef()
		formDict := raw.Dict()
		fieldsArr := raw.NewArray()
		appendWidgetToPage := func(pageIdx int, ref raw.ObjectRef) {
			if pageIdx < 0 || pageIdx >= len(pageRefs) {
				return
			}
			pref := pageRefs[pageIdx]
			pageObj := objects[pref]
			pd, ok := pageObj.(*raw.DictObj)
			if !ok {
				return
			}
			if annotsVal, ok := pd.Get(raw.NameLiteral("Annots")); ok {
				if arr, ok := annotsVal.(*raw.ArrayObj); ok {
					arr.Append(raw.Ref(ref.Num, ref.Gen))
					return
				}
			}
			arr := raw.NewArray()
			arr.Append(raw.Ref(ref.Num, ref.Gen))
			pd.Set(raw.NameLiteral("Annots"), arr)
		}
		for _, f := range doc.AcroForm.Fields {
			fieldRef := nextRef()
			fd := raw.Dict()
			if f.Type != "" {
				fd.Set(raw.NameLiteral("FT"), raw.NameLiteral(f.Type))
			} else {
				fd.Set(raw.NameLiteral("FT"), raw.NameLiteral("Tx"))
			}
			fd.Set(raw.NameLiteral("Type"), raw.NameLiteral("Annot"))
			fd.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Widget"))
			if f.Name != "" {
				fd.Set(raw.NameLiteral("T"), raw.Str([]byte(f.Name)))
			}
			if f.Value != "" {
				fd.Set(raw.NameLiteral("V"), raw.Str([]byte(f.Value)))
			}
			if f.Flags != 0 {
				fd.Set(raw.NameLiteral("Ff"), raw.NumberInt(int64(f.Flags)))
				fd.Set(raw.NameLiteral("F"), raw.NumberInt(int64(f.Flags)))
			}
			if cropSet(f.Rect) {
				fd.Set(raw.NameLiteral("Rect"), rectArray(f.Rect))
			}
			if f.PageIndex >= 0 && f.PageIndex < len(pageRefs) {
				pref := pageRefs[f.PageIndex]
				fd.Set(raw.NameLiteral("P"), raw.Ref(pref.Num, pref.Gen))
				appendWidgetToPage(f.PageIndex, fieldRef)
			}
			if len(f.Border) == 3 {
				fd.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberFloat(f.Border[0]), raw.NumberFloat(f.Border[1]), raw.NumberFloat(f.Border[2])))
			} else {
				fd.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(0)))
			}
			if len(f.Color) > 0 {
				colArr := raw.NewArray()
				for _, c := range f.Color {
					colArr.Append(raw.NumberFloat(c))
				}
				fd.Set(raw.NameLiteral("C"), colArr)
			}
			if len(f.Appearance) > 0 {
				apRef := nextRef()
				apDict := raw.Dict()
				apDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(f.Appearance))))
				objects[apRef] = raw.NewStream(apDict, f.Appearance)
				ap := raw.Dict()
				ap.Set(raw.NameLiteral("N"), raw.Ref(apRef.Num, apRef.Gen))
				fd.Set(raw.NameLiteral("AP"), ap)
			}
			if f.AppearanceState != "" {
				fd.Set(raw.NameLiteral("AS"), raw.NameLiteral(f.AppearanceState))
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

	if encryptionHandler != nil {
		for ref, obj := range objects {
			if encryptRef != nil && ref == *encryptRef {
				continue
			}
			if metadataRef != nil && ref == *metadataRef && !doc.MetadataEncrypted {
				continue
			}
			objects[ref] = encryptObject(obj, ref, encryptionHandler, metadataRef, doc.MetadataEncrypted)
		}
	}

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

	_, serializeSpan := tracer.StartSpan(goctx, "writer.serialize_objects")
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
	serializeSpan.Finish()
	// XRef
	_, xrefSpan := tracer.StartSpan(goctx, "writer.xref")
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

		trailer := buildTrailer(size, catalogRef, infoRef, encryptRef, doc, cfg, incr.prevOffset, idPair)
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
		trailer := buildTrailer(size, catalogRef, infoRef, encryptRef, doc, cfg, incr.prevOffset, idPair)
		buf.WriteString("trailer\n")
		buf.Write(serializePrimitive(trailer))
		buf.WriteString("\nstartxref\n")
		buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))
	}
	xrefSpan.Finish()

	_, err = out.Write(buf.Bytes())
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
			var dest raw.Object
			if item.Dest != nil {
				dest = raw.NewArray(
					raw.Ref(pref.Num, pref.Gen),
					raw.NameLiteral("XYZ"),
					xyzDestValue(item.Dest.X),
					xyzDestValue(item.Dest.Y),
					xyzDestValue(item.Dest.Zoom),
				)
			} else {
				dest = raw.NewArray(raw.Ref(pref.Num, pref.Gen), raw.NameLiteral("Fit"))
			}
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

func xyzDestValue(v *float64) raw.Object {
	if v == nil {
		return raw.NullObj{}
	}
	return raw.NumberFloat(*v)
}

func tracerFromConfig(cfg Config) observability.Tracer {
	if cfg.Tracer != nil {
		return cfg.Tracer
	}
	return observability.NopTracer()
}

func loggerFromConfig(cfg Config) observability.Logger {
	if cfg.Logger != nil {
		return cfg.Logger
	}
	return observability.NopLogger{}
}

func contextFrom(ctx Context) context.Context {
	if c, ok := ctx.(context.Context); ok {
		return c
	}
	if c, ok := ctx.(interface{ Context() context.Context }); ok {
		return c.Context()
	}
	return context.Background()
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

func buildTrailer(size int, catalogRef raw.ObjectRef, infoRef *raw.ObjectRef, encryptRef *raw.ObjectRef, doc *semantic.Document, cfg Config, prev int64, ids [2][]byte) *raw.DictObj {
	trailer := raw.Dict()
	trailer.Set(raw.NameLiteral("Size"), raw.NumberInt(int64(size)))
	trailer.Set(raw.NameLiteral("Root"), raw.Ref(catalogRef.Num, catalogRef.Gen))
	if infoRef != nil {
		trailer.Set(raw.NameLiteral("Info"), raw.Ref(infoRef.Num, infoRef.Gen))
	}
	if encryptRef != nil {
		trailer.Set(raw.NameLiteral("Encrypt"), raw.Ref(encryptRef.Num, encryptRef.Gen))
	}
	idArr := raw.NewArray(
		raw.HexStr(ids[0]),
		raw.HexStr(ids[1]),
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
	if xo.Interpolate {
		h.Write([]byte{1})
	}
	if xo.SMask != nil {
		h.Write([]byte(xoKey(name+":SMask", *xo.SMask)))
	}
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

func fontKey(base, encoding, subtype string, font *semantic.Font) string {
	h := sha256.New()
	h.Write([]byte(base))
	h.Write([]byte(encoding))
	h.Write([]byte(subtype))
	if font != nil {
		keys := make([]int, 0, len(font.Widths))
		for k := range font.Widths {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			h.Write([]byte(fmt.Sprintf("%d:%d", k, font.Widths[k])))
		}
		if font.DescendantFont != nil {
			keys := make([]int, 0, len(font.DescendantFont.W))
			for k := range font.DescendantFont.W {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				h.Write([]byte(fmt.Sprintf("%d:%d", k, font.DescendantFont.W[k])))
			}
			h.Write([]byte(font.DescendantFont.Subtype))
			h.Write([]byte(font.DescendantFont.BaseFont))
		}
		if font.CIDSystemInfo != nil {
			h.Write([]byte(font.CIDSystemInfo.Registry))
			h.Write([]byte(font.CIDSystemInfo.Ordering))
			h.Write([]byte(fmt.Sprint(font.CIDSystemInfo.Supplement)))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func encodeWidths(widths map[int]int) (first, last int, arr *raw.ArrayObj) {
	if len(widths) == 0 {
		return 0, 0, raw.NewArray()
	}
	first = math.MaxInt32
	last = -1
	for k := range widths {
		if k < first {
			first = k
		}
		if k > last {
			last = k
		}
	}
	arr = raw.NewArray()
	for i := first; i <= last; i++ {
		if w, ok := widths[i]; ok {
			arr.Append(raw.NumberInt(int64(w)))
		} else {
			arr.Append(raw.NumberInt(0))
		}
	}
	return first, last, arr
}

func encodeCIDWidths(widths map[int]int) *raw.ArrayObj {
	arr := raw.NewArray()
	if len(widths) == 0 {
		return arr
	}
	codes := make([]int, 0, len(widths))
	for c := range widths {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	start := codes[0]
	prev := codes[0]
	current := widths[codes[0]]
	for i := 1; i < len(codes); i++ {
		code := codes[i]
		w := widths[code]
		if w == current && code == prev+1 {
			prev = code
			continue
		}
		arr.Append(raw.NumberInt(int64(start)))
		arr.Append(raw.NumberInt(int64(prev)))
		arr.Append(raw.NumberInt(int64(current)))
		start = code
		prev = code
		current = w
	}
	arr.Append(raw.NumberInt(int64(start)))
	arr.Append(raw.NumberInt(int64(prev)))
	arr.Append(raw.NumberInt(int64(current)))
	return arr
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

func encryptObject(obj raw.Object, ref raw.ObjectRef, handler security.Handler, metadataRef *raw.ObjectRef, encryptMetadata bool) raw.Object {
	if handler == nil {
		return obj
	}
	if metadataRef != nil && !encryptMetadata && ref == *metadataRef {
		return obj
	}
	switch v := obj.(type) {
	case raw.StringObj:
		encrypted, err := handler.Encrypt(ref.Num, ref.Gen, v.Value(), security.DataClassString)
		if err != nil {
			return obj
		}
		return raw.Str(encrypted)
	case raw.HexStringObj:
		encrypted, err := handler.Encrypt(ref.Num, ref.Gen, v.Value(), security.DataClassString)
		if err != nil {
			return obj
		}
		return raw.HexStr(encrypted)
	case *raw.ArrayObj:
		arr := raw.NewArray()
		for _, item := range v.Items {
			arr.Append(encryptObject(item, ref, handler, metadataRef, encryptMetadata))
		}
		return arr
	case *raw.DictObj:
		d := raw.Dict()
		for k, val := range v.KV {
			d.Set(raw.NameLiteral(k), encryptObject(val, ref, handler, metadataRef, encryptMetadata))
		}
		return d
	case *raw.StreamObj:
		class := security.DataClassStream
		if metadataRef != nil && ref == *metadataRef {
			class = security.DataClassMetadataStream
		}
		data, err := handler.Encrypt(ref.Num, ref.Gen, v.Data, class)
		if err != nil {
			return obj
		}
		dictEncrypted := encryptObject(v.Dict, ref, handler, metadataRef, encryptMetadata)
		if dd, ok := dictEncrypted.(*raw.DictObj); ok {
			return raw.NewStream(dd, data)
		}
		return raw.NewStream(v.Dict, data)
	default:
		return obj
	}
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
