package writer

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
)

// SerializationContext provides access to the writer's state for serializers.
type SerializationContext interface {
	NextRef() raw.ObjectRef
	AddObject(ref raw.ObjectRef, obj raw.Object)
	PageRef(index int) *raw.ObjectRef
}

// AnnotationSerializer serializes semantic annotations into raw objects.
type AnnotationSerializer interface {
	Serialize(annot semantic.Annotation, ctx SerializationContext) (raw.ObjectRef, error)
}

// ActionSerializer serializes semantic actions into raw objects.
type ActionSerializer interface {
	Serialize(action semantic.Action, ctx SerializationContext) raw.Object
}

// ColorSpaceSerializer serializes semantic color spaces into raw objects.
type ColorSpaceSerializer interface {
	Serialize(cs semantic.ColorSpace, ctx SerializationContext) raw.Object
}

type defaultAnnotationSerializer struct {
	actionSerializer ActionSerializer
}

func newAnnotationSerializer(as ActionSerializer) AnnotationSerializer {
	return &defaultAnnotationSerializer{actionSerializer: as}
}

func (s *defaultAnnotationSerializer) Serialize(a semantic.Annotation, ctx SerializationContext) (raw.ObjectRef, error) {
	ref := ctx.NextRef()
	dict := raw.Dict()
	dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Annot"))

	base := a.Base()
	subtype := base.Subtype
	if subtype == "" {
		subtype = "Link"
	}
	dict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(subtype))

	rect := base.RectVal
	// Note: Fallback to crop/media box is handled by the caller (objectBuilder)
	// because it requires access to the Page object which is not easily available here
	// without passing more context. For now, we assume Rect is set or we handle it if passed.
	// Actually, looking at object_builder.go, it uses p.CropBox/MediaBox.
	// We might need to pass the page context or the default rect.
	// Let's assume the caller sets the Rect on the annotation before calling Serialize
	// or we pass the default rect.

	dict.Set(raw.NameLiteral("Rect"), rectArray(rect))

	switch t := a.(type) {
	case *semantic.LinkAnnotation:
		if t.Action != nil {
			if act := s.actionSerializer.Serialize(t.Action, ctx); act != nil {
				dict.Set(raw.NameLiteral("A"), act)
			}
		} else if t.URI != "" {
			// Fallback for legacy URI field
			action := raw.Dict()
			action.Set(raw.NameLiteral("S"), raw.NameLiteral("URI"))
			action.Set(raw.NameLiteral("URI"), raw.Str([]byte(t.URI)))
			dict.Set(raw.NameLiteral("A"), action)
		}
	case *semantic.TextAnnotation:
		if t.Open {
			dict.Set(raw.NameLiteral("Open"), raw.Bool(true))
		}
		if t.Icon != "" {
			dict.Set(raw.NameLiteral("Name"), raw.NameLiteral(t.Icon))
		}
	case *semantic.HighlightAnnotation:
		if len(t.QuadPoints) > 0 {
			qp := raw.NewArray()
			for _, v := range t.QuadPoints {
				qp.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("QuadPoints"), qp)
		}
	case *semantic.UnderlineAnnotation:
		if len(t.QuadPoints) > 0 {
			qp := raw.NewArray()
			for _, v := range t.QuadPoints {
				qp.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("QuadPoints"), qp)
		}
	case *semantic.StrikeOutAnnotation:
		if len(t.QuadPoints) > 0 {
			qp := raw.NewArray()
			for _, v := range t.QuadPoints {
				qp.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("QuadPoints"), qp)
		}
	case *semantic.SquigglyAnnotation:
		if len(t.QuadPoints) > 0 {
			qp := raw.NewArray()
			for _, v := range t.QuadPoints {
				qp.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("QuadPoints"), qp)
		}
	case *semantic.FreeTextAnnotation:
		if t.DA != "" {
			dict.Set(raw.NameLiteral("DA"), raw.Str([]byte(t.DA)))
		}
		if t.Q != 0 {
			dict.Set(raw.NameLiteral("Q"), raw.NumberInt(int64(t.Q)))
		}
	case *semantic.LineAnnotation:
		if len(t.L) == 4 {
			l := raw.NewArray()
			for _, v := range t.L {
				l.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("L"), l)
		}
		if len(t.LE) == 2 {
			le := raw.NewArray()
			for _, v := range t.LE {
				le.Append(raw.NameLiteral(v))
			}
			dict.Set(raw.NameLiteral("LE"), le)
		}
		if len(t.IC) > 0 {
			ic := raw.NewArray()
			for _, v := range t.IC {
				ic.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("IC"), ic)
		}
	case *semantic.SquareAnnotation:
		if len(t.IC) > 0 {
			ic := raw.NewArray()
			for _, v := range t.IC {
				ic.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("IC"), ic)
		}
		if len(t.RD) == 4 {
			rd := raw.NewArray()
			for _, v := range t.RD {
				rd.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("RD"), rd)
		}
	case *semantic.CircleAnnotation:
		if len(t.IC) > 0 {
			ic := raw.NewArray()
			for _, v := range t.IC {
				ic.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("IC"), ic)
		}
		if len(t.RD) == 4 {
			rd := raw.NewArray()
			for _, v := range t.RD {
				rd.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("RD"), rd)
		}
	case *semantic.WidgetAnnotation:
		if t.Field != nil {
			if ft := t.Field.FieldType(); ft != "" {
				dict.Set(raw.NameLiteral("FT"), raw.NameLiteral(ft))
			}
			if name := t.Field.FieldName(); name != "" {
				dict.Set(raw.NameLiteral("T"), raw.Str([]byte(name)))
			}

			switch f := t.Field.(type) {
			case *semantic.TextFormField:
				if f.Value != "" {
					dict.Set(raw.NameLiteral("V"), raw.Str([]byte(f.Value)))
				}
			case *semantic.ChoiceFormField:
				if len(f.Selected) > 0 {
					if len(f.Selected) == 1 {
						dict.Set(raw.NameLiteral("V"), raw.Str([]byte(f.Selected[0])))
					} else {
						arr := raw.NewArray()
						for _, s := range f.Selected {
							arr.Append(raw.Str([]byte(s)))
						}
						dict.Set(raw.NameLiteral("V"), arr)
					}
				}
			case *semantic.ButtonFormField:
				if f.IsCheck || f.IsRadio {
					if f.Checked {
						val := f.OnState
						if val == "" {
							val = "Yes"
						}
						dict.Set(raw.NameLiteral("V"), raw.NameLiteral(val))
					} else {
						dict.Set(raw.NameLiteral("V"), raw.NameLiteral("Off"))
					}
				}
			case *semantic.GenericFormField:
				if f.Value != "" {
					dict.Set(raw.NameLiteral("V"), raw.Str([]byte(f.Value)))
				}
			case *semantic.SignatureFormField:
				if f.Signature != nil {
					sigRef := serializeSignature(f.Signature, ctx)
					dict.Set(raw.NameLiteral("V"), raw.Ref(sigRef.Num, sigRef.Gen))
				}
			}

			if flags := t.Field.FieldFlags(); flags != 0 {
				dict.Set(raw.NameLiteral("Ff"), raw.NumberInt(int64(flags)))
			}
		}
	case *semantic.StampAnnotation:
		if t.Name != "" {
			dict.Set(raw.NameLiteral("Name"), raw.NameLiteral(t.Name))
		}
	case *semantic.InkAnnotation:
		if len(t.InkList) > 0 {
			arr := raw.NewArray()
			for _, path := range t.InkList {
				pathArr := raw.NewArray()
				for _, pt := range path {
					pathArr.Append(raw.NumberFloat(pt))
				}
				arr.Append(pathArr)
			}
			dict.Set(raw.NameLiteral("InkList"), arr)
		}
	case *semantic.FileAttachmentAnnotation:
		if t.Name != "" {
			dict.Set(raw.NameLiteral("Name"), raw.NameLiteral(t.Name))
		}
		if len(t.File.Data) > 0 {
			fsRef := ctx.NextRef()
			fsDict := raw.Dict()
			fsDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("EmbeddedFile"))
			fsDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.File.Data))))
			ctx.AddObject(fsRef, raw.NewStream(fsDict, t.File.Data))

			fDict := raw.Dict()
			fDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Filespec"))
			fDict.Set(raw.NameLiteral("F"), raw.Str([]byte(t.File.Name)))

			efDict := raw.Dict()
			efDict.Set(raw.NameLiteral("F"), raw.Ref(fsRef.Num, fsRef.Gen))
			fDict.Set(raw.NameLiteral("EF"), efDict)

			dict.Set(raw.NameLiteral("FS"), fDict)
		}
	case *semantic.PopupAnnotation:
		if t.Open {
			dict.Set(raw.NameLiteral("Open"), raw.Bool(true))
		}
		if t.Parent != nil {
			pRef := t.Parent.Reference()
			if pRef.Num != 0 {
				dict.Set(raw.NameLiteral("Parent"), raw.Ref(pRef.Num, pRef.Gen))
			}
		}
	case *semantic.SoundAnnotation:
		if t.Name != "" {
			dict.Set(raw.NameLiteral("Name"), raw.NameLiteral(t.Name))
		}
		if len(t.Sound.Data) > 0 {
			soundRef := ctx.NextRef()
			soundDict := raw.Dict()
			soundDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Sound"))
			// Basic sound parameters, could be expanded
			soundDict.Set(raw.NameLiteral("R"), raw.NumberInt(44100)) // Rate
			soundDict.Set(raw.NameLiteral("C"), raw.NumberInt(1))     // Channels
			soundDict.Set(raw.NameLiteral("B"), raw.NumberInt(16))    // Bits
			soundDict.Set(raw.NameLiteral("E"), raw.NameLiteral("Signed"))
			soundDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.Sound.Data))))
			ctx.AddObject(soundRef, raw.NewStream(soundDict, t.Sound.Data))
			dict.Set(raw.NameLiteral("Sound"), raw.Ref(soundRef.Num, soundRef.Gen))
		}
	case *semantic.MovieAnnotation:
		if t.Title != "" {
			dict.Set(raw.NameLiteral("T"), raw.Str([]byte(t.Title)))
		}
		if len(t.Movie.Data) > 0 {
			// Movie dictionary is complex, simplified here
			movieDict := raw.Dict()
			// FileSpec for the movie
			fsRef := ctx.NextRef()
			fsDict := raw.Dict()
			fsDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("EmbeddedFile"))
			fsDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.Movie.Data))))
			ctx.AddObject(fsRef, raw.NewStream(fsDict, t.Movie.Data))

			fDict := raw.Dict()
			fDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Filespec"))
			fDict.Set(raw.NameLiteral("F"), raw.Str([]byte(t.Movie.Name)))
			efDict := raw.Dict()
			efDict.Set(raw.NameLiteral("F"), raw.Ref(fsRef.Num, fsRef.Gen))
			fDict.Set(raw.NameLiteral("EF"), efDict)

			movieDict.Set(raw.NameLiteral("F"), fDict)
			dict.Set(raw.NameLiteral("Movie"), movieDict)
		}
	case *semantic.ScreenAnnotation:
		if t.Title != "" {
			dict.Set(raw.NameLiteral("T"), raw.Str([]byte(t.Title)))
		}
		if t.Action != nil {
			if act := s.actionSerializer.Serialize(t.Action, ctx); act != nil {
				dict.Set(raw.NameLiteral("A"), act)
			}
		}
	case *semantic.PrinterMarkAnnotation:
		// No specific fields for now
	case *semantic.TrapNetAnnotation:
		if t.LastModified != "" {
			dict.Set(raw.NameLiteral("LastModified"), raw.Str([]byte(t.LastModified)))
		}
		if len(t.Version) > 0 {
			ver := raw.NewArray()
			for _, v := range t.Version {
				ver.Append(raw.NumberInt(int64(v)))
			}
			dict.Set(raw.NameLiteral("Version"), ver)
		}
	case *semantic.WatermarkAnnotation:
		if t.FixedPrint {
			dict.Set(raw.NameLiteral("FixedPrint"), raw.Bool(true))
		}
	case *semantic.ThreeDAnnotation:
		if len(t.ThreeD.Data) > 0 {
			streamRef := ctx.NextRef()
			streamDict := raw.Dict()
			streamDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("3D"))
			streamDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("U3D")) // Assuming U3D for now
			streamDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.ThreeD.Data))))
			ctx.AddObject(streamRef, raw.NewStream(streamDict, t.ThreeD.Data))
			dict.Set(raw.NameLiteral("3DD"), raw.Ref(streamRef.Num, streamRef.Gen))
		}
		if t.View != "" {
			// Simplified view handling
			viewDict := raw.Dict()
			viewDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("3DView"))
			viewDict.Set(raw.NameLiteral("XN"), raw.Str([]byte(t.View)))
			dict.Set(raw.NameLiteral("3DV"), viewDict)
		}
	case *semantic.RedactAnnotation:
		if t.OverlayText != "" {
			dict.Set(raw.NameLiteral("OverlayText"), raw.Str([]byte(t.OverlayText)))
		}
		if len(t.Repeat) > 0 {
			rep := raw.NewArray()
			for _, v := range t.Repeat {
				rep.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("Repeat"), rep)
		}
	case *semantic.ProjectionAnnotation:
		if t.ProjectionType != "" {
			dict.Set(raw.NameLiteral("ProjectionType"), raw.NameLiteral(t.ProjectionType))
		}
	}

	if base.Contents != "" {
		dict.Set(raw.NameLiteral("Contents"), raw.Str([]byte(base.Contents)))
	}

	if len(base.Appearance) > 0 {
		apRef := ctx.NextRef()
		apDict := raw.Dict()
		apDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(base.Appearance))))
		apStream := raw.NewStream(apDict, base.Appearance)
		ctx.AddObject(apRef, apStream)
		ap := raw.Dict()
		ap.Set(raw.NameLiteral("N"), raw.Ref(apRef.Num, apRef.Gen))
		dict.Set(raw.NameLiteral("AP"), ap)
	}

	if base.Flags != 0 {
		dict.Set(raw.NameLiteral("F"), raw.NumberInt(int64(base.Flags)))
	}

	if len(base.Border) == 3 {
		dict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberFloat(base.Border[0]), raw.NumberFloat(base.Border[1]), raw.NumberFloat(base.Border[2])))
	} else {
		dict.Set(raw.NameLiteral("Border"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(0)))
	}

	if len(base.Color) > 0 {
		colArr := raw.NewArray()
		for _, c := range base.Color {
			colArr.Append(raw.NumberFloat(c))
		}
		dict.Set(raw.NameLiteral("C"), colArr)
	}

	if base.AppearanceState != "" {
		dict.Set(raw.NameLiteral("AS"), raw.NameLiteral(base.AppearanceState))
	}

	if len(base.AssociatedFiles) > 0 {
		if af := SerializeAssociatedFiles(base.AssociatedFiles, ctx); af != nil {
			dict.Set(raw.NameLiteral("AF"), af)
		}
	}

	ctx.AddObject(ref, dict)
	return ref, nil
}

