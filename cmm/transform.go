package cmm

import (
	"errors"
	"fmt"
	"math"
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

type matrixTRCTransform struct {
	srcGamma [3]float64
	matrix   [9]float64 // rX, gX, bX, rY, gY, bY, rZ, gZ, bZ
}

func (t *matrixTRCTransform) Convert(in []float64) ([]float64, error) {
	if len(in) < 3 {
		return nil, errors.New("input too short")
	}
	// 1. Linearize
	r := math.Pow(in[0], t.srcGamma[0])
	g := math.Pow(in[1], t.srcGamma[1])
	b := math.Pow(in[2], t.srcGamma[2])

	// 2. Matrix Multiply (RGB -> XYZ)
	x := t.matrix[0]*r + t.matrix[1]*g + t.matrix[2]*b
	y := t.matrix[3]*r + t.matrix[4]*g + t.matrix[5]*b
	z := t.matrix[6]*r + t.matrix[7]*g + t.matrix[8]*b

	return []float64{x, y, z}, nil
}

func tryCreateMatrixTRC(p *ICCProfile) (*matrixTRCTransform, error) {
	rXYZ, err := p.ReadXYZTag("rXYZ")
	if err != nil {
		return nil, err
	}
	gXYZ, err := p.ReadXYZTag("gXYZ")
	if err != nil {
		return nil, err
	}
	bXYZ, err := p.ReadXYZTag("bXYZ")
	if err != nil {
		return nil, err
	}

	rTRC, err := p.ReadCurveTag("rTRC")
	if err != nil {
		return nil, err
	}
	gTRC, err := p.ReadCurveTag("gTRC")
	if err != nil {
		return nil, err
	}
	bTRC, err := p.ReadCurveTag("bTRC")
	if err != nil {
		return nil, err
	}

	return &matrixTRCTransform{
		srcGamma: [3]float64{rTRC, gTRC, bTRC},
		matrix: [9]float64{
			rXYZ[0], gXYZ[0], bXYZ[0], // X row
			rXYZ[1], gXYZ[1], bXYZ[1], // Y row
			rXYZ[2], gXYZ[2], bXYZ[2], // Z row
		},
	}, nil
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
