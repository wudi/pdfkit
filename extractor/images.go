package extractor

// ImageAsset represents an image XObject found on a page.
type ImageAsset struct {
	Page             int
	ResourceName     string
	Width            int
	Height           int
	BitsPerComponent int
	ColorSpace       string
	Data             []byte
}

// ExtractImages walks page resources and returns embedded image XObjects.
func (e *Extractor) ExtractImages() ([]ImageAsset, error) {
	var assets []ImageAsset
	for idx, page := range e.pages {
		resDict := derefDict(e.raw, valueFromDict(page, "Resources"))
		if resDict == nil {
			continue
		}
		xobjects := derefDict(e.raw, valueFromDict(resDict, "XObject"))
		if xobjects == nil {
			continue
		}
		for name, obj := range xobjects.KV {
			data, dict, ok := streamData(e.dec, obj)
			if !ok || dict == nil {
				continue
			}
			subtype, _ := nameFromDict(dict, "Subtype")
			if subtype != "Image" {
				continue
			}
			width, _ := intFromObject(valueFromDict(dict, "Width"))
			height, _ := intFromObject(valueFromDict(dict, "Height"))
			bpc, _ := intFromObject(valueFromDict(dict, "BitsPerComponent"))
			color, _ := nameFromDict(dict, "ColorSpace")
			asset := ImageAsset{
				Page:             idx,
				ResourceName:     name,
				Width:            width,
				Height:           height,
				BitsPerComponent: bpc,
				ColorSpace:       color,
				Data:             data,
			}
			assets = append(assets, asset)
		}
	}
	return assets, nil
}
