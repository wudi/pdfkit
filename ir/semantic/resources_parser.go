package semantic

import (
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
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

	// Font
	if fontObj, ok := dict.Get(raw.NameLiteral("Font")); ok {
		if fontDict, ok := resolveDict(fontObj, resolver); ok {
			for k, v := range fontDict.KV {
				f, err := parseFont(v, resolver)
				if err == nil {
					res.Fonts[k] = f
				}
			}
		}
	}

	// XObject
	if xObj, ok := dict.Get(raw.NameLiteral("XObject")); ok {
		if xDict, ok := resolveDict(xObj, resolver); ok {
			for k, v := range xDict.KV {
				xo, err := parseXObject(v, resolver)
				if err == nil {
					res.XObjects[k] = *xo
				}
			}
		}
	}

	// Pattern
	if patObj, ok := dict.Get(raw.NameLiteral("Pattern")); ok {
		if patDict, ok := resolveDict(patObj, resolver); ok {
			for k, v := range patDict.KV {
				p, err := parsePattern(v, resolver)
				if err == nil {
					res.Patterns[k] = p
				}
			}
		}
	}

	// Shading
	if shObj, ok := dict.Get(raw.NameLiteral("Shading")); ok {
		if shDict, ok := resolveDict(shObj, resolver); ok {
			for k, v := range shDict.KV {
				s, err := parseShading(v, resolver)
				if err == nil {
					res.Shadings[k] = s
				}
			}
		}
	}

	// Properties
	if propObj, ok := dict.Get(raw.NameLiteral("Properties")); ok {
		if propDict, ok := resolveDict(propObj, resolver); ok {
			for k, v := range propDict.KV {
				p, err := parsePropertyList(v, resolver)
				if err == nil {
					res.Properties[k] = p
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
			data, err := decodeStream(stream)
			if err != nil {
				data = stream.Data
			}
			return &SpectrallyDefinedColorSpace{Data: data}, nil
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

func parseFont(obj raw.Object, resolver rawResolver) (*Font, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("font is not a dict")
	}

	f := &Font{}
	if ref, ok := obj.(raw.Reference); ok {
		f.OriginalRef = ref.Ref()
	}

	if s, ok := dict.Get(raw.NameLiteral("Subtype")); ok {
		if name, ok := s.(raw.NameObj); ok {
			f.Subtype = name.Value()
		}
	}
	if s, ok := dict.Get(raw.NameLiteral("BaseFont")); ok {
		if name, ok := s.(raw.NameObj); ok {
			f.BaseFont = name.Value()
		}
	}
	if s, ok := dict.Get(raw.NameLiteral("Encoding")); ok {
		if name, ok := s.(raw.NameObj); ok {
			f.Encoding = name.Value()
		} else {
			// Handle Dictionary (or Reference to Dictionary)
			// Resolve if reference
			if ref, ok := s.(raw.Reference); ok {
				resolved, err := resolver.Resolve(ref.Ref())
				if err == nil {
					s = resolved
				}
			}

			if name, ok := s.(raw.NameObj); ok {
				f.Encoding = name.Value()
			} else if encDict, ok := resolveDict(s, resolver); ok {
				f.EncodingDict = parseEncodingDict(encDict, resolver)
			} else if stream, ok := s.(*raw.StreamObj); ok {
				// It's a Stream (CMap)
				data, err := decodeStream(stream)
				if err != nil {
					data = stream.Data
				}
				f.EncodingCMap = data
			}
		}
	}

	// Type0 DescendantFonts
	if f.Subtype == "Type0" {
		if df, ok := dict.Get(raw.NameLiteral("DescendantFonts")); ok {
			if arr, ok := resolveArray(df, resolver); ok && len(arr.Items) > 0 {
				if descDict, ok := resolveDict(arr.Items[0], resolver); ok {
					f.DescendantFont = parseCIDFont(descDict, resolver)
				}
			}
		}
		if tuObj, ok := dict.Get(raw.NameLiteral("ToUnicode")); ok {
			// Preserve ToUnicode CMap
			if ref, ok := tuObj.(raw.Reference); ok {
				resolved, err := resolver.Resolve(ref.Ref())
				if err == nil {
					tuObj = resolved
				}
			}
			if stream, ok := tuObj.(*raw.StreamObj); ok {
				data, err := decodeStream(stream)
				if err != nil {
					data = stream.Data
				}
				f.ToUnicodeCMap = data
			}
		}
	} else {
		// Simple Font Widths
		if wObj, ok := dict.Get(raw.NameLiteral("Widths")); ok {
			if arr, ok := resolveArray(wObj, resolver); ok {
				f.Widths = make(map[int]int)
				first := 0
				if fc, ok := dict.Get(raw.NameLiteral("FirstChar")); ok {
					if n, ok := fc.(raw.NumberObj); ok {
						first = int(n.Int())
					}
				}
				for i, item := range arr.Items {
					if n, ok := item.(raw.NumberObj); ok {
						f.Widths[first+i] = int(n.Int())
					}
				}
			}
		}
	}

	// FontDescriptor
	if fdObj, ok := dict.Get(raw.NameLiteral("FontDescriptor")); ok {
		if fdDict, ok := resolveDict(fdObj, resolver); ok {
			f.Descriptor = parseFontDescriptor(fdDict, resolver)
		}
	}

	return f, nil
}

func parseCIDFont(dict *raw.DictObj, resolver rawResolver) *CIDFont {
	cf := &CIDFont{}
	if s, ok := dict.Get(raw.NameLiteral("Subtype")); ok {
		if name, ok := s.(raw.NameObj); ok {
			cf.Subtype = name.Value()
		}
	}
	if s, ok := dict.Get(raw.NameLiteral("BaseFont")); ok {
		if name, ok := s.(raw.NameObj); ok {
			cf.BaseFont = name.Value()
		}
	}
	if dw, ok := dict.Get(raw.NameLiteral("DW")); ok {
		if n, ok := dw.(raw.NumberObj); ok {
			cf.DW = int(n.Int())
		}
	}
	// W array parsing (CID widths)
	if wObj, ok := dict.Get(raw.NameLiteral("W")); ok {
		if arr, ok := resolveArray(wObj, resolver); ok {
			cf.W = parseCIDWidths(arr)
		}
	}
	// CIDSystemInfo
	if csiObj, ok := dict.Get(raw.NameLiteral("CIDSystemInfo")); ok {
		if csiDict, ok := resolveDict(csiObj, resolver); ok {
			cf.CIDSystemInfo = parseCIDSystemInfo(csiDict)
		}
	}

	// CIDToGIDMap
	if mapObj, ok := dict.Get(raw.NameLiteral("CIDToGIDMap")); ok {
		// It can be a Name (e.g. /Identity) or a Stream
		if name, ok := mapObj.(raw.NameObj); ok {
			cf.CIDToGIDMapName = name.Value()
		} else {
			// Resolve if reference
			if ref, ok := mapObj.(raw.Reference); ok {
				resolved, err := resolver.Resolve(ref.Ref())
				if err == nil {
					mapObj = resolved
				}
			}
			if stream, ok := mapObj.(*raw.StreamObj); ok {
				data, err := decodeStream(stream)
				if err != nil {
					data = stream.Data
				}
				cf.CIDToGIDMap = data
			}
		}
	}

	// FontDescriptor
	if fdObj, ok := dict.Get(raw.NameLiteral("FontDescriptor")); ok {
		if fdDict, ok := resolveDict(fdObj, resolver); ok {
			cf.Descriptor = parseFontDescriptor(fdDict, resolver)
		}
	}
	return cf
}

func parseCIDWidths(arr *raw.ArrayObj) map[int]int {
	widths := make(map[int]int)
	for i := 0; i < len(arr.Items); {
		// format: c [w1 w2 ...] or c_first c_last w
		if i+1 >= len(arr.Items) {
			break
		}
		c1Obj, ok1 := arr.Items[i].(raw.NumberObj)
		if !ok1 {
			i++
			continue
		}
		c1 := int(c1Obj.Int())

		next := arr.Items[i+1]
		if list, ok := next.(*raw.ArrayObj); ok {
			// c [w1 w2 ...]
			for j, wObj := range list.Items {
				if w, ok := wObj.(raw.NumberObj); ok {
					widths[c1+j] = int(w.Int())
				}
			}
			i += 2
		} else if c2Obj, ok := next.(raw.NumberObj); ok {
			// c_first c_last w
			if i+2 >= len(arr.Items) {
				break
			}
			wObj, ok := arr.Items[i+2].(raw.NumberObj)
			if ok {
				c2 := int(c2Obj.Int())
				w := int(wObj.Int())
				for c := c1; c <= c2; c++ {
					widths[c] = w
				}
			}
			i += 3
		} else {
			i++
		}
	}
	return widths
}

func parseCIDSystemInfo(dict *raw.DictObj) CIDSystemInfo {
	csi := CIDSystemInfo{}
	if r, ok := dict.Get(raw.NameLiteral("Registry")); ok {
		if s, ok := r.(raw.StringObj); ok {
			csi.Registry = string(s.Value())
		}
	}
	if o, ok := dict.Get(raw.NameLiteral("Ordering")); ok {
		if s, ok := o.(raw.StringObj); ok {
			csi.Ordering = string(s.Value())
		}
	}
	if s, ok := dict.Get(raw.NameLiteral("Supplement")); ok {
		if n, ok := s.(raw.NumberObj); ok {
			csi.Supplement = int(n.Int())
		}
	}
	return csi
}

func parseEncodingDict(dict *raw.DictObj, resolver rawResolver) *EncodingDict {
	ed := &EncodingDict{}
	if base, ok := dict.Get(raw.NameLiteral("BaseEncoding")); ok {
		if name, ok := base.(raw.NameObj); ok {
			ed.BaseEncoding = name.Value()
		}
	}
	if diff, ok := dict.Get(raw.NameLiteral("Differences")); ok {
		if arr, ok := resolveArray(diff, resolver); ok {
			ed.Differences = parseEncodingDifferences(arr)
		}
	}
	return ed
}

func parseEncodingDifferences(arr *raw.ArrayObj) []EncodingDifference {
	var diffs []EncodingDifference
	currentCode := 0
	for _, item := range arr.Items {
		if n, ok := item.(raw.NumberObj); ok {
			currentCode = int(n.Int())
		} else if name, ok := item.(raw.NameObj); ok {
			diffs = append(diffs, EncodingDifference{
				Code: currentCode,
				Name: name.Value(),
			})
			currentCode++
		}
	}
	return diffs
}

func parseFontDescriptor(dict *raw.DictObj, resolver rawResolver) *FontDescriptor {
	fd := &FontDescriptor{}
	if n, ok := dict.Get(raw.NameLiteral("FontName")); ok {
		if name, ok := n.(raw.NameObj); ok {
			fd.FontName = name.Value()
		}
	}
	if f, ok := dict.Get(raw.NameLiteral("Flags")); ok {
		if n, ok := f.(raw.NumberObj); ok {
			fd.Flags = int(n.Int())
		}
	}
	if bb, ok := dict.Get(raw.NameLiteral("FontBBox")); ok {
		nums := parseNumberArray(bb)
		if len(nums) >= 4 {
			copy(fd.FontBBox[:], nums[:4])
		}
	}
	// Metrics
	if v, ok := dict.Get(raw.NameLiteral("ItalicAngle")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			fd.ItalicAngle = n.Float()
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("Ascent")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			fd.Ascent = n.Float()
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("Descent")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			fd.Descent = n.Float()
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("CapHeight")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			fd.CapHeight = n.Float()
		}
	}
	if v, ok := dict.Get(raw.NameLiteral("StemV")); ok {
		if n, ok := v.(raw.NumberObj); ok {
			fd.StemV = int(n.Int())
		}
	}

	// FontFile
	keys := []string{"FontFile", "FontFile2", "FontFile3"}
	for _, key := range keys {
		if ffObj, ok := dict.Get(raw.NameLiteral(key)); ok {
			// Resolve stream
			if ref, ok := ffObj.(raw.Reference); ok {
				resolved, err := resolver.Resolve(ref.Ref())
				if err == nil {
					ffObj = resolved
				}
			}
			if stream, ok := ffObj.(*raw.StreamObj); ok {
				data, err := decodeStream(stream)
				if err != nil {
					// Warning: failed to decode font stream
					data = stream.Data
				}
				fd.FontFile = data
				fd.FontFileType = key
				// Subtype
				if s, ok := stream.Dict.Get(raw.NameLiteral("Subtype")); ok {
					if name, ok := s.(raw.NameObj); ok {
						fd.FontFileSubtype = name.Value()
					}
				}
				// Lengths
				if l1, ok := stream.Dict.Get(raw.NameLiteral("Length1")); ok {
					if n, ok := l1.(raw.NumberObj); ok {
						fd.Length1 = int(n.Int())
					}
				}
				if l2, ok := stream.Dict.Get(raw.NameLiteral("Length2")); ok {
					if n, ok := l2.(raw.NumberObj); ok {
						fd.Length2 = int(n.Int())
					}
				}
				if l3, ok := stream.Dict.Get(raw.NameLiteral("Length3")); ok {
					if n, ok := l3.(raw.NumberObj); ok {
						fd.Length3 = int(n.Int())
					}
				}
				break
			}
		}
	}

	return fd
}

func parseXObject(obj raw.Object, resolver rawResolver) (*XObject, error) {
	// Resolve
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	stream, ok := obj.(*raw.StreamObj)
	if !ok {
		return nil, fmt.Errorf("XObject is not a stream")
	}
	dict := stream.Dict

	data, err := decodeStream(stream)
	if err != nil {
		// Warning: failed to decode XObject stream
		data = stream.Data
	}

	xo := &XObject{Data: data}

	if s, ok := dict.Get(raw.NameLiteral("Subtype")); ok {
		if name, ok := s.(raw.NameObj); ok {
			xo.Subtype = name.Value()
		}
	}

	if xo.Subtype == "Image" {
		if w, ok := dict.Get(raw.NameLiteral("Width")); ok {
			if n, ok := w.(raw.NumberObj); ok {
				xo.Width = int(n.Int())
			}
		}
		if h, ok := dict.Get(raw.NameLiteral("Height")); ok {
			if n, ok := h.(raw.NumberObj); ok {
				xo.Height = int(n.Int())
			}
		}
		if bpc, ok := dict.Get(raw.NameLiteral("BitsPerComponent")); ok {
			if n, ok := bpc.(raw.NumberObj); ok {
				xo.BitsPerComponent = int(n.Int())
			}
		}
		if cs, ok := dict.Get(raw.NameLiteral("ColorSpace")); ok {
			if c, err := parseColorSpace(cs, resolver); err == nil {
				xo.ColorSpace = c
			}
		}
		if i, ok := dict.Get(raw.NameLiteral("Interpolate")); ok {
			if b, ok := i.(raw.BoolObj); ok {
				xo.Interpolate = b.Value()
			}
		}
		if sm, ok := dict.Get(raw.NameLiteral("SMask")); ok {
			// Recursive call for SMask XObject
			if smXo, err := parseXObject(sm, resolver); err == nil {
				xo.SMask = smXo
			}
		}
	} else if xo.Subtype == "Form" {
		if bb, ok := dict.Get(raw.NameLiteral("BBox")); ok {
			if rect := parseRectangleFromObj(bb); rect != nil {
				xo.BBox = *rect
			}
		}
		if m, ok := dict.Get(raw.NameLiteral("Matrix")); ok {
			xo.Matrix = parseNumberArray(m)
		}
		if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
			if r, err := parseResources(res, resolver); err == nil {
				xo.Resources = r
			}
		}
		if g, ok := dict.Get(raw.NameLiteral("Group")); ok {
			if gDict, ok := resolveDict(g, resolver); ok {
				xo.Group = parseTransparencyGroup(gDict, resolver)
			}
		}
	}

	return xo, nil
}

func parseTransparencyGroup(dict *raw.DictObj, resolver rawResolver) *TransparencyGroup {
	tg := &TransparencyGroup{}
	if cs, ok := dict.Get(raw.NameLiteral("CS")); ok {
		if c, err := parseColorSpace(cs, resolver); err == nil {
			tg.CS = c
		}
	}
	if i, ok := dict.Get(raw.NameLiteral("I")); ok {
		if b, ok := i.(raw.BoolObj); ok {
			tg.Isolated = b.Value()
		}
	}
	if k, ok := dict.Get(raw.NameLiteral("K")); ok {
		if b, ok := k.(raw.BoolObj); ok {
			tg.Knockout = b.Value()
		}
	}
	return tg
}

func parsePattern(obj raw.Object, resolver rawResolver) (Pattern, error) {
	// Resolve
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("pattern is not a dict")
	}

	pt := 0
	if t, ok := dict.Get(raw.NameLiteral("PatternType")); ok {
		if n, ok := t.(raw.NumberObj); ok {
			pt = int(n.Int())
		}
	}

	if pt == 1 {
		// Tiling
		tp := &TilingPattern{BasePattern: BasePattern{Type: 1}}
		if stream, ok := obj.(*raw.StreamObj); ok {
			tp.Content = stream.Data
		}
		if paint, ok := dict.Get(raw.NameLiteral("PaintType")); ok {
			if n, ok := paint.(raw.NumberObj); ok {
				tp.PaintType = int(n.Int())
			}
		}
		if tiling, ok := dict.Get(raw.NameLiteral("TilingType")); ok {
			if n, ok := tiling.(raw.NumberObj); ok {
				tp.TilingType = int(n.Int())
			}
		}
		if bb, ok := dict.Get(raw.NameLiteral("BBox")); ok {
			if rect := parseRectangleFromObj(bb); rect != nil {
				tp.BBox = *rect
			}
		}
		if xs, ok := dict.Get(raw.NameLiteral("XStep")); ok {
			if n, ok := xs.(raw.NumberObj); ok {
				tp.XStep = n.Float()
			}
		}
		if ys, ok := dict.Get(raw.NameLiteral("YStep")); ok {
			if n, ok := ys.(raw.NumberObj); ok {
				tp.YStep = n.Float()
			}
		}
		if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
			if r, err := parseResources(res, resolver); err == nil {
				tp.Resources = r
			}
		}
		return tp, nil
	} else if pt == 2 {
		// Shading Pattern
		sp := &ShadingPattern{BasePattern: BasePattern{Type: 2}}
		if sh, ok := dict.Get(raw.NameLiteral("Shading")); ok {
			if s, err := parseShading(sh, resolver); err == nil {
				sp.Shading = s
			}
		}
		if gs, ok := dict.Get(raw.NameLiteral("ExtGState")); ok {
			if g, err := parseExtGState(gs, resolver); err == nil {
				sp.ExtGState = g
			}
		}
		return sp, nil
	}

	return nil, fmt.Errorf("unknown pattern type %d", pt)
}

