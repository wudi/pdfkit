package filters

import (
	"bytes"
	"testing"
)

func TestScaleJPXSample(t *testing.T) {
	t.Run("unsigned", func(t *testing.T) {
		if got := scaleJPXSample(64, 7, false); got == 0 {
			t.Fatalf("expected non-zero scale")
		}
	})
	t.Run("signed", func(t *testing.T) {
		if got := scaleJPXSample(-16, 5, true); got >= 128 {
			t.Fatalf("expected signed sample to map below midpoint, got %d", got)
		}
	})
}

func TestComposeJPXPixelBuffer(t *testing.T) {
	gray := jpxComponent{samples: []int32{0, 255}, precision: 8}
	alpha := jpxComponent{samples: []int32{128, 255}, precision: 8}
	buf, err := composeJPXPixelBuffer([]jpxComponent{gray, alpha}, 1, 2, jpxColorSpaceGray)
	if err != nil {
		t.Fatalf("compose gray alpha: %v", err)
	}
	expected := []byte{0, 0, 0, 128, 255, 255, 255, 255}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("unexpected gray+alpha output %v", buf)
	}

	r := jpxComponent{samples: []int32{255}, precision: 8}
	g := jpxComponent{samples: []int32{0}, precision: 8}
	b := jpxComponent{samples: []int32{0}, precision: 8}
	k := jpxComponent{samples: []int32{0}, precision: 8}
	buf, err = composeJPXPixelBuffer([]jpxComponent{r, g, b, k}, 1, 1, jpxColorSpaceCMYK)
	if err != nil {
		t.Fatalf("compose cmyk: %v", err)
	}
	expected = []byte{0, 255, 255, 255}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("unexpected cmyk output %v", buf)
	}

	sy := jpxComponent{samples: []int32{235}, precision: 8}
	scb := jpxComponent{samples: []int32{128}, precision: 8}
	scr := jpxComponent{samples: []int32{128}, precision: 8}
	buf, err = composeJPXPixelBuffer([]jpxComponent{sy, scb, scr}, 1, 1, jpxColorSpaceSYCC)
	if err != nil {
		t.Fatalf("compose sycc: %v", err)
	}
	if buf[0] <= 200 || buf[1] <= 200 || buf[2] <= 200 {
		t.Fatalf("expected near white from SYCC midpoints, got %v", buf)
	}

	_, err = composeJPXPixelBuffer([]jpxComponent{jpxComponent{samples: []int32{0}, precision: 8}, jpxComponent{samples: []int32{}, precision: 8}}, 1, 1, jpxColorSpaceRGB)
	if err == nil {
		t.Fatalf("expected mismatch error when lengths differ")
	}
}