// SerializeAssociatedFiles serializes a list of embedded files into an AF array.
func SerializeAssociatedFiles(files []semantic.EmbeddedFile, ctx SerializationContext) raw.Object {
	if len(files) == 0 {
		return nil
	}
	arr := raw.NewArray()
	for _, ef := range files {
		if len(ef.Data) == 0 && ef.Name == "" {
			continue
		}
		// Create EmbeddedFile Stream
		streamRef := ctx.NextRef()
		streamDict := raw.Dict()
		streamDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("EmbeddedFile"))
		if ef.Subtype != "" {
			streamDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(ef.Subtype))
		}
		streamDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(ef.Data))))
		ctx.AddObject(streamRef, raw.NewStream(streamDict, ef.Data))

		// Create Filespec
		fsRef := ctx.NextRef()
		fsDict := raw.Dict()
		fsDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Filespec"))
		fsDict.Set(raw.NameLiteral("F"), raw.Str([]byte(ef.Name)))
		fsDict.Set(raw.NameLiteral("UF"), raw.Str([]byte(ef.Name)))
		if ef.Description != "" {
			fsDict.Set(raw.NameLiteral("Desc"), raw.Str([]byte(ef.Description)))
		}
		rel := ef.Relationship
		if rel == "" {
			rel = "Unspecified"
		}
		fsDict.Set(raw.NameLiteral("AFRelationship"), raw.NameLiteral(rel))

		efDict := raw.Dict()
		efDict.Set(raw.NameLiteral("F"), raw.Ref(streamRef.Num, streamRef.Gen))
		fsDict.Set(raw.NameLiteral("EF"), efDict)

		ctx.AddObject(fsRef, fsDict)
		arr.Append(raw.Ref(fsRef.Num, fsRef.Gen))
	}
	if arr.Len() == 0 {
		return nil
	}
	return arr
}

