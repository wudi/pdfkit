package filters

import (
	"errors"
	"fmt"
	"math"
)

type jpxColorSpace int

const (
	jpxColorSpaceUnknown jpxColorSpace = iota
	jpxColorSpaceRGB
	jpxColorSpaceGray
	jpxColorSpaceSYCC
	jpxColorSpaceCMYK
)

type jpxComponent struct {
	samples   []int32
	precision int
	signed    bool
}

func composeJPXPixelBuffer(comps []jpxComponent, width, height int, space jpxColorSpace) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid JPX bounds")
	}
	pixels := width * height
	for idx, comp := range comps {
		if len(comp.samples) != pixels {
			return nil, fmt.Errorf("component %d length mismatch", idx)
		}
	}
	out := make([]byte, pixels*4)
	switch len(comps) {
	case 1:
		fillJPXGray(out, comps[0])
	case 2:
		fillJPXGrayAlpha(out, comps[0], comps[1])
	case 3:
		if space == jpxColorSpaceSYCC {
			fillJPXSYCC(out, comps[0], comps[1], comps[2])
		} else {
			fillJPXRGB(out, comps[0], comps[1], comps[2])
		}
	case 4:
		if space == jpxColorSpaceCMYK {
			fillJPXCMYK(out, comps[0], comps[1], comps[2], comps[3])
		} else {
			fillJPXRGBA(out, comps[0], comps[1], comps[2], comps[3])
		}
	default:
		return nil, fmt.Errorf("unsupported JPX component count %d", len(comps))
	}
	return out, nil
}

func fillJPXGray(dst []byte, gray jpxComponent) {
	for i := range gray.samples {
		g := scaleJPXSample(gray.samples[i], gray.precision, gray.signed)
		off := i * 4
		dst[off] = g
		dst[off+1] = g
		dst[off+2] = g
		dst[off+3] = 255
	}
}

func fillJPXGrayAlpha(dst []byte, gray, alpha jpxComponent) {
	for i := range gray.samples {
		g := scaleJPXSample(gray.samples[i], gray.precision, gray.signed)
		a := scaleJPXSample(alpha.samples[i], alpha.precision, alpha.signed)
		off := i * 4
		dst[off] = g
		dst[off+1] = g
		dst[off+2] = g
		dst[off+3] = a
	}
}

func fillJPXRGB(dst []byte, rComp, gComp, bComp jpxComponent) {
	for i := range rComp.samples {
		r := scaleJPXSample(rComp.samples[i], rComp.precision, rComp.signed)
		g := scaleJPXSample(gComp.samples[i], gComp.precision, gComp.signed)
		b := scaleJPXSample(bComp.samples[i], bComp.precision, bComp.signed)
		off := i * 4
		dst[off] = r
		dst[off+1] = g
		dst[off+2] = b
		dst[off+3] = 255
	}
}

func fillJPXRGBA(dst []byte, rComp, gComp, bComp, aComp jpxComponent) {
	for i := range rComp.samples {
		r := scaleJPXSample(rComp.samples[i], rComp.precision, rComp.signed)
		g := scaleJPXSample(gComp.samples[i], gComp.precision, gComp.signed)
		b := scaleJPXSample(bComp.samples[i], bComp.precision, bComp.signed)
		a := scaleJPXSample(aComp.samples[i], aComp.precision, aComp.signed)
		off := i * 4
		dst[off] = r
		dst[off+1] = g
		dst[off+2] = b
		dst[off+3] = a
	}
}

func fillJPXSYCC(dst []byte, yComp, cbComp, crComp jpxComponent) {
	for i := range yComp.samples {
		y := scaleJPXSample(yComp.samples[i], yComp.precision, yComp.signed)
		cb := scaleJPXSample(cbComp.samples[i], cbComp.precision, cbComp.signed)
		cr := scaleJPXSample(crComp.samples[i], crComp.precision, crComp.signed)
		r, g, b := convertYCbCrToRGB(y, cb, cr)
		off := i * 4
		dst[off] = r
		dst[off+1] = g
		dst[off+2] = b
		dst[off+3] = 255
	}
}

func fillJPXCMYK(dst []byte, cComp, mComp, yComp, kComp jpxComponent) {
	for i := range cComp.samples {
		c := scaleJPXSample(cComp.samples[i], cComp.precision, cComp.signed)
		m := scaleJPXSample(mComp.samples[i], mComp.precision, mComp.signed)
		y := scaleJPXSample(yComp.samples[i], yComp.precision, yComp.signed)
		k := scaleJPXSample(kComp.samples[i], kComp.precision, kComp.signed)
		r, g, b := convertCMYKToRGB(c, m, y, k)
		off := i * 4
		dst[off] = r
		dst[off+1] = g
		dst[off+2] = b
		dst[off+3] = 255
	}
}

func scaleJPXSample(value int32, precision int, signed bool) uint8 {
	if precision <= 0 {
		return 0
	}
	if signed {
		shift := int64(1) << uint(minInt(precision-1, 60))
		max := shift*2 - 1
		v := int64(value) + shift
		if v < 0 {
			v = 0
		}
		if v > max {
			v = max
		}
		return uint8((v*255 + max/2) / max)
	}
	limit := int64(1)<<uint(minInt(precision, 60)) - 1
	v := int64(value)
	if v < 0 {
		v = 0
	}
	if v > limit {
		v = limit
	}
	return uint8((v*255 + limit/2) / limit)
}

func convertYCbCrToRGB(y, cb, cr uint8) (uint8, uint8, uint8) {
	Y := float64(y)
	Cb := float64(int(cb) - 128)
	Cr := float64(int(cr) - 128)
	r := clampByte(Y + 1.402*Cr)
	g := clampByte(Y - 0.344136*Cb - 0.714136*Cr)
	b := clampByte(Y + 1.772*Cb)
	return r, g, b
}

func convertCMYKToRGB(c, m, y, k uint8) (uint8, uint8, uint8) {
	cf := float64(c) / 255.0
	mf := float64(m) / 255.0
	yf := float64(y) / 255.0
	kf := float64(k) / 255.0
	r := clampByte((1 - math.Min(1, cf+kf)) * 255)
	g := clampByte((1 - math.Min(1, mf+kf)) * 255)
	b := clampByte((1 - math.Min(1, yf+kf)) * 255)
	return r, g, b
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
