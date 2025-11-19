package extractor

import (
	"sort"

	"pdflib/ir/decoded"
	"pdflib/ir/raw"
)

// FontInfo groups font dictionaries referenced throughout the document.
type FontInfo struct {
	ResourceName string
	BaseFont     string
	Subtype      string
	Encoding     string
	HasToUnicode bool
	Pages        []int
}

// ExtractFonts reports the distinct fonts referenced by pages and their usage.
func (e *Extractor) ExtractFonts() []FontInfo {
	fontMap := make(map[*raw.DictObj]*FontInfo)
	for idx, page := range e.pages {
		resDict := derefDict(e.raw, valueFromDict(page, "Resources"))
		if resDict == nil {
			continue
		}
		fontDict := derefDict(e.raw, valueFromDict(resDict, "Font"))
		if fontDict == nil {
			continue
		}
		for name, obj := range fontDict.KV {
			dict := derefDict(e.raw, obj)
			if dict == nil {
				continue
			}
			info, ok := fontMap[dict]
			if !ok {
				baseFont, _ := nameFromDict(dict, "BaseFont")
				subtype, _ := nameFromDict(dict, "Subtype")
				encoding, _ := nameFromDict(dict, "Encoding")
				info = &FontInfo{
					ResourceName: name,
					BaseFont:     baseFont,
					Subtype:      subtype,
					Encoding:     encoding,
					HasToUnicode: hasStream(e.dec, valueFromDict(dict, "ToUnicode")),
				}
				fontMap[dict] = info
			}
			if !containsInt(info.Pages, idx) {
				info.Pages = append(info.Pages, idx)
			}
		}
	}
	out := make([]FontInfo, 0, len(fontMap))
	for _, info := range fontMap {
		sort.Ints(info.Pages)
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BaseFont == out[j].BaseFont {
			return out[i].ResourceName < out[j].ResourceName
		}
		return out[i].BaseFont < out[j].BaseFont
	})
	return out
}

func containsInt(values []int, v int) bool {
	for _, existing := range values {
		if existing == v {
			return true
		}
	}
	return false
}

func hasStream(dec *decoded.DecodedDocument, obj raw.Object) bool {
	if dec == nil || obj == nil {
		return false
	}
	switch v := obj.(type) {
	case raw.RefObj:
		_, _, _, ok := streamData(dec, v)
		return ok
	case raw.Stream:
		return true
	}
	return false
}
