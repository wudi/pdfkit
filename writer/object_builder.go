package writer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/security"
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

	pageRefs []raw.ObjectRef

	annotSerializer  AnnotationSerializer
	actionSerializer ActionSerializer
	csSerializer     ColorSpaceSerializer
	funcSerializer   FunctionSerializer
}

func newObjectBuilder(doc *semantic.Document, cfg Config, startObjNum int, as AnnotationSerializer, actS ActionSerializer, csS ColorSpaceSerializer, fs FunctionSerializer) *objectBuilder {
	b := &objectBuilder{
		doc:         doc,
		cfg:         cfg,
		objects:     make(map[raw.ObjectRef]raw.Object),
		objNum:      startObjNum,
		fontRefs:    make(map[string]raw.ObjectRef),
		xobjectRefs: make(map[string]raw.ObjectRef),
		patternRefs: make(map[string]raw.ObjectRef),
		shadingRefs: make(map[string]raw.ObjectRef),
	}
	if actS != nil {
		b.actionSerializer = actS
	} else {
		b.actionSerializer = newActionSerializer()
	}
	if as != nil {
		b.annotSerializer = as
	} else {
		b.annotSerializer = newAnnotationSerializer(b.actionSerializer)
	}
	if fs != nil {
		b.funcSerializer = fs
	} else {
		b.funcSerializer = newFunctionSerializer()
	}
	if csS != nil {
		b.csSerializer = csS
	} else {
		b.csSerializer = newColorSpaceSerializer(b.funcSerializer)
	}
	return b
}

func (b *objectBuilder) NextRef() raw.ObjectRef {
	return b.nextRef()
}

func (b *objectBuilder) AddObject(ref raw.ObjectRef, obj raw.Object) {
	b.objects[ref] = obj
}

func (b *objectBuilder) PageRef(index int) *raw.ObjectRef {
	if index >= 0 && index < len(b.pageRefs) {
		return &b.pageRefs[index]
	}
	return nil
}

func (b *objectBuilder) nextRef() raw.ObjectRef {
	ref := raw.ObjectRef{Num: b.objNum, Gen: 0}
	b.objNum++
	return ref
}

