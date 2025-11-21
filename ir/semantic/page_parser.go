package semantic

import (
	"fmt"

	"context"
	"pdflib/filters"
	"pdflib/geo"
	"pdflib/ir/raw"
)

type inheritedPageProps struct {
	MediaBox  *Rectangle
	CropBox   *Rectangle
	Rotate    *int
	Resources raw.Object
}

// parsePages traverses the page tree and returns a flat list of pages.
func parsePages(obj raw.Object, resolver rawResolver, inherited inheritedPageProps) ([]*Page, error) {
	// Resolve indirect reference
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	dict, ok := obj.(*raw.DictObj)
	if !ok {
		return nil, fmt.Errorf("pages object is not a dictionary")
	}

	// Update inherited props
	newInherited := inherited

	if mbObj, ok := dict.Get(raw.NameLiteral("MediaBox")); ok {
		if mb := parseRectangleFromObj(mbObj); mb != nil {
			newInherited.MediaBox = mb
		}
	}
	if cbObj, ok := dict.Get(raw.NameLiteral("CropBox")); ok {
		if cb := parseRectangleFromObj(cbObj); cb != nil {
			newInherited.CropBox = cb
		}
	}
	if rotObj, ok := dict.Get(raw.NameLiteral("Rotate")); ok {
		if r, ok := rotObj.(raw.NumberObj); ok {
			val := int(r.I)
			newInherited.Rotate = &val
		}
	}
	if resObj, ok := dict.Get(raw.NameLiteral("Resources")); ok {
		newInherited.Resources = resObj
	}

	typeVal, ok := dict.Get(raw.NameLiteral("Type"))
	isPage := false
	if ok {
		if name, ok := typeVal.(raw.NameObj); ok {
			if name.Value() == "Page" {
				isPage = true
			}
		}
	} else {
		// Infer from Kids presence
		if _, hasKids := dict.Get(raw.NameLiteral("Kids")); !hasKids {
			isPage = true
		}
	}

	if isPage {
		page, err := parsePage(dict, resolver, newInherited)
		if err != nil {
			return nil, err
		}
		return []*Page{page}, nil
	}

	// It's a Pages node
	kidsObj, ok := dict.Get(raw.NameLiteral("Kids"))
	if !ok {
		return nil, fmt.Errorf("pages node missing Kids")
	}

	kidsArr, ok := resolveArray(kidsObj, resolver)
	if !ok {
		return nil, fmt.Errorf("Kids is not an array")
	}

	var pages []*Page
	for _, kid := range kidsArr.Items {
		subPages, err := parsePages(kid, resolver, newInherited)
		if err != nil {
			// Warning: failed to parse kid
			continue
		}
		pages = append(pages, subPages...)
	}

	return pages, nil
}

func parsePage(dict *raw.DictObj, resolver rawResolver, inherited inheritedPageProps) (*Page, error) {
	page := &Page{}

	// MediaBox
	if mbObj, ok := dict.Get(raw.NameLiteral("MediaBox")); ok {
		if mb := parseRectangleFromObj(mbObj); mb != nil {
			page.MediaBox = *mb
		}
	} else if inherited.MediaBox != nil {
		page.MediaBox = *inherited.MediaBox
	} else {
		// Default MediaBox? A4?
		page.MediaBox = Rectangle{0, 0, 612, 792} // Letter default
	}

	// CropBox
	if cbObj, ok := dict.Get(raw.NameLiteral("CropBox")); ok {
		if cb := parseRectangleFromObj(cbObj); cb != nil {
			page.CropBox = *cb
		}
	} else if inherited.CropBox != nil {
		page.CropBox = *inherited.CropBox
	} else {
		page.CropBox = page.MediaBox
	}

	// Rotate
	if rotObj, ok := dict.Get(raw.NameLiteral("Rotate")); ok {
		if r, ok := rotObj.(raw.NumberObj); ok {
			page.Rotate = int(r.I)
		}
	} else if inherited.Rotate != nil {
		page.Rotate = *inherited.Rotate
	}

	// Resources
	if resObj, ok := dict.Get(raw.NameLiteral("Resources")); ok {
		res, err := parseResources(resObj, resolver)
		if err == nil {
			page.Resources = res
		} else {
			// Warning: failed to parse resources
		}
	} else if inherited.Resources != nil {
		res, err := parseResources(inherited.Resources, resolver)
		if err == nil {
			page.Resources = res
		}
	}

	// Contents
	if contentsObj, ok := dict.Get(raw.NameLiteral("Contents")); ok {
		streams, err := parseContentStreams(contentsObj, resolver)
		if err != nil {
			// Warning: failed to parse content streams
		} else {
			page.Contents = streams
		}
	}

	// Parse Viewports
	if vpObj, ok := dict.Get(raw.NameLiteral("VP")); ok {
		vps, err := parseViewports(vpObj, resolver)
		if err != nil {
			// Warning: failed to parse viewports
		} else {
			page.Viewports = vps
		}
	}

	// Parse OutputIntents (PDF 2.0)
	if oiObj, ok := dict.Get(raw.NameLiteral("OutputIntents")); ok {
		ois, err := parseOutputIntents(oiObj, resolver)
		if err != nil {
			// Warning: failed to parse page OutputIntents
		} else {
			page.OutputIntents = ois
		}
	}

	return page, nil
}

