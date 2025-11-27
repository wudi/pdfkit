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

	// 2. Try ICC-based conversion if available
	if srcICC, ok := t.src.(*ICCProfile); ok {
		if dstICC, ok := t.dst.(*ICCProfile); ok {
			// Try generic ICC conversion (LUT or Matrix)
			out, err := convertICC(srcICC, dstICC, in)
			if err == nil {
				return out, nil
			}
			// Fallback if ICC fails (e.g. complex LUTs)
		}
	}

	// 3. Simple conversions (fallback)
	// If we had a real CMM, we would use the LUTs here.
	// For now, we implement basic conversions between standard spaces if possible.

	dstCh := numChannels(t.dst.ColorSpace())
	out := make([]float64, dstCh)

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

	if t.src.ColorSpace() == "GRAY" && t.dst.ColorSpace() == "RGB " {
		g := in[0]
		out[0], out[1], out[2] = g, g, g
		return out, nil
	}

	if t.src.ColorSpace() == "RGB " && t.dst.ColorSpace() == "GRAY" {
		r, g, b := in[0], in[1], in[2]
		out[0] = 0.299*r + 0.587*g + 0.114*b
		return out, nil
	}

	if t.src.ColorSpace() == "GRAY" && t.dst.ColorSpace() == "CMYK" {
		k := 1.0 - in[0]
		out[0], out[1], out[2], out[3] = 0, 0, 0, k
		return out, nil
	}

	if t.src.ColorSpace() == "CMYK" && t.dst.ColorSpace() == "GRAY" {
		// CMYK -> RGB -> Gray
		c, m, y, k := in[0], in[1], in[2], in[3]
		r := (1.0 - c) * (1.0 - k)
		g := (1.0 - m) * (1.0 - k)
		b := (1.0 - y) * (1.0 - k)
		out[0] = 0.299*r + 0.587*g + 0.114*b
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

func convertICC(src, dst *ICCProfile, in []float64) ([]float64, error) {
	// 1. Src -> PCS
	toPCS, err := createToPCS(src)
	if err != nil {
		return nil, err
	}
	pcsSrc, err := toPCS.Convert(in)
	if err != nil {
		return nil, err
	}

	// Check PCS mismatch
	srcPCS := src.PCS()
	dstPCS := dst.PCS()

	var pcsDst []float64
	if srcPCS == dstPCS {
		pcsDst = pcsSrc
	} else {
		// Convert PCS -> PCS (XYZ <-> Lab)
		if srcPCS == "XYZ " && dstPCS == "Lab " {
			pcsDst = XYZToLab(pcsSrc)
		} else if srcPCS == "Lab " && dstPCS == "XYZ " {
			pcsDst = LabToXYZ(pcsSrc)
		} else {
			// Fallback or error?
			pcsDst = pcsSrc
		}
	}

	// 2. PCS -> Dst
	fromPCS, err := createFromPCS(dst)
	if err != nil {
		return nil, err
	}
	return fromPCS.Convert(pcsDst)
}

func createToPCS(p *ICCProfile) (Transform, error) {
	// Try A2B0 (Device to PCS)
	if lut, err := p.ReadLUTTag("A2B0"); err == nil {
		return lut, nil
	}
	// Try Matrix/TRC
	return tryCreateMatrixTRC(p)
}

func createFromPCS(p *ICCProfile) (Transform, error) {
	// Try B2A0 (PCS to Device)
	if lut, err := p.ReadLUTTag("B2A0"); err == nil {
		return lut, nil
	}
	// Try Matrix/TRC (Inverse)
	mat, err := tryCreateMatrixTRC(p)
	if err != nil {
		return nil, err
	}
	return mat.Inverse()
}

// D50 White Point for XYZ <-> Lab conversion
const (
	D50X = 0.9642
	D50Y = 1.0000
	D50Z = 0.8249
)

func XYZToLab(xyz []float64) []float64 {
	if len(xyz) < 3 {
		return xyz
	}
	x := xyz[0] / D50X
	y := xyz[1] / D50Y
	z := xyz[2] / D50Z

	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Pow(t, 1.0/3.0)
		}
		return 7.787*t + 16.0/116.0
	}

	fx := f(x)
	fy := f(y)
	fz := f(z)

	L := 116.0*fy - 16.0
	a := 500.0 * (fx - fy)
	b := 200.0 * (fy - fz)

	return []float64{L, a, b}
}

func LabToXYZ(lab []float64) []float64 {
	if len(lab) < 3 {
		return lab
	}
	L := lab[0]
	a := lab[1]
	b := lab[2]

	fy := (L + 16.0) / 116.0
	fx := a/500.0 + fy
	fz := fy - b/200.0

	fInv := func(t float64) float64 {
		if t > 0.206893 { // 6/29
			return t * t * t
		}
		return (t - 16.0/116.0) / 7.787
	}

	x := D50X * fInv(fx)
	y := D50Y * fInv(fy)
	z := D50Z * fInv(fz)

	return []float64{x, y, z}
}

func (t *matrixTRCTransform) Inverse() (Transform, error) {
	invMat, err := invertMatrix(t.matrix)
	if err != nil {
		return nil, err
	}
	return &inverseMatrixTRCTransform{
		destGamma: t.srcGamma,
		matrix:    invMat,
	}, nil
}

type inverseMatrixTRCTransform struct {
	destGamma [3]float64
	matrix    [9]float64 // XYZ -> Linear RGB matrix
}

func (t *inverseMatrixTRCTransform) Convert(in []float64) ([]float64, error) {
	if len(in) < 3 {
		return nil, errors.New("input too short")
	}
	x, y, z := in[0], in[1], in[2]

	// 1. Matrix Multiply (XYZ -> Linear RGB)
	rLin := t.matrix[0]*x + t.matrix[1]*y + t.matrix[2]*z
	gLin := t.matrix[3]*x + t.matrix[4]*y + t.matrix[5]*z
	bLin := t.matrix[6]*x + t.matrix[7]*y + t.matrix[8]*z

	// 2. Apply Gamma (Linear RGB -> RGB)
	// val = pow(lin, 1/gamma)
	r := math.Pow(math.Max(0, rLin), 1.0/t.destGamma[0])
	g := math.Pow(math.Max(0, gLin), 1.0/t.destGamma[1])
	b := math.Pow(math.Max(0, bLin), 1.0/t.destGamma[2])

	return []float64{r, g, b}, nil
}

func invertMatrix(m [9]float64) ([9]float64, error) {
	// m = [a, b, c, d, e, f, g, h, i]
	//      0  1  2  3  4  5  6  7  8
	a, b, c := m[0], m[1], m[2]
	d, e, f := m[3], m[4], m[5]
	g, h, i := m[6], m[7], m[8]

	det := a*(e*i-f*h) - b*(d*i-f*g) + c*(d*h-e*g)
	if math.Abs(det) < 1e-10 {
		return [9]float64{}, errors.New("matrix is singular")
	}
	invDet := 1.0 / det

	return [9]float64{
		(e*i - f*h) * invDet, (c*h - b*i) * invDet, (b*f - c*e) * invDet,
		(f*g - d*i) * invDet, (a*i - c*g) * invDet, (c*d - a*f) * invDet,
		(d*h - e*g) * invDet, (g*b - a*h) * invDet, (a*e - b*d) * invDet,
	}, nil
}
