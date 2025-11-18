package filters

import (
	"errors"
	"image"
)

// jbig2MonochromeToNRGBA expands 1bpp JBIG2 image data into opaque NRGBA pixels.
func jbig2MonochromeToNRGBA(width, height, stride int, data []byte) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid JBIG2 dimensions")
	}
	if stride <= 0 {
		return nil, errors.New("invalid JBIG2 stride")
	}
	if len(data) < stride*height {
		return nil, errors.New("JBIG2 data truncated")
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		row := data[y*stride : y*stride+stride]
		dst := img.Pix[y*img.Stride : y*img.Stride+width*4]
		for x := 0; x < width; x++ {
			byteIdx := x / 8
			bit := 7 - (x % 8)
			if byteIdx >= len(row) {
				return nil, errors.New("JBIG2 row underrun")
			}
			set := row[byteIdx]&(1<<bit) != 0
			val := byte(255)
			if set {
				val = 0
			}
			dst[4*x] = val
			dst[4*x+1] = val
			dst[4*x+2] = val
			dst[4*x+3] = 255
		}
	}
	return img.Pix, nil
}