func parseOutputIntents(obj raw.Object, resolver rawResolver) ([]OutputIntent, error) {
	arr, ok := resolveArray(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("OutputIntents is not an array")
	}

	var intents []OutputIntent
	for _, item := range arr.Items {
		dict, ok := resolveDict(item, resolver)
		if !ok {
			continue
		}

		oi := OutputIntent{}

		if s, ok := dict.Get(raw.NameLiteral("S")); ok {
			if n, ok := s.(raw.NameObj); ok {
				oi.S = n.Value()
			}
		}

		if oci, ok := dict.Get(raw.NameLiteral("OutputConditionIdentifier")); ok {
			if s, ok := oci.(raw.StringObj); ok {
				oi.OutputConditionIdentifier = string(s.Value())
			}
		}

		if info, ok := dict.Get(raw.NameLiteral("Info")); ok {
			if s, ok := info.(raw.StringObj); ok {
				oi.Info = string(s.Value())
			}
		}

		if dest, ok := dict.Get(raw.NameLiteral("DestOutputProfile")); ok {
			// DestOutputProfile is a stream
			if ref, ok := dest.(raw.Reference); ok {
				resolved, err := resolver.Resolve(ref.Ref())
				if err == nil {
					if stream, ok := resolved.(*raw.StreamObj); ok {
						oi.DestOutputProfile = stream.Data
					}
				}
			} else if stream, ok := dest.(*raw.StreamObj); ok {
				oi.DestOutputProfile = stream.Data
			}
		}

		intents = append(intents, oi)
	}
	return intents, nil
}

func parseViewports(obj raw.Object, resolver rawResolver) ([]geo.Viewport, error) {
	// Resolve
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	arr, ok := obj.(*raw.ArrayObj)
	if !ok {
		// If it's a dict, treat as single item array
		if dict, ok := obj.(*raw.DictObj); ok {
			arr = &raw.ArrayObj{Items: []raw.Object{dict}}
		} else {
			return nil, fmt.Errorf("VP entry is not an array or dict")
		}
	}

	var viewports []geo.Viewport
	for _, item := range arr.Items {
		vpDict, ok := resolveDict(item, resolver)
		if !ok {
			continue
		}

		vp := geo.Viewport{}

		// BBox
		if bboxObj, ok := vpDict.Get(raw.NameLiteral("BBox")); ok {
			vp.BBox = parseNumberArray(bboxObj)
		}

		// Name
		if nameObj, ok := vpDict.Get(raw.NameLiteral("Name")); ok {
			if s, ok := nameObj.(raw.StringObj); ok {
				vp.Name = string(s.Value())
			} else if n, ok := nameObj.(raw.NameObj); ok {
				vp.Name = string(n.Value())
			}
		}

		// Measure
		if measureObj, ok := vpDict.Get(raw.NameLiteral("Measure")); ok {
			m, err := parseMeasure(measureObj, resolver)
			if err == nil {
				vp.Measure = m
			}
		}

		viewports = append(viewports, vp)
	}
	return viewports, nil
}

func parseMeasure(obj raw.Object, resolver rawResolver) (*geo.Measure, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("measure is not a dict")
	}

	m := &geo.Measure{}

	// Subtype
	if s, ok := dict.Get(raw.NameLiteral("Subtype")); ok {
		if name, ok := s.(raw.NameObj); ok {
			m.Subtype = string(name.Value())
		}
	}

	// Bounds
	if b, ok := dict.Get(raw.NameLiteral("Bounds")); ok {
		m.Bounds = parseNumberArray(b)
	}

	// GPTS
	if g, ok := dict.Get(raw.NameLiteral("GPTS")); ok {
		m.GPTS = parseNumberArray(g)
	}

	// LPTS
	if l, ok := dict.Get(raw.NameLiteral("LPTS")); ok {
		m.LPTS = parseNumberArray(l)
	}

	// GCS
	if gcsObj, ok := dict.Get(raw.NameLiteral("GCS")); ok {
		gcs, err := parseGCS(gcsObj, resolver)
		if err == nil {
			m.GCS = gcs
		}
	}

	return m, nil
}