func parseShading(obj raw.Object, resolver rawResolver) (Shading, error) {
	// Resolve
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("shading is not a dict")
	}

	st := 0
	if t, ok := dict.Get(raw.NameLiteral("ShadingType")); ok {
		if n, ok := t.(raw.NumberObj); ok {
			st = int(n.Int())
		}
	}

	var cs ColorSpace
	if c, ok := dict.Get(raw.NameLiteral("ColorSpace")); ok {
		if parsed, err := parseColorSpace(c, resolver); err == nil {
			cs = parsed
		}
	}

	base := BaseShading{Type: st, ColorSpace: cs}
	if bb, ok := dict.Get(raw.NameLiteral("BBox")); ok {
		if rect := parseRectangleFromObj(bb); rect != nil {
			base.BBox = *rect
		}
	}
	if aa, ok := dict.Get(raw.NameLiteral("AntiAlias")); ok {
		if b, ok := aa.(raw.BoolObj); ok {
			base.AntiAlias = b.Value()
		}
	}

	if st >= 1 && st <= 3 {
		// Function based
		fs := &FunctionShading{BaseShading: base}
		if c, ok := dict.Get(raw.NameLiteral("Coords")); ok {
			fs.Coords = parseNumberArray(c)
		}
		if d, ok := dict.Get(raw.NameLiteral("Domain")); ok {
			fs.Domain = parseNumberArray(d)
		}
		if e, ok := dict.Get(raw.NameLiteral("Extend")); ok {
			if arr, ok := resolveArray(e, resolver); ok {
				for _, item := range arr.Items {
					if b, ok := item.(raw.BoolObj); ok {
						fs.Extend = append(fs.Extend, b.Value())
					}
				}
			}
		}
		if _, ok := dict.Get(raw.NameLiteral("Function")); ok {
			// Parse function (simplified: skip for now or implement parseFunction)
			// We need parseFunction to support shadings fully.
		}
		return fs, nil
	} else if st >= 4 && st <= 7 {
		// Mesh based
		ms := &MeshShading{BaseShading: base}
		if stream, ok := obj.(*raw.StreamObj); ok {
			ms.Stream = stream.Data
		}
		if bpc, ok := dict.Get(raw.NameLiteral("BitsPerCoordinate")); ok {
			if n, ok := bpc.(raw.NumberObj); ok {
				ms.BitsPerCoordinate = int(n.Int())
			}
		}
		if bpc, ok := dict.Get(raw.NameLiteral("BitsPerComponent")); ok {
			if n, ok := bpc.(raw.NumberObj); ok {
				ms.BitsPerComponent = int(n.Int())
			}
		}
		if bpf, ok := dict.Get(raw.NameLiteral("BitsPerFlag")); ok {
			if n, ok := bpf.(raw.NumberObj); ok {
				ms.BitsPerFlag = int(n.Int())
			}
		}
		if d, ok := dict.Get(raw.NameLiteral("Decode")); ok {
			ms.Decode = parseNumberArray(d)
		}
		return ms, nil
	}

	return nil, fmt.Errorf("unknown shading type %d", st)
}