func (b *objectBuilder) Build() (map[raw.ObjectRef]raw.Object, raw.ObjectRef, *raw.ObjectRef, *raw.ObjectRef, error) {
	catalogRef := b.nextRef()
	pagesRef := b.nextRef()
	b.pageRefs = make([]raw.ObjectRef, 0, len(b.doc.Pages))
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
	unionProperties := raw.Dict()
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
		b.pageRefs = append(b.pageRefs, ref)
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
		if p.Trans != nil {
			transDict := raw.Dict()
			if p.Trans.Style != "" {
				transDict.Set(raw.NameLiteral("S"), raw.NameLiteral(p.Trans.Style))
			}
			if p.Trans.Duration != nil {
				transDict.Set(raw.NameLiteral("D"), raw.NumberFloat(*p.Trans.Duration))
			}
			if p.Trans.Dimension != "" {
				transDict.Set(raw.NameLiteral("Dm"), raw.NameLiteral(p.Trans.Dimension))
			}
			if p.Trans.Motion != "" {
				transDict.Set(raw.NameLiteral("M"), raw.NameLiteral(p.Trans.Motion))
			}
			if p.Trans.Direction != 0 {
				transDict.Set(raw.NameLiteral("Di"), raw.NumberInt(int64(p.Trans.Direction)))
			}
			if p.Trans.Scale != nil {
				transDict.Set(raw.NameLiteral("SS"), raw.NumberFloat(*p.Trans.Scale))
			}
			if p.Trans.Base != nil {
				transDict.Set(raw.NameLiteral("B"), raw.Bool(*p.Trans.Base))
			}
			pageDict.Set(raw.NameLiteral("Trans"), transDict)
		}
		// Viewports
		if len(p.Viewports) > 0 {
			vpArr := raw.NewArray()
			for _, vp := range p.Viewports {
				vpDict := raw.Dict()
				vpDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Viewport"))
				if len(vp.BBox) == 4 {
					vpDict.Set(raw.NameLiteral("BBox"), raw.NewArray(
						raw.NumberFloat(vp.BBox[0]),
						raw.NumberFloat(vp.BBox[1]),
						raw.NumberFloat(vp.BBox[2]),
						raw.NumberFloat(vp.BBox[3]),
					))
				}
				if vp.Name != "" {
					vpDict.Set(raw.NameLiteral("Name"), raw.Str([]byte(vp.Name)))
				}
				if vp.Measure != nil {
					mDict := raw.Dict()
					mDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Measure"))
					if vp.Measure.Subtype != "" {
						mDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(vp.Measure.Subtype))
					}
					if len(vp.Measure.Bounds) > 0 {
						arr := raw.NewArray()
						for _, v := range vp.Measure.Bounds {
							arr.Append(raw.NumberFloat(v))
						}
						mDict.Set(raw.NameLiteral("Bounds"), arr)
					}
					if len(vp.Measure.GPTS) > 0 {
						arr := raw.NewArray()
						for _, v := range vp.Measure.GPTS {
							arr.Append(raw.NumberFloat(v))
						}
						mDict.Set(raw.NameLiteral("GPTS"), arr)
					}
					if len(vp.Measure.LPTS) > 0 {
						arr := raw.NewArray()
						for _, v := range vp.Measure.LPTS {
							arr.Append(raw.NumberFloat(v))
						}
						mDict.Set(raw.NameLiteral("LPTS"), arr)
					}
					if vp.Measure.GCS != nil {
						gcsDict := raw.Dict()
						if vp.Measure.GCS.Type != "" {
							gcsDict.Set(raw.NameLiteral("Type"), raw.NameLiteral(vp.Measure.GCS.Type))
						}
						if vp.Measure.GCS.WKT != "" {
							gcsDict.Set(raw.NameLiteral("WKT"), raw.Str([]byte(vp.Measure.GCS.WKT)))
						}
						if vp.Measure.GCS.EPSG != 0 {
							gcsDict.Set(raw.NameLiteral("EPSG"), raw.NumberInt(int64(vp.Measure.GCS.EPSG)))
						}
						mDict.Set(raw.NameLiteral("GCS"), gcsDict)
					}
					vpDict.Set(raw.NameLiteral("Measure"), mDict)
				}
				vpArr.Append(vpDict)
			}
			pageDict.Set(raw.NameLiteral("VP"), vpArr)
		}
		// OutputIntents (Page Level)
		if len(p.OutputIntents) > 0 {
			arr := raw.NewArray()
			for _, oi := range p.OutputIntents {
				dict := raw.Dict()
				dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("OutputIntent"))
				if oi.S != "" {
					dict.Set(raw.NameLiteral("S"), raw.NameLiteral(oi.S))
				}
				if oi.OutputConditionIdentifier != "" {
					dict.Set(raw.NameLiteral("OutputConditionIdentifier"), raw.Str([]byte(oi.OutputConditionIdentifier)))
				}
				if oi.Info != "" {
					dict.Set(raw.NameLiteral("Info"), raw.Str([]byte(oi.Info)))
				}
				var profileRef *raw.ObjectRef
				if len(oi.DestOutputProfile) > 0 {
					pr := b.nextRef()
					profileRef = &pr
					pd := raw.Dict()
					pd.Set(raw.NameLiteral("N"), raw.NumberInt(3)) // Default to 3 components?
					pd.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(oi.DestOutputProfile))))
					b.objects[pr] = raw.NewStream(pd, oi.DestOutputProfile)
				}
				if profileRef != nil {
					dict.Set(raw.NameLiteral("DestOutputProfile"), raw.Ref(profileRef.Num, profileRef.Gen))
				}
				ref := b.nextRef()
				b.objects[ref] = dict
				arr.Append(raw.Ref(ref.Num, ref.Gen))
			}
			pageDict.Set(raw.NameLiteral("OutputIntents"), arr)
		}
		// Associated Files (Page Level)
		if len(p.AssociatedFiles) > 0 {
			if af := SerializeAssociatedFiles(p.AssociatedFiles, b); af != nil {
				pageDict.Set(raw.NameLiteral("AF"), af)
			}
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
				if gs.BlendMode != "" {
					entry.Set(raw.NameLiteral("BM"), raw.NameLiteral(gs.BlendMode))
				}
				if gs.AlphaSource != nil {
					entry.Set(raw.NameLiteral("AIS"), raw.Bool(*gs.AlphaSource))
				}
				if gs.TextKnockout != nil {
					entry.Set(raw.NameLiteral("TK"), raw.Bool(*gs.TextKnockout))
				}
				if gs.Overprint != nil {
					entry.Set(raw.NameLiteral("OP"), raw.Bool(*gs.Overprint))
				}
				if gs.OverprintFill != nil {
					entry.Set(raw.NameLiteral("op"), raw.Bool(*gs.OverprintFill))
				}
				if gs.OverprintMode != nil {
					entry.Set(raw.NameLiteral("OPM"), raw.NumberInt(int64(*gs.OverprintMode)))
				}
				if gs.UseBlackPtComp != nil {
					entry.Set(raw.NameLiteral("UseBlackPtComp"), raw.Bool(*gs.UseBlackPtComp))
				}
				if gs.SoftMask != nil {
					smDict := raw.Dict()
					smDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Mask"))
					smDict.Set(raw.NameLiteral("S"), raw.NameLiteral(gs.SoftMask.Subtype))
					if gs.SoftMask.Group != nil {
						gRef := b.ensureXObject("SMaskGroup", *gs.SoftMask.Group)
						smDict.Set(raw.NameLiteral("G"), raw.Ref(gRef.Num, gRef.Gen))
					}
					if len(gs.SoftMask.BackdropColor) > 0 {
						bc := raw.NewArray()
						for _, c := range gs.SoftMask.BackdropColor {
							bc.Append(raw.NumberFloat(c))
						}
						smDict.Set(raw.NameLiteral("BC"), bc)
					}
					if gs.SoftMask.Transfer != "" {
						smDict.Set(raw.NameLiteral("TR"), raw.NameLiteral(gs.SoftMask.Transfer))
					}
					entry.Set(raw.NameLiteral("SMask"), smDict)
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
				obj := b.csSerializer.Serialize(cs, b)
				csDict.Set(raw.NameLiteral(name), obj)
				if _, ok := unionColorSpaces.KV[name]; !ok {
					unionColorSpaces.Set(raw.NameLiteral(name), obj)
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
		if p.Resources != nil && len(p.Resources.Properties) > 0 {
			propDict := raw.Dict()
			for name, prop := range p.Resources.Properties {
				ref := b.ensurePropertyList(name, prop)
				propDict.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
				if _, ok := unionProperties.KV[name]; !ok {
					unionProperties.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
				}
			}
			if propDict.Len() > 0 {
				resDict.Set(raw.NameLiteral("Properties"), propDict)
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
				base := a.Base()
				if !cropSet(base.RectVal) {
					// fall back to crop/media box coordinates
					if cropSet(p.CropBox) {
						a.SetRect(p.CropBox)
					} else {
						a.SetRect(p.MediaBox)
					}
				}
				aRef, err := b.annotSerializer.Serialize(a, b)
				if err != nil {
					return nil, raw.ObjectRef{}, nil, nil, err
				}
				annotArr.Append(raw.Ref(aRef.Num, aRef.Gen))
				annotationRefs[i] = append(annotationRefs[i], aRef)
			}
			pageDict.Set(raw.NameLiteral("Annots"), annotArr)
		}
		b.objects[ref] = pageDict
	}
	// Pages tree
	kidsArr := raw.NewArray()
	for _, r := range b.pageRefs {
		kidsArr.Append(raw.Ref(r.Num, r.Gen))
	}
	pagesDict := raw.Dict()
	pagesDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pagesDict.Set(raw.NameLiteral("Count"), raw.NumberInt(int64(len(b.pageRefs))))
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
		if unionProperties.Len() > 0 {
			pagesRes.Set(raw.NameLiteral("Properties"), unionProperties)
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
		structRootRef, parentTreeRef, parentTree = buildStructureTree(b.doc.StructTree, b.pageRefs, b.nextRef, b.objects)
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

		first, last, total := b.buildOutlines(b.doc.Outlines, outlineRef, b.pageRefs, b.objects, b.nextRef, outlineRef)
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
				if bead.PageIndex >= 0 && bead.PageIndex < len(b.pageRefs) {
					pref := b.pageRefs[bead.PageIndex]
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
		fieldRefMap := make(map[semantic.FormField]raw.ObjectRef)

		appendWidgetToPage := func(pageIdx int, ref raw.ObjectRef) {
			if pageIdx < 0 || pageIdx >= len(b.pageRefs) {
				return
			}
			pref := b.pageRefs[pageIdx]
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
			// Convert FormField to WidgetAnnotation for serialization
			widget := &semantic.WidgetAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype:         "Widget",
					RectVal:         f.FieldRect(),
					Flags:           f.FieldFlags(), // Using Field flags as Annotation flags (legacy behavior)
					Appearance:      f.GetAppearance(),
					AppearanceState: f.GetAppearanceState(),
					Border:          f.GetBorder(),
					Color:           f.GetColor(),
				},
				Field: f,
			}

			fieldRef, err := b.annotSerializer.Serialize(widget, b)
			if err != nil {
				return nil, raw.ObjectRef{}, nil, nil, err
			}
			fieldRefMap[f] = fieldRef

			// Post-processing for P (Page) reference which isn't handled by serializer
			if f.FieldPageIndex() >= 0 && f.FieldPageIndex() < len(b.pageRefs) {
				pref := b.pageRefs[f.FieldPageIndex()]
				if obj, ok := b.objects[fieldRef]; ok {
					if dict, ok := obj.(*raw.DictObj); ok {
						dict.Set(raw.NameLiteral("P"), raw.Ref(pref.Num, pref.Gen))
					}
				}
				appendWidgetToPage(f.FieldPageIndex(), fieldRef)
			}

			fieldsArr.Append(raw.Ref(fieldRef.Num, fieldRef.Gen))
		}
		formDict.Set(raw.NameLiteral("Fields"), fieldsArr)
		if b.doc.AcroForm.NeedAppearances {
			formDict.Set(raw.NameLiteral("NeedAppearances"), raw.Bool(true))
		}
		if len(b.doc.AcroForm.CalculationOrder) > 0 {
			coArr := raw.NewArray()
			for _, f := range b.doc.AcroForm.CalculationOrder {
				if ref, ok := fieldRefMap[f]; ok {
					coArr.Append(raw.Ref(ref.Num, ref.Gen))
				}
			}
			if coArr.Len() > 0 {
				formDict.Set(raw.NameLiteral("CO"), coArr)
			}
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
		if fd.Length1 > 0 {
			streamDict.Set(raw.NameLiteral("Length1"), raw.NumberInt(int64(fd.Length1)))
		}
		if fd.Length2 > 0 {
			streamDict.Set(raw.NameLiteral("Length2"), raw.NumberInt(int64(fd.Length2)))
		}
		if fd.Length3 > 0 {
			streamDict.Set(raw.NameLiteral("Length3"), raw.NumberInt(int64(fd.Length3)))
		}
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
	} else if subtype == "Type3" {
		if len(font.FontMatrix) > 0 {
			arr := raw.NewArray()
			for _, v := range font.FontMatrix {
				arr.Append(raw.NumberFloat(v))
			}
			fontDict.Set(raw.NameLiteral("FontMatrix"), arr)
		}
		if len(font.CharProcs) > 0 {
			cpDict := raw.Dict()
			for name, stream := range font.CharProcs {
				sDict := raw.Dict()
				sDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(stream))))
				sRef := b.nextRef()
				b.objects[sRef] = raw.NewStream(sDict, stream)
				cpDict.Set(raw.NameLiteral(name), raw.Ref(sRef.Num, sRef.Gen))
			}
			fontDict.Set(raw.NameLiteral("CharProcs"), cpDict)
		}
		if cropSet(font.FontBBox) {
			fontDict.Set(raw.NameLiteral("FontBBox"), rectArray(font.FontBBox))
		}
		if font.Resources != nil {
			if resDict := b.serializeResources(font.Resources); resDict != nil {
				fontDict.Set(raw.NameLiteral("Resources"), resDict)
			}
		}
		if len(font.Widths) > 0 {
			first, last, widthsArr := encodeWidths(font.Widths)
			fontDict.Set(raw.NameLiteral("FirstChar"), raw.NumberInt(int64(first)))
			fontDict.Set(raw.NameLiteral("LastChar"), raw.NumberInt(int64(last)))
			fontDict.Set(raw.NameLiteral("Widths"), widthsArr)
		}
		if encoding != "" {
			fontDict.Set(raw.NameLiteral("Encoding"), raw.NameLiteral(encoding))
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
		dict.Set(raw.NameLiteral("ColorSpace"), b.csSerializer.Serialize(xo.ColorSpace, b))
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
	if sub == "Form" && xo.Group != nil {
		gDict := raw.Dict()
		gDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Group"))
		gDict.Set(raw.NameLiteral("S"), raw.NameLiteral("Transparency"))
		if xo.Group.CS != nil {
			gDict.Set(raw.NameLiteral("CS"), b.csSerializer.Serialize(xo.Group.CS, b))
		}
		if xo.Group.Isolated {
			gDict.Set(raw.NameLiteral("I"), raw.Bool(true))
		}
		if xo.Group.Knockout {
			gDict.Set(raw.NameLiteral("K"), raw.Bool(true))
		}
		dict.Set(raw.NameLiteral("Group"), gDict)
	}
	if xo.SMask != nil {
		maskName := fmt.Sprintf("%s:SMask", name)
		maskRef := b.ensureXObject(maskName, *xo.SMask)
		dict.Set(raw.NameLiteral("SMask"), raw.Ref(maskRef.Num, maskRef.Gen))
	}
	if len(xo.AssociatedFiles) > 0 {
		if af := SerializeAssociatedFiles(xo.AssociatedFiles, b); af != nil {
			dict.Set(raw.NameLiteral("AF"), af)
		}
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
	pt := p.PatternType()
	if pt == 0 {
		pt = 1
	}
	dict.Set(raw.NameLiteral("PatternType"), raw.NumberInt(int64(pt)))

	switch pat := p.(type) {
	case *semantic.TilingPattern:
		paint := pat.PaintType
		if paint == 0 {
			paint = 1
		}
		dict.Set(raw.NameLiteral("PaintType"), raw.NumberInt(int64(paint)))
		tiling := pat.TilingType
		if tiling == 0 {
			tiling = 1
		}
		dict.Set(raw.NameLiteral("TilingType"), raw.NumberInt(int64(tiling)))
		if cropSet(pat.BBox) {
			dict.Set(raw.NameLiteral("BBox"), rectArray(pat.BBox))
		}
		if pat.XStep > 0 {
			dict.Set(raw.NameLiteral("XStep"), raw.NumberFloat(pat.XStep))
		}
		if pat.YStep > 0 {
			dict.Set(raw.NameLiteral("YStep"), raw.NumberFloat(pat.YStep))
		}
		if pat.Resources != nil {
			if resDict := b.serializeResources(pat.Resources); resDict != nil {
				dict.Set(raw.NameLiteral("Resources"), resDict)
			}
		}
		content := pat.Content
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(content))))
		b.objects[ref] = raw.NewStream(dict, content)

	case *semantic.ShadingPattern:
		if pat.Shading != nil {
			shRef := b.ensureShading(name+":Shading", pat.Shading)
			dict.Set(raw.NameLiteral("Shading"), raw.Ref(shRef.Num, shRef.Gen))
		}
		b.objects[ref] = dict
	}

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
	stype := s.ShadingType()
	if stype == 0 {
		stype = 2
	}
	dict.Set(raw.NameLiteral("ShadingType"), raw.NumberInt(int64(stype)))
	dict.Set(raw.NameLiteral("ColorSpace"), b.csSerializer.Serialize(s.ShadingColorSpace(), b))

	switch sh := s.(type) {
	case *semantic.FunctionShading:
		if cropSet(sh.BBox) {
			dict.Set(raw.NameLiteral("BBox"), rectArray(sh.BBox))
		}
		if sh.AntiAlias {
			dict.Set(raw.NameLiteral("AntiAlias"), raw.Bool(true))
		}
		if len(sh.Coords) > 0 {
			arr := raw.NewArray()
			for _, c := range sh.Coords {
				arr.Append(raw.NumberFloat(c))
			}
			dict.Set(raw.NameLiteral("Coords"), arr)
		}
		if len(sh.Domain) > 0 {
			arr := raw.NewArray()
			for _, d := range sh.Domain {
				arr.Append(raw.NumberFloat(d))
			}
			dict.Set(raw.NameLiteral("Domain"), arr)
		}
		if len(sh.Extend) > 0 {
			arr := raw.NewArray()
			for _, e := range sh.Extend {
				arr.Append(raw.Bool(e))
			}
			dict.Set(raw.NameLiteral("Extend"), arr)
		}
		if len(sh.Function) > 0 {
			if len(sh.Function) == 1 {
				fRef := b.funcSerializer.Serialize(sh.Function[0], b)
				dict.Set(raw.NameLiteral("Function"), raw.Ref(fRef.Num, fRef.Gen))
			} else {
				arr := raw.NewArray()
				for _, f := range sh.Function {
					fRef := b.funcSerializer.Serialize(f, b)
					arr.Append(raw.Ref(fRef.Num, fRef.Gen))
				}
				dict.Set(raw.NameLiteral("Function"), arr)
			}
		}
		b.objects[ref] = dict

	case *semantic.MeshShading:
		if cropSet(sh.BBox) {
			dict.Set(raw.NameLiteral("BBox"), rectArray(sh.BBox))
		}
		if sh.AntiAlias {
			dict.Set(raw.NameLiteral("AntiAlias"), raw.Bool(true))
		}
		dict.Set(raw.NameLiteral("BitsPerCoordinate"), raw.NumberInt(int64(sh.BitsPerCoordinate)))
		dict.Set(raw.NameLiteral("BitsPerComponent"), raw.NumberInt(int64(sh.BitsPerComponent)))
		dict.Set(raw.NameLiteral("BitsPerFlag"), raw.NumberInt(int64(sh.BitsPerFlag)))
		if len(sh.Decode) > 0 {
			arr := raw.NewArray()
			for _, d := range sh.Decode {
				arr.Append(raw.NumberFloat(d))
			}
			dict.Set(raw.NameLiteral("Decode"), arr)
		}
		if sh.Function != nil {
			fRef := b.funcSerializer.Serialize(sh.Function, b)
			dict.Set(raw.NameLiteral("Function"), raw.Ref(fRef.Num, fRef.Gen))
		}
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(sh.Stream))))
		b.objects[ref] = raw.NewStream(dict, sh.Stream)
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
			d.Set(raw.NameLiteral("Dest"), serializeDestination(item.Dest, pref))
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

