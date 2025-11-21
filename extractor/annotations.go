package extractor

import "github.com/wudi/pdfkit/ir/raw"

// AnnotationInfo summarizes a page annotation.
type AnnotationInfo struct {
	Page     int
	Subtype  string
	Rect     [4]float64
	Contents string
	URI      string
	Flags    int
	Color    []float64
}

// ExtractAnnotations returns annotations found across all pages.
func (e *Extractor) ExtractAnnotations() ([]AnnotationInfo, error) {
	var annots []AnnotationInfo
	for idx, page := range e.pages {
		arr := derefArray(e.raw, valueFromDict(page, "Annots"))
		if arr == nil {
			continue
		}
		for _, obj := range arr.Items {
			dict := derefDict(e.raw, obj)
			if dict == nil {
				continue
			}
			info := AnnotationInfo{Page: idx}
			info.Subtype, _ = nameFromDict(dict, "Subtype")
			info.Rect = rectFromArray(derefArray(e.raw, valueFromDict(dict, "Rect")))
			info.Contents, _ = stringFromObject(valueFromDict(dict, "Contents"))
			if flags, ok := intFromObject(valueFromDict(dict, "F")); ok {
				info.Flags = flags
			}
			if color := derefArray(e.raw, valueFromDict(dict, "C")); color != nil {
				info.Color = extractFloatArray(color)
			}
			info.URI = extractAnnotationURI(e.raw, dict)
			annots = append(annots, info)
		}
	}
	return annots, nil
}

func rectFromArray(arr *raw.ArrayObj) [4]float64 {
	var rect [4]float64
	if arr == nil {
		return rect
	}
	for i := 0; i < len(arr.Items) && i < 4; i++ {
		if val, ok := floatFromObject(arr.Items[i]); ok {
			rect[i] = val
		}
	}
	return rect
}

func extractFloatArray(arr *raw.ArrayObj) []float64 {
	if arr == nil {
		return nil
	}
	out := make([]float64, 0, len(arr.Items))
	for _, item := range arr.Items {
		if val, ok := floatFromObject(item); ok {
			out = append(out, val)
		}
	}
	return out
}

func extractAnnotationURI(doc *raw.Document, dict *raw.DictObj) string {
	if dict == nil {
		return ""
	}
	if uri, ok := stringFromDict(dict, "URI"); ok {
		return uri
	}
	action := derefDict(doc, valueFromDict(dict, "A"))
	if action == nil {
		return ""
	}
	if typ, ok := nameFromDict(action, "S"); ok && typ == "URI" {
		if uri, ok := stringFromDict(action, "URI"); ok {
			return uri
		}
	}
	return ""
}