type defaultActionSerializer struct{}

func newActionSerializer() ActionSerializer {
	return &defaultActionSerializer{}
}

func (s *defaultActionSerializer) Serialize(a semantic.Action, ctx SerializationContext) raw.Object {
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
		if pref := ctx.PageRef(act.PageIndex); pref != nil {
			d.Set(raw.NameLiteral("D"), serializeDestination(act.Dest, *pref))
		}
	case semantic.JavaScriptAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("JavaScript"))
		d.Set(raw.NameLiteral("JS"), raw.Str([]byte(act.JS)))
	case semantic.NamedAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("Named"))
		d.Set(raw.NameLiteral("N"), raw.NameLiteral(act.Name))
	case semantic.LaunchAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("Launch"))
		if act.File != "" {
			d.Set(raw.NameLiteral("F"), raw.Str([]byte(act.File)))
		}
		if act.NewWindow != nil {
			d.Set(raw.NameLiteral("NewWindow"), raw.Bool(*act.NewWindow))
		}
	case semantic.SubmitFormAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("SubmitForm"))
		fDict := raw.Dict()
		fDict.Set(raw.NameLiteral("FS"), raw.NameLiteral("URL"))
		fDict.Set(raw.NameLiteral("F"), raw.Str([]byte(act.URL)))
		d.Set(raw.NameLiteral("F"), fDict)
		if act.Flags != 0 {
			d.Set(raw.NameLiteral("Flags"), raw.NumberInt(int64(act.Flags)))
		}
	case semantic.ResetFormAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("ResetForm"))
		if len(act.Fields) > 0 {
			arr := raw.NewArray()
			for _, f := range act.Fields {
				arr.Append(raw.Str([]byte(f)))
			}
			d.Set(raw.NameLiteral("Fields"), arr)
		}
		if act.Flags != 0 {
			d.Set(raw.NameLiteral("Flags"), raw.NumberInt(int64(act.Flags)))
		}
	case semantic.ImportDataAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("ImportData"))
		d.Set(raw.NameLiteral("F"), raw.Str([]byte(act.File)))
	case semantic.GoToRAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("GoToR"))
		d.Set(raw.NameLiteral("F"), raw.Str([]byte(act.File)))
		if act.DestName != "" {
			d.Set(raw.NameLiteral("D"), raw.Str([]byte(act.DestName)))
		} else if act.Dest != nil {
			// For GoToR, D is [page /Fit ...] where page is an integer index
			// We assume page index 0 if not specified, as OutlineDestination doesn't carry it.
			// Ideally, GoToRAction should have a PageIndex field.
			// For now, we default to 0.
			arr := raw.NewArray(
				raw.NumberInt(0),
				raw.NameLiteral("XYZ"),
				xyzDestValue(act.Dest.X),
				xyzDestValue(act.Dest.Y),
				xyzDestValue(act.Dest.Zoom),
			)
			d.Set(raw.NameLiteral("D"), arr)
		}
		if act.NewWindow != nil {
			d.Set(raw.NameLiteral("NewWindow"), raw.Bool(*act.NewWindow))
		}
	case semantic.GoToEAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("GoToE"))
		if act.DestName != "" {
			d.Set(raw.NameLiteral("D"), raw.Str([]byte(act.DestName)))
		} else if act.Dest != nil {
			// GoToE destination is also [page /Fit ...]
			arr := raw.NewArray(
				raw.NumberInt(0),
				raw.NameLiteral("XYZ"),
				xyzDestValue(act.Dest.X),
				xyzDestValue(act.Dest.Y),
				xyzDestValue(act.Dest.Zoom),
			)
			d.Set(raw.NameLiteral("D"), arr)
		}
		if act.NewWindow != nil {
			d.Set(raw.NameLiteral("NewWindow"), raw.Bool(*act.NewWindow))
		}
		if act.Target != nil {
			// Target dictionary (T)
			tDict := raw.Dict()
			tDict.Set(raw.NameLiteral("R"), raw.NameLiteral("C")) // Relation: Child
			tDict.Set(raw.NameLiteral("N"), raw.Str([]byte(act.Target.Name)))
			// We might need to serialize the embedded file stream here if it's new
			// But usually EmbeddedFiles are handled at document level.
			// If we need to link to a specific embedded file, we use the name in the Names tree.
			// The spec says T is a target dictionary.
			d.Set(raw.NameLiteral("T"), tDict)
		}
	case semantic.HideAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("Hide"))
		d.Set(raw.NameLiteral("T"), raw.Str([]byte(act.TargetName)))
		d.Set(raw.NameLiteral("H"), raw.Bool(act.Hide))
	case semantic.GoTo3DViewAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("GoTo3DView"))
		if act.Target != nil {
			ref := act.Target.Reference()
			if ref.Num != 0 {
				d.Set(raw.NameLiteral("TA"), raw.Ref(ref.Num, ref.Gen))
			}
		}
		d.Set(raw.NameLiteral("V"), raw.NameLiteral(act.View))
	case semantic.ThreadAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("Thread"))
		if act.File != "" {
			d.Set(raw.NameLiteral("F"), raw.Str([]byte(act.File)))
		}
		if act.Thread.Num != 0 {
			d.Set(raw.NameLiteral("D"), raw.Ref(act.Thread.Num, act.Thread.Gen))
		}
		if act.Bead.Num != 0 {
			d.Set(raw.NameLiteral("B"), raw.Ref(act.Bead.Num, act.Bead.Gen))
		}
	case semantic.RichMediaExecuteAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("RichMediaExecute"))
		if act.Command.Num != 0 {
			d.Set(raw.NameLiteral("TI"), raw.Ref(act.Command.Num, act.Command.Gen))
		}
	case semantic.SoundAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("Sound"))
		if act.Sound != nil && len(act.Sound.Data) > 0 {
			sRef := ctx.NextRef()
			sDict := raw.Dict()
			sDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Sound"))
			sDict.Set(raw.NameLiteral("R"), raw.NumberInt(44100))
			sDict.Set(raw.NameLiteral("C"), raw.NumberInt(1))
			sDict.Set(raw.NameLiteral("B"), raw.NumberInt(16))
			sDict.Set(raw.NameLiteral("E"), raw.NameLiteral("Signed"))
			sDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(act.Sound.Data))))
			ctx.AddObject(sRef, raw.NewStream(sDict, act.Sound.Data))
			d.Set(raw.NameLiteral("Sound"), raw.Ref(sRef.Num, sRef.Gen))
		}
		if act.Volume != nil {
			d.Set(raw.NameLiteral("Volume"), raw.NumberFloat(*act.Volume))
		}
		if act.Synchronous != nil {
			d.Set(raw.NameLiteral("Synchronous"), raw.Bool(*act.Synchronous))
		}
		if act.Repeat != nil {
			d.Set(raw.NameLiteral("Repeat"), raw.Bool(*act.Repeat))
		}
		if act.Mix != nil {
			d.Set(raw.NameLiteral("Mix"), raw.Bool(*act.Mix))
		}
	case semantic.MovieAction:
		d.Set(raw.NameLiteral("S"), raw.NameLiteral("Movie"))
		if act.Title != "" {
			d.Set(raw.NameLiteral("T"), raw.Str([]byte(act.Title)))
		}
		if act.Operation != "" {
			d.Set(raw.NameLiteral("Operation"), raw.NameLiteral(act.Operation))
		}
	}
	return d
}