func (b *objectBuilder) serializeResources(res *semantic.Resources) raw.Dictionary {
	if res == nil {
		return nil
	}
	resDict := raw.Dict()

	if len(res.Fonts) > 0 {
		d := raw.Dict()
		for name, font := range res.Fonts {
			ref := b.ensureFont(font)
			d.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
		}
		resDict.Set(raw.NameLiteral("Font"), d)
	}
	if len(res.XObjects) > 0 {
		d := raw.Dict()
		for name, xo := range res.XObjects {
			ref := b.ensureXObject(name, xo)
			d.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
		}
		resDict.Set(raw.NameLiteral("XObject"), d)
	}
	if len(res.ExtGStates) > 0 {
		d := raw.Dict()
		// ExtGState serialization is currently inline in Build.
		// We should probably refactor ensureExtGState but for now let's duplicate or leave it?
		// The logic in Build is complex (unions).
		// For Type 3 / Patterns, we just need to serialize them.
		// Let's implement a simple inline serialization here matching Build's logic but without unions.
		for name, gs := range res.ExtGStates {
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
			if gs.BlendMode != "" {
				entry.Set(raw.NameLiteral("BM"), raw.NameLiteral(gs.BlendMode))
			}
			if gs.AlphaSource != nil {
				entry.Set(raw.NameLiteral("AIS"), raw.Bool(*gs.AlphaSource))
			}
			if gs.TextKnockout != nil {
				entry.Set(raw.NameLiteral("TK"), raw.Bool(*gs.TextKnockout))
			}
			if gs.Overprint != nil {
				entry.Set(raw.NameLiteral("OP"), raw.Bool(*gs.Overprint))
			}
			if gs.OverprintFill != nil {
				entry.Set(raw.NameLiteral("op"), raw.Bool(*gs.OverprintFill))
			}
			if gs.OverprintMode != nil {
				entry.Set(raw.NameLiteral("OPM"), raw.NumberInt(int64(*gs.OverprintMode)))
			}
			if gs.UseBlackPtComp != nil {
				entry.Set(raw.NameLiteral("UseBlackPtComp"), raw.Bool(*gs.UseBlackPtComp))
			}
			if gs.SoftMask != nil {
				smDict := raw.Dict()
				smDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Mask"))
				smDict.Set(raw.NameLiteral("S"), raw.NameLiteral(gs.SoftMask.Subtype))
				if gs.SoftMask.Group != nil {
					gRef := b.ensureXObject("SMaskGroup", *gs.SoftMask.Group)
					smDict.Set(raw.NameLiteral("G"), raw.Ref(gRef.Num, gRef.Gen))
				}
				if len(gs.SoftMask.BackdropColor) > 0 {
					bc := raw.NewArray()
					for _, c := range gs.SoftMask.BackdropColor {
						bc.Append(raw.NumberFloat(c))
					}
					smDict.Set(raw.NameLiteral("BC"), bc)
				}
				if gs.SoftMask.Transfer != "" {
					smDict.Set(raw.NameLiteral("TR"), raw.NameLiteral(gs.SoftMask.Transfer))
				}
				entry.Set(raw.NameLiteral("SMask"), smDict)
			}
			d.Set(raw.NameLiteral(name), entry)
		}
		resDict.Set(raw.NameLiteral("ExtGState"), d)
	}
	if len(res.ColorSpaces) > 0 {
		d := raw.Dict()
		for name, cs := range res.ColorSpaces {
			obj := b.csSerializer.Serialize(cs, b)
			d.Set(raw.NameLiteral(name), obj)
		}
		resDict.Set(raw.NameLiteral("ColorSpace"), d)
	}
	if len(res.Patterns) > 0 {
		d := raw.Dict()
		for name, pat := range res.Patterns {
			ref := b.ensurePattern(name, pat)
			d.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
		}
		resDict.Set(raw.NameLiteral("Pattern"), d)
	}
	if len(res.Shadings) > 0 {
		d := raw.Dict()
		for name, sh := range res.Shadings {
			ref := b.ensureShading(name, sh)
			d.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
		}
		resDict.Set(raw.NameLiteral("Shading"), d)
	}
	if len(res.Properties) > 0 {
		d := raw.Dict()
		for name, prop := range res.Properties {
			ref := b.ensurePropertyList(name, prop)
			d.Set(raw.NameLiteral(name), raw.Ref(ref.Num, ref.Gen))
		}
		resDict.Set(raw.NameLiteral("Properties"), d)
	}

	return resDict
}

