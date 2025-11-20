package writer

import (
	"fmt"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/security"
	"sort"
	"strings"
)

type objectBuilder struct {
	doc     *semantic.Document
	cfg     Config
	objects map[raw.ObjectRef]raw.Object
	objNum  int

	fontRefs    map[string]raw.ObjectRef
	xobjectRefs map[string]raw.ObjectRef
	patternRefs map[string]raw.ObjectRef
	shadingRefs map[string]raw.ObjectRef
}

func newObjectBuilder(doc *semantic.Document, cfg Config, startObjNum int) *objectBuilder {
	return &objectBuilder{
		doc:         doc,
		cfg:         cfg,
		objects:     make(map[raw.ObjectRef]raw.Object),
		objNum:      startObjNum,
		fontRefs:    make(map[string]raw.ObjectRef),
		xobjectRefs: make(map[string]raw.ObjectRef),
		patternRefs: make(map[string]raw.ObjectRef),
		shadingRefs: make(map[string]raw.ObjectRef),
	}
}

func (b *objectBuilder) nextRef() raw.ObjectRef {
	ref := raw.ObjectRef{Num: b.objNum, Gen: 0}
	b.objNum++
	return ref
}

func (b *objectBuilder) Build() (map[raw.ObjectRef]raw.Object, raw.ObjectRef, *raw.ObjectRef, *raw.ObjectRef, error) {
	catalogRef := b.nextRef()
	pagesRef := b.nextRef()
	pageRefs := make([]raw.ObjectRef, 0, len(b.doc.Pages))
	pageDicts := make([]*raw.DictObj, len(b.doc.Pages))

	// Document info dictionary
	var infoRef *raw.ObjectRef
	if b.doc.Info != nil {
		infoDict := raw.Dict()
		if b.doc.Info.Title != "" {
			infoDict.Set(raw.NameLiteral("Title"), raw.Str([]byte(b.doc.Info.Title)))
		}
		if b.doc.Info.Author != "" {
			infoDict.Set(raw.NameLiteral("Author"), raw.Str([]byte(b.doc.Info.Author)))
		}
		if b.doc.Info.Subject != "" {
			infoDict.Set(raw.NameLiteral("Subject"), raw.Str([]byte(b.doc.Info.Subject)))
		}
		if b.doc.Info.Creator != "" {
			infoDict.Set(raw.NameLiteral("Creator"), raw.Str([]byte(b.doc.Info.Creator)))
		}
		if b.doc.Info.Producer != "" {
			infoDict.Set(raw.NameLiteral("Producer"), raw.Str([]byte(b.doc.Info.Producer)))
		}
		if len(b.doc.Info.Keywords) > 0 {
			infoDict.Set(raw.NameLiteral("Keywords"), raw.Str([]byte(strings.Join(b.doc.Info.Keywords, ","))))
		}
		if infoDict.Len() > 0 {
			ref := b.nextRef()
			infoRef = &ref
			b.objects[ref] = infoDict
		}
	}

	// XMP metadata stream reference
	var metadataRef *raw.ObjectRef
	if b.doc.Metadata != nil && len(b.doc.Metadata.Raw) > 0 {
		ref := b.nextRef()
		metadataRef = &ref
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Metadata"))
		dict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("XML"))
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(b.doc.Metadata.Raw))))
		b.objects[ref] = raw.NewStream(dict, b.doc.Metadata.Raw)
	}

	// Encrypt dictionary (Standard handler)
	var encryptRef *raw.ObjectRef
	var encryptionHandler security.Handler
	if b.doc.Encrypted {
		ref := b.nextRef()
		encryptRef = &ref
		idPair := fileID(b.doc, b.cfg) // Helper from writer_impl.go, need to move or export
		enc, _, err := security.BuildStandardEncryption(b.doc.UserPassword, b.doc.OwnerPassword, b.doc.Permissions, idPair[0], b.doc.MetadataEncrypted)
		if err != nil {
			return nil, raw.ObjectRef{}, nil, nil, err
		}
		handler, err := (&security.HandlerBuilder{}).WithEncryptDict(enc).WithFileID(idPair[0]).Build()
		if err != nil {
			return nil, raw.ObjectRef{}, nil, nil, err
		}
		if err := handler.Authenticate(b.doc.UserPassword); err != nil {
			return nil, raw.ObjectRef{}, nil, nil, err
		}
		encryptionHandler = handler
		b.objects[ref] = enc
	}

	// OutputIntents
	var outputIntentRefs []raw.ObjectRef
	for _, oi := range b.doc.OutputIntents {
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
			pr := b.nextRef()
			profileRef = &pr
			pd := raw.Dict()
			pd.Set(raw.NameLiteral("N"), raw.NumberInt(3))
			pd.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(oi.DestOutputProfile))))
			b.objects[pr] = raw.NewStream(pd, oi.DestOutputProfile)
		}
		if profileRef != nil {
			dict.Set(raw.NameLiteral("DestOutputProfile"), raw.Ref(profileRef.Num, profileRef.Gen))
		}
		ref := b.nextRef()
		b.objects[ref] = dict
		outputIntentRefs = append(outputIntentRefs, ref)
	}

	var embeddedFilesDict *raw.DictObj
	var afFileSpecRefs []raw.ObjectRef
	if len(b.doc.EmbeddedFiles) > 0 {
		namesArr := raw.NewArray()
		for _, ef := range b.doc.EmbeddedFiles {
			if len(ef.Data) == 0 || ef.Name == "" {
				continue
			}
			streamDict := raw.Dict()
			streamDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("EmbeddedFile"))
			if ef.Subtype != "" {
				streamDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(pdfNameLiteral(ef.Subtype)))
			}
			streamDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(ef.Data))))
			params := raw.Dict()
			params.Set(raw.NameLiteral("Size"), raw.NumberInt(int64(len(ef.Data))))
			if params.Len() > 0 {
				streamDict.Set(raw.NameLiteral("Params"), params)
			}
			streamRef := b.nextRef()
			b.objects[streamRef] = raw.NewStream(streamDict, ef.Data)

			fileSpec := raw.Dict()
			fileSpec.Set(raw.NameLiteral("Type"), raw.NameLiteral("Filespec"))
			fileSpec.Set(raw.NameLiteral("F"), raw.Str([]byte(ef.Name)))
			fileSpec.Set(raw.NameLiteral("UF"), raw.Str([]byte(ef.Name)))
			if ef.Description != "" {
				fileSpec.Set(raw.NameLiteral("Desc"), raw.Str([]byte(ef.Description)))
			}
			afRel := ef.Relationship
			if afRel == "" {
				afRel = "Unspecified"
			}
			fileSpec.Set(raw.NameLiteral("AFRelationship"), raw.NameLiteral(afRel))
			refDict := raw.Dict()
			refDict.Set(raw.NameLiteral("F"), raw.Ref(streamRef.Num, streamRef.Gen))
			fileSpec.Set(raw.NameLiteral("EF"), refDict)
			fileSpecRef := b.nextRef()
			b.objects[fileSpecRef] = fileSpec

			namesArr.Append(raw.Str([]byte(ef.Name)))
			namesArr.Append(raw.Ref(fileSpecRef.Num, fileSpecRef.Gen))
			afFileSpecRefs = append(afFileSpecRefs, fileSpecRef)
		}
		if namesArr.Len() > 0 {
			dict := raw.Dict()
			dict.Set(raw.NameLiteral("Names"), namesArr)
			embeddedFilesDict = dict
		}
	}

	// Page content streams
	contentRefs := []raw.ObjectRef{}
	annotationRefs := make([][]raw.ObjectRef, len(b.doc.Pages))
	for _, p := range b.doc.Pages {
		contentData := []byte{}
		for _, cs := range p.Contents {
			contentData = append(contentData, serializeContentStream(cs)...)
		}
		streamData := contentData
		contentRef := b.nextRef()
		dict := raw.Dict()
		switch filter := pickContentFilter(b.cfg); filter {
		case FilterFlate:
			data, err := flateEncode(streamData, b.cfg.Compression)
			if err != nil {
				return nil, raw.ObjectRef{}, nil, nil, err
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
				return nil, raw.ObjectRef{}, nil, nil, err
			}
			streamData = data
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("LZWDecode"))
		case FilterJPX:
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("JPXDecode"))
		case FilterJBIG2:
			dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("JBIG2Decode"))
		}
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(streamData))))
		b.objects[contentRef] = raw.NewStream(dict, streamData)
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
	for i, p := range b.doc.Pages {
		ref := b.nextRef()
		pageRefs = append(pageRefs, ref)
		p.MediaBox = semantic.Rectangle{LLX: 0, LLY: 0, URX: p.MediaBox.URX, URY: p.MediaBox.URY}
		pageDict := raw.Dict()
		pageDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
		pageDict.Set(raw.NameLiteral("Parent"), raw.Ref(pagesRef.Num, pagesRef.Gen))
		pageDicts[i] = pageDict
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
				fRef := b.ensureFont(font)
				fontResDict.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
				unionFonts.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
			}
		} else {
			fRef := b.ensureFont(nil)
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
				val := cs.ColorSpaceName()
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
				xref := b.ensureXObject(name, xo)
				xDict.Set(raw.NameLiteral(name), raw.Ref(xref.Num, xref.Gen))
				if _, ok := unionXObjects.KV[name]; !ok {
					unionXObjects.Set(raw.NameLiteral(name), raw.Ref(xref.Num, xref.Gen))
				}
				if xo.Subtype == "Image" || xo.Subtype == "" {
					if xo.ColorSpace != nil && xo.ColorSpace.ColorSpaceName() == "DeviceGray" {
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
				pRef := b.ensurePattern(name, pat)
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
				shRef := b.ensureShading(name, sh)
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
				aRef := b.nextRef()
				aDict := raw.Dict()
				aDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Annot"))

				base := a.Base()
				subtype := base.Subtype
				if subtype == "" {
					subtype = "Link"
				}
				aDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(subtype))
				rect := base.RectVal
				if !cropSet(rect) {
					// fall back to crop/media box coordinates
					if cropSet(p.CropBox) {
						rect = p.CropBox
					} else {
						rect = p.MediaBox
					}
				}
				aDict.Set(raw.NameLiteral("Rect"), rectArray(rect))

				if link, ok := a.(*semantic.LinkAnnotation); ok {
					if link.Action != nil {
						if act := b.serializeAction(link.Action, pageRefs); act != nil {
							aDict.Set(raw.NameLiteral("A"), act)
						}
					} else if link.URI != "" {
						// Fallback for legacy URI field
						action := raw.Dict()
						action.Set(raw.NameLiteral("S"), raw.NameLiteral("URI"))
						action.Set(raw.NameLiteral("URI"), raw.Str([]byte(link.URI)))
						aDict.Set(raw.NameLiteral("A"), action)
					}
				} else if base.Contents != "" {
					aDict.Set(raw.NameLiteral("Contents"), raw.Str([]byte(base.Contents)))
				}
				if len(base.Appearance) > 0 {
					apRef := b.nextRef()
					apDict := raw.Dict()
					apDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(base.Appearance))))
					apStream := raw.NewStream(apDict, base.Appearance)
					b.objects[apRef] = apStream
					ap := raw.Dict()
					ap.Set(raw.NameLiteral("N"), raw.Ref(apRef.Num, apRef.Gen))
					aDict.Set(raw.NameLiteral("AP"), ap)
				}
				if base.Flags != 0 {
					aDict.Set(raw.NameLiteral("F"), raw.NumberInt(int64(base.Flags)))
				}
				if len(base.Border) == 3 {
					aDict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberFloat(base.Border[0]), raw.NumberFloat(base.Border[1]), raw.NumberFloat(base.Border[2])))
				} else {
					aDict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(0)))
				}
				if len(base.Color) > 0 {
					colArr := raw.NewArray()
					for _, c := range base.Color {
						colArr.Append(raw.NumberFloat(c))
					}
					aDict.Set(raw.NameLiteral("C"), colArr)
				}
				if base.AppearanceState != "" {
					aDict.Set(raw.NameLiteral("AS"), raw.NameLiteral(base.AppearanceState))
				}
				b.objects[aRef] = aDict
				annotArr.Append(raw.Ref(aRef.Num, aRef.Gen))
				annotationRefs[i] = append(annotationRefs[i], aRef)
			}
			pageDict.Set(raw.NameLiteral("Annots"), annotArr)
		}
		b.objects[ref] = pageDict
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
	b.objects[pagesRef] = pagesDict
	var structRootRef *raw.ObjectRef
	var parentTreeRef *raw.ObjectRef
	var parentTree map[int]map[int]raw.ObjectRef
	if b.doc.StructTree != nil {
		structRootRef, parentTreeRef, parentTree = buildStructureTree(b.doc.StructTree, pageRefs, b.nextRef, b.objects)
		for idx := range parentTree {
			if idx >= 0 && idx < len(pageDicts) {
				pageDicts[idx].Set(raw.NameLiteral("StructParents"), raw.NumberInt(int64(idx)))
			}
		}
	}
	// Catalog
	catalogDict := raw.Dict()
	catalogDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	catalogDict.Set(raw.NameLiteral("Pages"), raw.Ref(pagesRef.Num, pagesRef.Gen))
	if structRootRef != nil {
		catalogDict.Set(raw.NameLiteral("StructTreeRoot"), raw.Ref(structRootRef.Num, structRootRef.Gen))
	}
	if parentTreeRef != nil {
		catalogDict.Set(raw.NameLiteral("ParentTree"), raw.Ref(parentTreeRef.Num, parentTreeRef.Gen))
	}
	if b.doc.Lang != "" {
		catalogDict.Set(raw.NameLiteral("Lang"), raw.Str([]byte(b.doc.Lang)))
	}
	if b.doc.Marked || b.doc.StructTree != nil {
		mark := raw.Dict()
		mark.Set(raw.NameLiteral("Marked"), raw.Bool(true))
		catalogDict.Set(raw.NameLiteral("MarkInfo"), mark)
	}
	if metadataRef != nil {
		catalogDict.Set(raw.NameLiteral("Metadata"), raw.Ref(metadataRef.Num, metadataRef.Gen))
	}
	if b.doc.Info != nil && b.doc.Info.Title != "" {
		vp := raw.Dict()
		vp.Set(raw.NameLiteral("DisplayDocTitle"), raw.Bool(true))
		catalogDict.Set(raw.NameLiteral("ViewerPreferences"), vp)
	}
	if len(b.doc.PageLabels) > 0 {
		nums := raw.NewArray()
		indices := make([]int, 0, len(b.doc.PageLabels))
		for idx := range b.doc.PageLabels {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			nums.Append(raw.NumberInt(int64(idx)))
			entry := raw.Dict()
			entry.Set(raw.NameLiteral("P"), raw.Str([]byte(b.doc.PageLabels[idx])))
			nums.Append(entry)
		}
		labels := raw.Dict()
		labels.Set(raw.NameLiteral("Nums"), nums)
		catalogDict.Set(raw.NameLiteral("PageLabels"), labels)
	}
	if len(b.doc.Outlines) > 0 {
		outlineRef := b.nextRef()
		outlineDict := raw.Dict()
		outlineDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Outlines"))
		b.objects[outlineRef] = outlineDict

		first, last, total := b.buildOutlines(b.doc.Outlines, outlineRef, pageRefs, b.objects, b.nextRef, outlineRef)
		outlineDict.Set(raw.NameLiteral("First"), raw.Ref(first.Num, first.Gen))
		outlineDict.Set(raw.NameLiteral("Last"), raw.Ref(last.Num, last.Gen))
		outlineDict.Set(raw.NameLiteral("Count"), raw.NumberInt(total))
		catalogDict.Set(raw.NameLiteral("Outlines"), raw.Ref(outlineRef.Num, outlineRef.Gen))
		catalogDict.Set(raw.NameLiteral("PageMode"), raw.NameLiteral("UseOutlines"))
	}
	if len(b.doc.Articles) > 0 {
		threadArr := raw.NewArray()
		for _, art := range b.doc.Articles {
			if len(art.Beads) == 0 {
				continue
			}
			threadRef := b.nextRef()
			threadArr.Append(raw.Ref(threadRef.Num, threadRef.Gen))
			threadDict := raw.Dict()
			threadDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Thread"))
			if art.Title != "" {
				threadDict.Set(raw.NameLiteral("T"), raw.Str([]byte(art.Title)))
			}
			beadRefs := make([]raw.ObjectRef, len(art.Beads))
			for i := range art.Beads {
				beadRefs[i] = b.nextRef()
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
				b.objects[beadRefs[i]] = bd
			}
			threadDict.Set(raw.NameLiteral("K"), raw.Ref(beadRefs[0].Num, beadRefs[0].Gen))
			b.objects[threadRef] = threadDict
		}
		if threadArr.Len() > 0 {
			catalogDict.Set(raw.NameLiteral("Threads"), threadArr)
		}
	}
	if b.doc.AcroForm != nil {
		formRef := b.nextRef()
		formDict := raw.Dict()
		fieldsArr := raw.NewArray()
		appendWidgetToPage := func(pageIdx int, ref raw.ObjectRef) {
			if pageIdx < 0 || pageIdx >= len(pageRefs) {
				return
			}
			pref := pageRefs[pageIdx]
			pageObj := b.objects[pref]
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
		for _, f := range b.doc.AcroForm.Fields {
			fieldRef := b.nextRef()
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
				apRef := b.nextRef()
				apDict := raw.Dict()
				apDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(f.Appearance))))
				b.objects[apRef] = raw.NewStream(apDict, f.Appearance)
				ap := raw.Dict()
				ap.Set(raw.NameLiteral("N"), raw.Ref(apRef.Num, apRef.Gen))
				fd.Set(raw.NameLiteral("AP"), ap)
			}
			if f.AppearanceState != "" {
				fd.Set(raw.NameLiteral("AS"), raw.NameLiteral(f.AppearanceState))
			}
			b.objects[fieldRef] = fd
			fieldsArr.Append(raw.Ref(fieldRef.Num, fieldRef.Gen))
		}
		formDict.Set(raw.NameLiteral("Fields"), fieldsArr)
		if b.doc.AcroForm.NeedAppearances {
			formDict.Set(raw.NameLiteral("NeedAppearances"), raw.Bool(true))
		}
		b.objects[formRef] = formDict
		catalogDict.Set(raw.NameLiteral("AcroForm"), raw.Ref(formRef.Num, formRef.Gen))
	}
	if len(outputIntentRefs) > 0 {
		arr := raw.NewArray()
		for _, ref := range outputIntentRefs {
			arr.Append(raw.Ref(ref.Num, ref.Gen))
		}
		catalogDict.Set(raw.NameLiteral("OutputIntents"), arr)
	}
	if embeddedFilesDict != nil {
		if namesObj, ok := catalogDict.Get(raw.NameLiteral("Names")); ok {
			if namesDict, ok := namesObj.(*raw.DictObj); ok {
				namesDict.Set(raw.NameLiteral("EmbeddedFiles"), embeddedFilesDict)
			}
		} else {
			namesDict := raw.Dict()
			namesDict.Set(raw.NameLiteral("EmbeddedFiles"), embeddedFilesDict)
			catalogDict.Set(raw.NameLiteral("Names"), namesDict)
		}
	}
	if len(afFileSpecRefs) > 0 {
		afArr := raw.NewArray()
		for _, ref := range afFileSpecRefs {
			afArr.Append(raw.Ref(ref.Num, ref.Gen))
		}
		catalogDict.Set(raw.NameLiteral("AF"), afArr)
	}
	b.objects[catalogRef] = catalogDict

	if encryptionHandler != nil {
		for ref, obj := range b.objects {
			if encryptRef != nil && ref == *encryptRef {
				continue
			}
			if metadataRef != nil && ref == *metadataRef && !b.doc.MetadataEncrypted {
				continue
			}
			b.objects[ref] = encryptObject(obj, ref, encryptionHandler, metadataRef, b.doc.MetadataEncrypted)
		}
	}

	return b.objects, catalogRef, infoRef, encryptRef, nil
}