type defaultColorSpaceSerializer struct {
	funcSerializer FunctionSerializer
}

func newColorSpaceSerializer(fs FunctionSerializer) ColorSpaceSerializer {
	return &defaultColorSpaceSerializer{funcSerializer: fs}
}

func (s *defaultColorSpaceSerializer) Serialize(cs semantic.ColorSpace, ctx SerializationContext) raw.Object {
	if cs == nil {
		return raw.NameLiteral("DeviceRGB")
	}
	switch t := cs.(type) {
	case *semantic.ICCBasedColorSpace:
		ref := ctx.NextRef()
		dict := raw.Dict()
		n := 3
		if len(t.Range) > 0 {
			n = len(t.Range) / 2
		}
		dict.Set(raw.NameLiteral("N"), raw.NumberInt(int64(n)))
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.Profile))))
		if len(t.Range) > 0 {
			arr := raw.NewArray()
			for _, v := range t.Range {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("Range"), arr)
		}
		if t.Alternate != nil {
			altName := t.Alternate.ColorSpaceName()
			dict.Set(raw.NameLiteral("Alternate"), raw.NameLiteral(altName))
		}

		stream := raw.NewStream(dict, t.Profile)
		ctx.AddObject(ref, stream)

		return raw.NewArray(raw.NameLiteral("ICCBased"), raw.Ref(ref.Num, ref.Gen))

	case *semantic.SeparationColorSpace:
		arr := raw.NewArray(raw.NameLiteral("Separation"), raw.NameLiteral(t.Name))
		if t.Alternate != nil {
			arr.Append(s.Serialize(t.Alternate, ctx))
		} else {
			arr.Append(raw.NameLiteral("DeviceGray"))
		}
		// TintTransform function
		if t.TintTransform != nil {
			ref := s.funcSerializer.Serialize(t.TintTransform, ctx)
			arr.Append(raw.Ref(ref.Num, ref.Gen))
		}
		return arr

	case *semantic.DeviceNColorSpace:
		arr := raw.NewArray(raw.NameLiteral("DeviceN"))
		names := raw.NewArray()
		for _, n := range t.Names {
			names.Append(raw.NameLiteral(n))
		}
		arr.Append(names)
		if t.Alternate != nil {
			arr.Append(s.Serialize(t.Alternate, ctx))
		} else {
			arr.Append(raw.NameLiteral("DeviceCMYK"))
		}
		if t.TintTransform != nil {
			ref := s.funcSerializer.Serialize(t.TintTransform, ctx)
			arr.Append(raw.Ref(ref.Num, ref.Gen))
		}
		if t.Attributes != nil {
			attr := raw.Dict()
			if t.Attributes.Subtype != "" {
				attr.Set(raw.NameLiteral("Subtype"), raw.NameLiteral(t.Attributes.Subtype))
			}
			arr.Append(attr)
		}
		return arr

	case *semantic.SpectrallyDefinedColorSpace:
		ref := ctx.NextRef()
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.Data))))
		stream := raw.NewStream(dict, t.Data)
		ctx.AddObject(ref, stream)
		return raw.NewArray(raw.NameLiteral("SpectrallyDefined"), raw.Ref(ref.Num, ref.Gen))

	case *semantic.PatternColorSpace:
		if t.Underlying != nil {
			return raw.NewArray(raw.NameLiteral("Pattern"), s.Serialize(t.Underlying, ctx))
		}
		return raw.NameLiteral("Pattern")
	}
	return raw.NameLiteral(cs.ColorSpaceName())
}

