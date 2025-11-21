package cmm

import (
	"bytes"
	"errors"
)

type factoryImpl struct{}

// NewFactory returns a default CMM factory.
func NewFactory() Factory {
	return &factoryImpl{}
}

func (f *factoryImpl) NewProfile(data []byte) (Profile, error) {
	return NewICCProfile(data)
}

func (f *factoryImpl) NewTransform(src, dst Profile, intent RenderingIntent) (Transform, error) {
	if src == nil || dst == nil {
		return nil, errors.New("source and destination profiles required")
	}

	// Optimization: Identity transform if profiles are the same
	if bytes.Equal(src.Data(), dst.Data()) {
		return &identityTransform{}, nil
	}

	// Try Matrix/TRC for RGB -> XYZ
	if src.ColorSpace() == "RGB " && dst.ColorSpace() == "XYZ " {
		if icc, ok := src.(*ICCProfile); ok {
			if trc, err := tryCreateMatrixTRC(icc); err == nil {
				return trc, nil
			}
		}
	}

	// In a full implementation, we would build a pipeline here.
	// For now, we return a basic transform that might fail or do simple conversion.
	return &basicTransform{src: src, dst: dst, intent: intent}, nil
}

type identityTransform struct{}

func (t *identityTransform) Convert(src []float64) ([]float64, error) {
	// Copy to avoid side effects if caller reuses slice
	dst := make([]float64, len(src))
	copy(dst, src)
	return dst, nil
}