func (b *objectBuilder) addFontDescriptor(fd *semantic.FontDescriptor) *raw.ObjectRef {
	if fd == nil {
		return nil
	}
	ref := b.nextRef()
	d := raw.Dict()
	d.Set(raw.NameLiteral("Type"), raw.NameLiteral("FontDescriptor"))
	name := fd.FontName
	if name == "" {
		name = "CustomFont"
	}
	d.Set(raw.NameLiteral("FontName"), raw.NameLiteral(name))
	flags := fd.Flags
	if flags == 0 {
		flags = 4
	}
	d.Set(raw.NameLiteral("Flags"), raw.NumberInt(int64(flags)))
	d.Set(raw.NameLiteral("ItalicAngle"), raw.NumberFloat(fd.ItalicAngle))
	d.Set(raw.NameLiteral("Ascent"), raw.NumberFloat(fd.Ascent))
	d.Set(raw.NameLiteral("Descent"), raw.NumberFloat(fd.Descent))
	d.Set(raw.NameLiteral("CapHeight"), raw.NumberFloat(fd.CapHeight))
	stem := fd.StemV
	if stem == 0 {
		stem = 80
	}
	d.Set(raw.NameLiteral("StemV"), raw.NumberInt(int64(stem)))
	bbox := raw.NewArray(
		raw.NumberFloat(fd.FontBBox[0]),
		raw.NumberFloat(fd.FontBBox[1]),
		raw.NumberFloat(fd.FontBBox[2]),
		raw.NumberFloat(fd.FontBBox[3]),
	)
	d.Set(raw.NameLiteral("FontBBox"), bbox)
	if len(fd.FontFile) > 0 {
		streamDict := raw.Dict()
		streamDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(fd.FontFile))))
		streamRef := b.nextRef()
		b.objects[streamRef] = raw.NewStream(streamDict, fd.FontFile)
		key := "FontFile2"
		if fd.FontFileType != "" {
			key = fd.FontFileType
		}
		d.Set(raw.NameLiteral(key), raw.Ref(streamRef.Num, streamRef.Gen))
	}
	b.objects[ref] = d
	return &ref
}