func parsePropertyList(obj raw.Object, resolver rawResolver) (PropertyList, error) {
	// Resolve
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("property list is not a dict")
	}

	typ := ""
	if t, ok := dict.Get(raw.NameLiteral("Type")); ok {
		if name, ok := t.(raw.NameObj); ok {
			typ = name.Value()
		}
	}

	if typ == "OCG" {
		ocg := &OptionalContentGroup{}
		if n, ok := dict.Get(raw.NameLiteral("Name")); ok {
			if s, ok := n.(raw.StringObj); ok {
				ocg.Name = string(s.Value())
			}
		}
		if i, ok := dict.Get(raw.NameLiteral("Intent")); ok {
			if name, ok := i.(raw.NameObj); ok {
				ocg.Intent = []string{name.Value()}
			} else if arr, ok := resolveArray(i, resolver); ok {
				for _, item := range arr.Items {
					if name, ok := item.(raw.NameObj); ok {
						ocg.Intent = append(ocg.Intent, name.Value())
					}
				}
			}
		}
		return ocg, nil
	} else if typ == "OCMD" {
		ocmd := &OptionalContentMembership{}
		if p, ok := dict.Get(raw.NameLiteral("P")); ok {
			if name, ok := p.(raw.NameObj); ok {
				ocmd.Policy = name.Value()
			}
		}
		if _, ok := dict.Get(raw.NameLiteral("OCGs")); ok {
			// Parse OCGs (recursive or reference)
			// Simplified: skip deep parsing for now
		}
		return ocmd, nil
	}

	return nil, fmt.Errorf("unknown property list type %s", typ)
}
