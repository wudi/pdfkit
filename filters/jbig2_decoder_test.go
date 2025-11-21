package filters

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
)

func TestJBIG2Decode_UsesResolver(t *testing.T) {
	orig := jbig2NativeDecode
	t.Cleanup(func() { jbig2NativeDecode = orig })

	var calledPage, calledGlobals []byte
	jbig2NativeDecode = func(ctx context.Context, page, globals []byte) ([]byte, error) {
		calledPage = append([]byte(nil), page...)
		calledGlobals = append([]byte(nil), globals...)
		return []byte{1, 2, 3, 4}, nil
	}

	params := raw.Dict()
	params.Set(raw.NameLiteral("JBIG2Globals"), raw.RefObj{R: raw.ObjectRef{Num: 5}})

	resolver := func(ctx context.Context, ref raw.ObjectRef) ([]byte, error) {
		if ref.Num != 5 {
			return nil, fmt.Errorf("unexpected ref %v", ref)
		}
		return []byte("globals"), nil
	}

	p := NewPipeline([]Decoder{NewJBIG2Decoder()}, Limits{})
	out, err := p.DecodeWithResolver(context.Background(), []byte("page-data"), []string{"JBIG2Decode"}, []raw.Dictionary{params}, resolver)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !bytes.Equal(out, []byte{1, 2, 3, 4}) {
		t.Fatalf("unexpected output %v", out)
	}
	if !bytes.Equal(calledPage, []byte("page-data")) {
		t.Fatalf("page data mismatch %q", calledPage)
	}
	if !bytes.Equal(calledGlobals, []byte("globals")) {
		t.Fatalf("globals mismatch %q", calledGlobals)
	}
}

func TestJBIG2Decode_MissingResolver(t *testing.T) {
	params := raw.Dict()
	params.Set(raw.NameLiteral("JBIG2Globals"), raw.RefObj{R: raw.ObjectRef{Num: 7}})

	p := NewPipeline([]Decoder{NewJBIG2Decoder()}, Limits{})
	if _, err := p.Decode(context.Background(), []byte{0x01}, []string{"JBIG2Decode"}, []raw.Dictionary{params}); err == nil {
		t.Fatalf("expected resolver error")
	}
}

func TestJBIG2MonochromeToNRGBA(t *testing.T) {
	// 0x81 => 10000001, 0x55 => 01010101
	data := []byte{0x81, 0x55}
	pix, err := jbig2MonochromeToNRGBA(8, 2, 1, data)
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	expected := []byte{
		0, 0, 0, 255, // black
		255, 255, 255, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
		0, 0, 0, 255, // black
		255, 255, 255, 255,
		0, 0, 0, 255,
		255, 255, 255, 255,
		0, 0, 0, 255,
		255, 255, 255, 255,
		0, 0, 0, 255,
		255, 255, 255, 255,
		0, 0, 0, 255,
	}
	if !bytes.Equal(pix, expected) {
		t.Fatalf("unexpected pixel data %v", pix)
	}
}
