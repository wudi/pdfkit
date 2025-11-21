package filters

import "github.com/wudi/pdfkit/ir/raw"

// ExtractFilters reads Filter and DecodeParms entries from a stream dictionary.
func ExtractFilters(dict raw.Dictionary) ([]string, []raw.Dictionary) {
	var names []string
	var params []raw.Dictionary

	filterObj, ok := dict.Get(raw.NameObj{Val: "Filter"})
	if !ok {
		return names, params
	}

	switch f := filterObj.(type) {
	case raw.Name:
		names = append(names, f.Value())
	case *raw.ArrayObj:
		for _, item := range f.Items {
			if n, ok := item.(raw.Name); ok {
				names = append(names, n.Value())
			}
		}
	}

	if len(names) > 0 {
		if pObj, ok := dict.Get(raw.NameObj{Val: "DecodeParms"}); ok {
			switch p := pObj.(type) {
			case raw.Dictionary:
				params = append(params, p)
			case *raw.ArrayObj:
				for _, item := range p.Items {
					if d, ok := item.(raw.Dictionary); ok {
						params = append(params, d)
					}
				}
			}
		}
	}

	return names, params
}
