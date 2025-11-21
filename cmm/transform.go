package cmm

import (
	"errors"
	"fmt"
)

type basicTransform struct {
	src, dst Profile
	intent   RenderingIntent
}

func (t *basicTransform) Convert(in []float64) ([]float64, error) {
	// 1. Validate input dimension
	srcCh := numChannels(t.src.ColorSpace())
	if len(in) != srcCh {
		return nil, fmt.Errorf("input channels mismatch: expected %d, got %d", srcCh, len(in))
	}

	// 2. Simple conversions (fallback)
	// If we had a real CMM, we would use the LUTs here.
	// For now, we implement basic conversions between standard spaces if possible.

	dstCh := numChannels(t.dst.ColorSpace())
	out := make([]float64, dstCh)

	// TODO: Implement actual color conversion logic.
	// For now, we just handle simple cases or return error.

	if t.src.ColorSpace() == "RGB " && t.dst.ColorSpace() == "CMYK" {
		// Simple RGB -> CMYK
		r, g, b := in[0], in[1], in[2]
		k := 1.0 - max(r, max(g, b))
		if k < 1.0 {
			out[0] = (1.0 - r - k) / (1.0 - k) // C
			out[1] = (1.0 - g - k) / (1.0 - k) // M
			out[2] = (1.0 - b - k) / (1.0 - k) // Y
		}
		out[3] = k
		return out, nil
	}

	if t.src.ColorSpace() == "CMYK" && t.dst.ColorSpace() == "RGB " {
		// Simple CMYK -> RGB
		c, m, y, k := in[0], in[1], in[2], in[3]
		out[0] = (1.0 - c) * (1.0 - k)
		out[1] = (1.0 - m) * (1.0 - k)
		out[2] = (1.0 - y) * (1.0 - k)
		return out, nil
	}

	// If dimensions match, just copy (dangerous assumption, but better than crash for now)
	if srcCh == dstCh {
		copy(out, in)
		return out, nil
	}

	return nil, errors.New("unsupported color conversion")
}

func numChannels(cs string) int {
	switch cs {
	case "RGB ":
		return 3
	case "CMYK":
		return 4
	case "GRAY":
		return 1
	case "Lab ":
		return 3
	case "XYZ ":
		return 3
	default:
		return 0
	}
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
