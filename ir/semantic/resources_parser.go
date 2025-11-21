package semantic

import (
	"fmt"
	"pdflib/ir/raw"
)

func parseResources(obj raw.Object, resolver rawResolver) (*Resources, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("resources is not a dict")
	}

	res := &Resources{
		Fonts:       make(map[string]*Font),
		ExtGStates:  make(map[string]ExtGState),
		ColorSpaces: make(map[string]ColorSpace),
		XObjects:    make(map[string]XObject),
		Patterns:    make(map[string]Pattern),
		Shadings:    make(map[string]Shading),
		Properties:  make(map[string]PropertyList),
	}

	// ExtGState
	if gsObj, ok := dict.Get(raw.NameLiteral("ExtGState")); ok {
		if gsDict, ok := resolveDict(gsObj, resolver); ok {
			for k, v := range gsDict.KV {
				gs, err := parseExtGState(v, resolver)
				if err == nil {
					res.ExtGStates[k] = *gs
				}
			}
		}
	}

	// ColorSpace
	if csObj, ok := dict.Get(raw.NameLiteral("ColorSpace")); ok {
		if csDict, ok := resolveDict(csObj, resolver); ok {
			for k, v := range csDict.KV {
				cs, err := parseColorSpace(v, resolver)
				if err == nil {
					res.ColorSpaces[k] = cs
				}
			}
		}
	}

	// Other resources can be implemented as needed.
	// For now, we focus on ExtGState for BPC support.

	return res, nil
}

func parseColorSpace(obj raw.Object, resolver rawResolver) (ColorSpace, error) {
	// Resolve
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	// Name
	if name, ok := obj.(raw.NameObj); ok {
		return DeviceColorSpace{Name: name.Value()}, nil
	}

	// Array
	arr, ok := obj.(*raw.ArrayObj)
	if !ok {
		return nil, fmt.Errorf("ColorSpace is not name or array")
	}
	if len(arr.Items) == 0 {
		return nil, fmt.Errorf("empty ColorSpace array")
	}

	nameObj, ok := arr.Items[0].(raw.NameObj)
	if !ok {
		return nil, fmt.Errorf("ColorSpace array first element is not name")
	}
	name := nameObj.Value()

	switch name {
	case "SpectrallyDefined":
		// [ /SpectrallyDefined <stream> ]
		if len(arr.Items) < 2 {
			return nil, fmt.Errorf("SpectrallyDefined missing stream")
		}
		streamObj := arr.Items[1]
		if ref, ok := streamObj.(raw.Reference); ok {
			resolved, err := resolver.Resolve(ref.Ref())
			if err == nil {
				streamObj = resolved
			}
		}
		if stream, ok := streamObj.(*raw.StreamObj); ok {
			return &SpectrallyDefinedColorSpace{Data: stream.Data}, nil
		}
		return nil, fmt.Errorf("SpectrallyDefined second element is not stream")
	}

	return DeviceColorSpace{Name: name}, nil // Fallback
}

func parseExtGState(obj raw.Object, resolver rawResolver) (*ExtGState, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("ExtGState is not a dict")
	}

	gs := &ExtGState{}

	if v, ok := dict.Get(raw.NameLiteral("LW")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			val := n.Float()
			gs.LineWidth = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("CA")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			val := n.Float()
			gs.StrokeAlpha = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("ca")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			val := n.Float()
			gs.FillAlpha = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("BM")); ok {
		if n, ok := v.(raw.NameObj); ok {
			gs.BlendMode = n.Value()
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("AIS")); ok {
		if b, ok := v.(raw.BoolObj); ok {
			val := b.Value()
			gs.AlphaSource = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("TK")); ok {
		if b, ok := v.(raw.BoolObj); ok {
			val := b.Value()
			gs.TextKnockout = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("OP")); ok {
		if b, ok := v.(raw.BoolObj); ok {
			val := b.Value()
			gs.Overprint = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("op")); ok {
		if b, ok := v.(raw.BoolObj); ok {
			val := b.Value()
			gs.OverprintFill = &val
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("OPM")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			val := int(n.Int())
			gs.OverprintMode = &val
		}
	}

	// UseBlackPtComp (PDF 2.0)
	if v, ok := dict.Get(raw.NameLiteral("UseBlackPtComp")); ok {
		if b, ok := v.(raw.BoolObj); ok {
			val := b.Value()
			gs.UseBlackPtComp = &val
		}
	}

	return gs, nil
}