func (b *objectBuilder) addToUnicode(font *semantic.Font) *raw.ObjectRef {
	if font == nil || len(font.ToUnicode) == 0 {
		return nil
	}
	cmap := buildToUnicodeCMap(font)
	if len(cmap) == 0 {
		return nil
	}
	ref := b.nextRef()
	d := raw.Dict()
	d.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(cmap))))
	b.objects[ref] = raw.NewStream(d, cmap)
	return &ref
}

func (b *objectBuilder) ensureFont(font *semantic.Font) raw.ObjectRef {
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
	if ref, ok := b.fontRefs[key]; ok {
		return ref
	}
	ref := b.nextRef()
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

		descRef := b.nextRef()
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
		if fd := b.addFontDescriptor(fontDescriptor(desc, font)); fd != nil {
			descDict.Set(raw.NameLiteral("FontDescriptor"), raw.Ref(fd.Num, fd.Gen))
		}
		b.objects[descRef] = descDict
		fontDict.Set(raw.NameLiteral("DescendantFonts"), raw.NewArray(raw.Ref(descRef.Num, descRef.Gen)))
		if uref := b.addToUnicode(font); uref != nil {
			fontDict.Set(raw.NameLiteral("ToUnicode"), raw.Ref(uref.Num, uref.Gen))
		}
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
		if fd := b.addFontDescriptor(fontDescriptor(nil, font)); fd != nil {
			fontDict.Set(raw.NameLiteral("FontDescriptor"), raw.Ref(fd.Num, fd.Gen))
		}
	}
	b.objects[ref] = fontDict
	b.fontRefs[key] = ref
	return ref
}