func parseGCS(obj raw.Object, resolver rawResolver) (*geo.CoordinateSystem, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("GCS is not a dict")
	}

	gcs := &geo.CoordinateSystem{}

	if t, ok := dict.Get(raw.NameLiteral("Type")); ok {
		if name, ok := t.(raw.NameObj); ok {
			gcs.Type = string(name.Value())
		}
	}

	if wkt, ok := dict.Get(raw.NameLiteral("WKT")); ok {
		if s, ok := wkt.(raw.StringObj); ok {
			gcs.WKT = string(s.Value())
		}
	}

	if epsg, ok := dict.Get(raw.NameLiteral("EPSG")); ok {
		if n, ok := epsg.(raw.NumberObj); ok {
			if n.IsInt {
				gcs.EPSG = int(n.I)
			}
		}
	}

	return gcs, nil
}

// Helper to resolve to array
func resolveArray(obj raw.Object, resolver rawResolver) (*raw.ArrayObj, bool) {
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, false
		}
		obj = resolved
	}
	arr, ok := obj.(*raw.ArrayObj)
	return arr, ok
}

func parseNumberArray(obj raw.Object) []float64 {
	arr, ok := obj.(*raw.ArrayObj)
	if !ok {
		return nil
	}
	var nums []float64
	for _, item := range arr.Items {
		if n, ok := item.(raw.NumberObj); ok {
			if n.IsInt {
				nums = append(nums, float64(n.I))
			} else {
				nums = append(nums, n.F)
			}
		}
	}
	return nums
}

func parseRectangleFromObj(obj raw.Object) *Rectangle {
	nums := parseNumberArray(obj)
	if len(nums) < 4 {
		return nil
	}
	return &Rectangle{LLX: nums[0], LLY: nums[1], URX: nums[2], URY: nums[3]}
}

func parseContentStreams(obj raw.Object, resolver rawResolver) ([]ContentStream, error) {
	// Resolve if reference
	if ref, ok := obj.(raw.Reference); ok {
		resolved, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		obj = resolved
	}

	var streams []ContentStream

	// Single Stream
	if stream, ok := obj.(*raw.StreamObj); ok {
		data, err := decodeStream(stream)
		if err != nil {
			// Warning: failed to decode stream
			data = stream.Data
		}
		streams = append(streams, ContentStream{RawBytes: data})
		return streams, nil
	}

	// Array of Streams
	if arr, ok := obj.(*raw.ArrayObj); ok {
		for _, item := range arr.Items {
			// Resolve item
			if ref, ok := item.(raw.Reference); ok {
				resolved, err := resolver.Resolve(ref.Ref())
				if err != nil {
					return nil, err
				}
				item = resolved
			}

			if stream, ok := item.(*raw.StreamObj); ok {
				data, err := decodeStream(stream)
				if err != nil {
					// Warning: failed to decode stream
					data = stream.Data
				}
				streams = append(streams, ContentStream{RawBytes: data})
			}
		}
		return streams, nil
	}

	return nil, fmt.Errorf("Contents is not a stream or array, got %T", obj)
}

func decodeStream(stream *raw.StreamObj) ([]byte, error) {
	filterObj, ok := stream.Dict.Get(raw.NameLiteral("Filter"))
	if !ok {
		return stream.Data, nil
	}

	var filterNames []string
	if name, ok := filterObj.(raw.NameObj); ok {
		filterNames = []string{name.Value()}
	} else if arr, ok := filterObj.(*raw.ArrayObj); ok {
		for _, item := range arr.Items {
			if name, ok := item.(raw.NameObj); ok {
				filterNames = append(filterNames, name.Value())
			}
		}
	}

	if len(filterNames) == 0 {
		return stream.Data, nil
	}

	var params []raw.Dictionary
	if paramObj, ok := stream.Dict.Get(raw.NameLiteral("DecodeParms")); ok {
		if dict, ok := paramObj.(*raw.DictObj); ok {
			params = []raw.Dictionary{dict}
		} else if arr, ok := paramObj.(*raw.ArrayObj); ok {
			for _, item := range arr.Items {
				if dict, ok := item.(*raw.DictObj); ok {
					params = append(params, dict)
				} else {
					params = append(params, nil)
				}
			}
		}
	}

	pipeline := filters.NewPipeline([]filters.Decoder{
		filters.NewFlateDecoder(),
		filters.NewASCII85Decoder(),
		filters.NewASCIIHexDecoder(),
		filters.NewLZWDecoder(),
		filters.NewRunLengthDecoder(),
	}, filters.Limits{MaxDecompressedSize: 100 * 1024 * 1024})

	return pipeline.Decode(context.Background(), stream.Data, filterNames, params)
}
