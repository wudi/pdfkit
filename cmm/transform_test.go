package cmm

import (
	"testing"
)

func TestFactoryIdentity(t *testing.T) {
	f := NewFactory()
	// Create a dummy profile
	data := makeProfileData("RGB ")
	p1, err := f.NewProfile(data)
	if err != nil {
		t.Fatalf("NewProfile failed: %v", err)
	}
	p2, err := f.NewProfile(data)
	if err != nil {
		t.Fatalf("NewProfile failed: %v", err)
	}

	tr, err := f.NewTransform(p1, p2, IntentPerceptual)
	if err != nil {
		t.Fatalf("NewTransform failed: %v", err)
	}

	in := []float64{0.1, 0.2, 0.3}
	out, err := tr.Convert(in)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	for i := range in {
		if out[i] != in[i] {
			t.Errorf("expected %f, got %f", in[i], out[i])
		}
	}
}

func TestBasicTransformRGBtoCMYK(t *testing.T) {
	// We need profiles that report "RGB " and "CMYK"
	// Since we mock the profile parsing in tests or use real data,
	// let's construct minimal data that parses to these spaces.

	rgbData := makeProfileData("RGB ")
	cmykData := makeProfileData("CMYK")

	f := NewFactory()
	src, _ := f.NewProfile(rgbData)
	dst, _ := f.NewProfile(cmykData)

	tr, err := f.NewTransform(src, dst, IntentPerceptual)
	if err != nil {
		t.Fatalf("NewTransform failed: %v", err)
	}

	// Black
	in := []float64{0, 0, 0}
	out, err := tr.Convert(in)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	// Expect K=1
	if out[3] != 1.0 {
		t.Errorf("expected K=1.0 for black, got %f", out[3])
	}

	// White
	in = []float64{1, 1, 1}
	out, _ = tr.Convert(in)
	if out[3] != 0.0 {
		t.Errorf("expected K=0.0 for white, got %f", out[3])
	}
}

func makeProfileData(cs string) []byte {
	data := make([]byte, 132)
	// Size
	data[3] = 132
	// Signature
	copy(data[36:40], "acsp")
	// ColorSpace
	copy(data[16:20], cs)
	return data
}
