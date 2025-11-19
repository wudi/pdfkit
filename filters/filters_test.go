package filters

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"pdflib/ir/raw"
)

func TestFlateDecode(t *testing.T) {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestSpeed)
	w.Write([]byte("hello world"))
	w.Close()

	dec := NewFlateDecoder()
	out, err := dec.Decode(context.Background(), buf.Bytes(), nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(out) != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFlateDecodeWithPredictor(t *testing.T) {
	var comp bytes.Buffer
	w, _ := flate.NewWriter(&comp, flate.BestSpeed)
	// PNG predictor row: filter byte 1 (Sub), then row bytes.
	w.Write([]byte{1, 10, 12, 20})
	w.Close()

	params := raw.Dict()
	params.Set(raw.NameObj{Val: "Predictor"}, raw.NumberInt(12))
	params.Set(raw.NameObj{Val: "Colors"}, raw.NumberInt(1))
	params.Set(raw.NameObj{Val: "BitsPerComponent"}, raw.NumberInt(8))
	params.Set(raw.NameObj{Val: "Columns"}, raw.NumberInt(3))

	dec := NewFlateDecoder()
	out, err := dec.Decode(context.Background(), comp.Bytes(), params)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	want := []byte{10, 22, 42}
	if !bytes.Equal(out, want) {
		t.Fatalf("predictor output mismatch: got %v want %v", out, want)
	}
}

func TestLZWDecode(t *testing.T) {
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, lzw.MSB, 8)
	input := []byte("hello hello hello")
	if _, err := w.Write(input); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	dec := NewLZWDecoder()
	out, err := dec.Decode(context.Background(), buf.Bytes(), nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !bytes.Equal(out, input) {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestLZWDecodeWithPredictor(t *testing.T) {
	// Single PNG row with filter None: [0,1,2,3]
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, lzw.MSB, 8)
	w.Write([]byte{0, 1, 2, 3})
	w.Close()

	params := raw.Dict()
	params.Set(raw.NameObj{Val: "Predictor"}, raw.NumberInt(12))
	params.Set(raw.NameObj{Val: "Colors"}, raw.NumberInt(1))
	params.Set(raw.NameObj{Val: "BitsPerComponent"}, raw.NumberInt(8))
	params.Set(raw.NameObj{Val: "Columns"}, raw.NumberInt(3))

	dec := NewLZWDecoder()
	out, err := dec.Decode(context.Background(), buf.Bytes(), params)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !bytes.Equal(out, []byte{1, 2, 3}) {
		t.Fatalf("unexpected output: %v", out)
	}
}

func TestRunLengthDecode(t *testing.T) {
	// literal run of 3 bytes (len=2), then repeat 'A' 2 times (len=255 => count=2), then EOD 128
	data := []byte{2, 'h', 'i', '!', 255, 'A', 128}
	dec := NewRunLengthDecoder()
	out, err := dec.Decode(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(out) != "hi!AA" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestASCII85Decode(t *testing.T) {
	dec := NewASCII85Decoder()
	out, err := dec.Decode(context.Background(), []byte("<~87cURD_*#4DfTZ)+T~>"), nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(out) != "Hello, World!" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestASCIIHexDecode(t *testing.T) {
	dec := NewASCIIHexDecoder()
	out, err := dec.Decode(context.Background(), []byte("68656c6c6f20776f726c64>"), nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(out) != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestDCTDecode(t *testing.T) {
	jpegData := sampleJPEG(t)
	dec := NewDCTDecoder()
	out, err := dec.Decode(context.Background(), jpegData, nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(out) != 2*1*4 {
		t.Fatalf("unexpected pixel size: %d", len(out))
	}
	// Ensure the decoded pixels are non-zero and differ, indicating decode happened.
	if out[0] == 0 && out[1] == 0 && out[2] == 0 {
		t.Fatalf("first pixel unexpectedly zero: %v", out[:4])
	}
	if bytes.Equal(out[:4], out[4:8]) {
		t.Fatalf("expected differing pixels, got %v and %v", out[:4], out[4:8])
	}
}

func TestPipelineDecodeDCT(t *testing.T) {
	jpegData := sampleJPEG(t)
	p := NewPipeline([]Decoder{NewDCTDecoder()}, Limits{})
	out, err := p.Decode(context.Background(), jpegData, []string{"DCTDecode"}, nil)
	if err != nil {
		t.Fatalf("pipeline decode error: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("pipeline decode produced empty data")
	}
}

func TestUnsupportedFilters(t *testing.T) {
	fp := NewPipeline([]Decoder{NewJPXDecoder()}, Limits{})
	_, err := fp.Decode(context.Background(), []byte{0x00}, []string{"JPXDecode"}, nil)
	var ue UnsupportedError
	if err == nil || !errors.As(err, &ue) || ue.Filter != "JPXDecode" {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}

func TestJPXMislabeledPNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	img.Set(1, 1, color.NRGBA{R: 200, G: 150, B: 100, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	dec := NewJPXDecoder()
	out, err := dec.Decode(context.Background(), buf.Bytes(), nil)
	if err != nil {
		t.Fatalf("decode mislabeled png: %v", err)
	}
	if len(out) != 2*2*4 {
		t.Fatalf("unexpected size from jpx decoder: %d", len(out))
	}
}

func toGray(img image.Image) *image.Gray {
	if g, ok := img.(*image.Gray); ok {
		return g
	}
	b := img.Bounds()
	g := image.NewGray(b)
	draw.Draw(g, b, img, b.Min, draw.Src)
	return g
}

func sampleJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}
