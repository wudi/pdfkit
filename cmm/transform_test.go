package cmm

import (
	"encoding/binary"
	"math"
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

func TestMatrixTRCTransform(t *testing.T) {
	// Construct a profile with rXYZ, gXYZ, bXYZ, rTRC, gTRC, bTRC
	// For simplicity, Identity Matrix and Gamma 1.0
	// rXYZ = [1 0 0]
	// gXYZ = [0 1 0]
	// bXYZ = [0 0 1]
	// TRC = Gamma 1.0 (count=0 or count=1 val=256)

	data := make([]byte, 1024) // ample space
	binary.BigEndian.PutUint32(data[0:4], 1024)
	copy(data[36:40], "acsp")
	copy(data[16:20], "RGB ")
	copy(data[20:24], "XYZ ") // PCS

	// Tags
	tags := []struct {
		sig  string
		data []byte
	}{
		{"rXYZ", makeXYZ(1, 0, 0)},
		{"gXYZ", makeXYZ(0, 1, 0)},
		{"bXYZ", makeXYZ(0, 0, 1)},
		{"rTRC", makeGamma(1.0)},
		{"gTRC", makeGamma(1.0)},
		{"bTRC", makeGamma(1.0)},
	}

	tagCount := uint32(len(tags))
	binary.BigEndian.PutUint32(data[128:132], tagCount)

	offset := uint32(132 + 12*len(tags))
	tagTableOffset := 132

	for _, tag := range tags {
		// Write Tag Entry
		binary.BigEndian.PutUint32(data[tagTableOffset:tagTableOffset+4], strToUint32(tag.sig))
		binary.BigEndian.PutUint32(data[tagTableOffset+4:tagTableOffset+8], offset)
		binary.BigEndian.PutUint32(data[tagTableOffset+8:tagTableOffset+12], uint32(len(tag.data)))
		tagTableOffset += 12

		// Write Tag Data
		copy(data[offset:], tag.data)
		offset += uint32(len(tag.data))
	}

	f := NewFactory()
	src, err := f.NewProfile(data)
	if err != nil {
		t.Fatalf("NewProfile failed: %v", err)
	}

	// Destination: XYZ
	dstData := makeProfileData("XYZ ")
	dst, _ := f.NewProfile(dstData)

	tr, err := f.NewTransform(src, dst, IntentPerceptual)
	if err != nil {
		t.Fatalf("NewTransform failed: %v", err)
	}

	// With Identity Matrix and Gamma 1.0, RGB -> XYZ should be Identity
	in := []float64{0.1, 0.5, 0.9}
	out, err := tr.Convert(in)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	for i := range in {
		if math.Abs(out[i]-in[i]) > 0.001 {
			t.Errorf("expected %f, got %f", in[i], out[i])
		}
	}
}

func makeXYZ(x, y, z float64) []byte {
	b := make([]byte, 20)
	copy(b[0:4], "XYZ ")
	binary.BigEndian.PutUint32(b[8:12], floatToS15Fixed16(x))
	binary.BigEndian.PutUint32(b[12:16], floatToS15Fixed16(y))
	binary.BigEndian.PutUint32(b[16:20], floatToS15Fixed16(z))
	return b
}

func makeGamma(g float64) []byte {
	b := make([]byte, 14)
	copy(b[0:4], "curv")
	binary.BigEndian.PutUint32(b[8:12], 1) // Count 1
	val := uint16(g * 256.0)
	binary.BigEndian.PutUint16(b[12:14], val)
	return b
}

func floatToS15Fixed16(f float64) uint32 {
	return uint32(int32(f * 65536.0))
}

func strToUint32(s string) uint32 {
	return binary.BigEndian.Uint32([]byte(s))
}

func TestICC_RGB_to_RGB(t *testing.T) {
	// Src: Gamma 1.0, Identity Matrix
	srcData := makeRGBProfile(1.0, 1, 0, 0, 0, 1, 0, 0, 0, 1)

	// Dst: Gamma 2.0, Identity Matrix
	dstData := makeRGBProfile(2.0, 1, 0, 0, 0, 1, 0, 0, 0, 1)

	f := NewFactory()
	src, err := f.NewProfile(srcData)
	if err != nil {
		t.Fatalf("NewProfile src failed: %v", err)
	}
	dst, err := f.NewProfile(dstData)
	if err != nil {
		t.Fatalf("NewProfile dst failed: %v", err)
	}

	tr, err := f.NewTransform(src, dst, IntentPerceptual)
	if err != nil {
		t.Fatalf("NewTransform failed: %v", err)
	}

	// Input 0.25
	// Src (Gamma 1.0) -> Linear 0.25
	// Dst (Gamma 2.0) -> Encoded = Linear^(1/2.0) = sqrt(0.25) = 0.5
	in := []float64{0.25, 0.25, 0.25}
	out, err := tr.Convert(in)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	expected := 0.5
	for i, v := range out {
		if math.Abs(v-expected) > 0.01 {
			t.Errorf("channel %d: expected %f, got %f", i, expected, v)
		}
	}
}

func makeRGBProfile(gamma float64, rX, rY, rZ, gX, gY, gZ, bX, bY, bZ float64) []byte {
	data := make([]byte, 2048)
	binary.BigEndian.PutUint32(data[0:4], 2048)
	copy(data[36:40], "acsp")
	copy(data[16:20], "RGB ")
	copy(data[20:24], "XYZ ")

	tags := []struct {
		sig  string
		data []byte
	}{
		{"rXYZ", makeXYZ(rX, rY, rZ)},
		{"gXYZ", makeXYZ(gX, gY, gZ)},
		{"bXYZ", makeXYZ(bX, bY, bZ)},
		{"rTRC", makeGamma(gamma)},
		{"gTRC", makeGamma(gamma)},
		{"bTRC", makeGamma(gamma)},
	}

	tagCount := uint32(len(tags))
	binary.BigEndian.PutUint32(data[128:132], tagCount)

	offset := uint32(132 + 12*len(tags))
	tagTableOffset := 132

	for _, tag := range tags {
		binary.BigEndian.PutUint32(data[tagTableOffset:tagTableOffset+4], strToUint32(tag.sig))
		binary.BigEndian.PutUint32(data[tagTableOffset+4:tagTableOffset+8], offset)
		binary.BigEndian.PutUint32(data[tagTableOffset+8:tagTableOffset+12], uint32(len(tag.data)))
		tagTableOffset += 12
		copy(data[offset:], tag.data)
		offset += uint32(len(tag.data))
	}
	return data
}
