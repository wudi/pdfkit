package filters

import (
	"bytes"
	"compress/flate"
	"context"
	"testing"
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