func (b *objectBuilder) ensurePropertyList(name string, pl semantic.PropertyList) raw.ObjectRef {
	ref := b.nextRef()
	dict := raw.Dict()

	switch p := pl.(type) {
	case *semantic.OptionalContentGroup:
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("OCG"))
		dict.Set(raw.NameLiteral("Name"), raw.Str([]byte(p.Name)))
		if len(p.Intent) > 0 {
			if len(p.Intent) == 1 {
				dict.Set(raw.NameLiteral("Intent"), raw.NameLiteral(p.Intent[0]))
			} else {
				arr := raw.NewArray()
				for _, i := range p.Intent {
					arr.Append(raw.NameLiteral(i))
				}
				dict.Set(raw.NameLiteral("Intent"), arr)
			}
		}
		// Usage serialization skipped for now
	case *semantic.OptionalContentMembership:
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("OCMD"))
		if len(p.OCGs) > 0 {
			if len(p.OCGs) == 1 {
				gRef := b.ensurePropertyList("", p.OCGs[0])
				dict.Set(raw.NameLiteral("OCGs"), raw.Ref(gRef.Num, gRef.Gen))
			} else {
				arr := raw.NewArray()
				for _, g := range p.OCGs {
					gRef := b.ensurePropertyList("", g)
					arr.Append(raw.Ref(gRef.Num, gRef.Gen))
				}
				dict.Set(raw.NameLiteral("OCGs"), arr)
			}
		}
		if p.Policy != "" {
			dict.Set(raw.NameLiteral("P"), raw.NameLiteral(p.Policy))
		}
	}
	b.objects[ref] = dict
	return ref
}