func serializeSignature(sig *semantic.Signature, ctx SerializationContext) raw.ObjectRef {
	ref := ctx.NextRef()
	dict := raw.Dict()
	dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Sig"))
	if sig.Filter != "" {
		dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral(sig.Filter))
	}
	if sig.SubFilter != "" {
		dict.Set(raw.NameLiteral("SubFilter"), raw.NameLiteral(sig.SubFilter))
	}
	if len(sig.Contents) > 0 {
		dict.Set(raw.NameLiteral("Contents"), raw.Str(sig.Contents))
	}
	if len(sig.Cert) > 0 {
		dict.Set(raw.NameLiteral("Cert"), raw.Str(sig.Cert))
	}
	if len(sig.ByteRange) > 0 {
		br := raw.NewArray()
		for _, v := range sig.ByteRange {
			br.Append(raw.NumberInt(int64(v)))
		}
		dict.Set(raw.NameLiteral("ByteRange"), br)
	}
	if sig.Name != "" {
		dict.Set(raw.NameLiteral("Name"), raw.Str([]byte(sig.Name)))
	}
	if sig.M != "" {
		dict.Set(raw.NameLiteral("M"), raw.Str([]byte(sig.M)))
	}
	if sig.Location != "" {
		dict.Set(raw.NameLiteral("Location"), raw.Str([]byte(sig.Location)))
	}
	if sig.Reason != "" {
		dict.Set(raw.NameLiteral("Reason"), raw.Str([]byte(sig.Reason)))
	}
	if sig.ContactInfo != "" {
		dict.Set(raw.NameLiteral("ContactInfo"), raw.Str([]byte(sig.ContactInfo)))
	}
	if len(sig.Reference) > 0 {
		refs := raw.NewArray()
		for _, r := range sig.Reference {
			rd := raw.Dict()
			rd.Set(raw.NameLiteral("Type"), raw.NameLiteral("SigRef"))
			if r.TransformMethod != "" {
				rd.Set(raw.NameLiteral("TransformMethod"), raw.NameLiteral(r.TransformMethod))
			}
			if r.DigestMethod != "" {
				rd.Set(raw.NameLiteral("DigestMethod"), raw.NameLiteral(r.DigestMethod))
			}
			if len(r.DigestValue) > 0 {
				rd.Set(raw.NameLiteral("DigestValue"), raw.Str(r.DigestValue))
			}
			if len(r.DigestLocation) > 0 {
				dl := raw.NewArray()
				for _, v := range r.DigestLocation {
					dl.Append(raw.NumberInt(int64(v)))
				}
				rd.Set(raw.NameLiteral("DigestLocation"), dl)
			}
			if r.TransformParams != nil {
				tp := raw.Dict()
				tp.Set(raw.NameLiteral("Type"), raw.NameLiteral("TransformParams"))
				if r.TransformParams.P != 0 {
					tp.Set(raw.NameLiteral("P"), raw.NumberInt(int64(r.TransformParams.P)))
				}
				if r.TransformParams.V != "" {
					tp.Set(raw.NameLiteral("V"), raw.NameLiteral(r.TransformParams.V))
				}
				if r.TransformParams.Action != "" {
					tp.Set(raw.NameLiteral("Action"), raw.NameLiteral(r.TransformParams.Action))
				}
				if len(r.TransformParams.Fields) > 0 {
					flds := raw.NewArray()
					for _, f := range r.TransformParams.Fields {
						flds.Append(raw.Str([]byte(f)))
					}
					tp.Set(raw.NameLiteral("Fields"), flds)
				}
				rd.Set(raw.NameLiteral("TransformParams"), tp)
			}
			refs.Append(rd)
		}
		dict.Set(raw.NameLiteral("Reference"), refs)
	}

	ctx.AddObject(ref, dict)
	return ref
}