func (b *objectBuilder) ensureXObject(name string, xo semantic.XObject) raw.ObjectRef {
	key := xoKey(name, xo)
	if ref, ok := b.xobjectRefs[key]; ok {
		return ref
	}
	ref := b.nextRef()
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
		var color string
		if xo.ColorSpace != nil {
			color = xo.ColorSpace.ColorSpaceName()
		}
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
		maskRef := b.ensureXObject(maskName, *xo.SMask)
		dict.Set(raw.NameLiteral("SMask"), raw.Ref(maskRef.Num, maskRef.Gen))
	}
	dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(xo.Data))))
	b.objects[ref] = raw.NewStream(dict, xo.Data)
	b.xobjectRefs[key] = ref
	return ref
}

func (b *objectBuilder) ensurePattern(name string, p semantic.Pattern) raw.ObjectRef {
	key := patternKey(name, p)
	if ref, ok := b.patternRefs[key]; ok {
		return ref
	}
	ref := b.nextRef()
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
	b.objects[ref] = raw.NewStream(dict, content)
	b.patternRefs[key] = ref
	return ref
}

func (b *objectBuilder) ensureShading(name string, s semantic.Shading) raw.ObjectRef {
	key := shadingKey(name, s)
	if ref, ok := b.shadingRefs[key]; ok {
		return ref
	}
	ref := b.nextRef()
	dict := raw.Dict()
	stype := s.ShadingType
	if stype == 0 {
		stype = 2
	}
	dict.Set(raw.NameLiteral("ShadingType"), raw.NumberInt(int64(stype)))
	var cs string
	if s.ColorSpace != nil {
		cs = s.ColorSpace.ColorSpaceName()
	}
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
		b.objects[ref] = raw.NewStream(dict, content)
	} else {
		b.objects[ref] = dict
	}
	b.shadingRefs[key] = ref
	return ref
}

