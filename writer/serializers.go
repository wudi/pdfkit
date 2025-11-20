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
			if t.Field.Type != "" {
				dict.Set(raw.NameLiteral("FT"), raw.NameLiteral(t.Field.Type))
			}
			if t.Field.Name != "" {
				dict.Set(raw.NameLiteral("T"), raw.Str([]byte(t.Field.Name)))
			}
			if t.Field.Value != "" {
				dict.Set(raw.NameLiteral("V"), raw.Str([]byte(t.Field.Value)))
			}
			if t.Field.Flags != 0 {
				dict.Set(raw.NameLiteral("Ff"), raw.NumberInt(int64(t.Field.Flags)))
			}
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

	ctx.AddObject(ref, dict)
	return ref, nil
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
	}
	return d
}

type defaultColorSpaceSerializer struct{}

func newColorSpaceSerializer() ColorSpaceSerializer {
	return &defaultColorSpaceSerializer{}
}

func (s *defaultColorSpaceSerializer) Serialize(cs semantic.ColorSpace, ctx SerializationContext) raw.Object {
	if cs == nil {
		return raw.NameLiteral("DeviceRGB")
	}
	name := cs.ColorSpaceName()
	if name == "ICCBased" {
		if icc, ok := cs.(*semantic.ICCBasedColorSpace); ok {
			ref := ctx.NextRef()
			dict := raw.Dict()
			n := 3
			if len(icc.Range) > 0 {
				n = len(icc.Range) / 2
			}
			dict.Set(raw.NameLiteral("N"), raw.NumberInt(int64(n)))
			dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(icc.Profile))))
			if len(icc.Range) > 0 {
				arr := raw.NewArray()
				for _, v := range icc.Range {
					arr.Append(raw.NumberFloat(v))
				}
				dict.Set(raw.NameLiteral("Range"), arr)
			}
			if icc.Alternate != nil {
				altName := icc.Alternate.ColorSpaceName()
				dict.Set(raw.NameLiteral("Alternate"), raw.NameLiteral(altName))
			}

			stream := raw.NewStream(dict, icc.Profile)
			ctx.AddObject(ref, stream)

			return raw.NewArray(raw.NameLiteral("ICCBased"), raw.Ref(ref.Num, ref.Gen))
		}
	}
	return raw.NameLiteral(name)
}