func (b *objectBuilder) buildOutlines(items []semantic.OutlineItem, parent raw.ObjectRef, pageRefs []raw.ObjectRef, objects map[raw.ObjectRef]raw.Object, nextRef func() raw.ObjectRef, root raw.ObjectRef) (first, last raw.ObjectRef, count int64) {
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
			firstChild, lastChild, childCount := b.buildOutlines(item.Children, refs[i], pageRefs, objects, nextRef, root)
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

func (b *objectBuilder) serializeAction(a semantic.Action, pageRefs []raw.ObjectRef) raw.Object {
	if a == nil {
		return nil
	}
	d := raw.Dict()
	switch act := a.(type) {
	case semantic.URIAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("URI"))
		d.Set(raw.NameLiteral("URI"), raw.Str([]byte(act.URI)))
	case semantic.GoToAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("GoTo"))
		if act.PageIndex >= 0 && act.PageIndex < len(pageRefs) {
			pref := pageRefs[act.PageIndex]
			var dest raw.Object
			if act.Dest != nil {
				dest = raw.NewArray(
					raw.Ref(pref.Num, pref.Gen),
					raw.NameLiteral("XYZ"),
					xyzDestValue(act.Dest.X),
					xyzDestValue(act.Dest.Y),
					xyzDestValue(act.Dest.Zoom),
				)
			} else {
				dest = raw.NewArray(raw.Ref(pref.Num, pref.Gen), raw.NameLiteral("Fit"))
			}
			d.Set(raw.NameLiteral("D"), dest)
		}
	}
	return d
}
