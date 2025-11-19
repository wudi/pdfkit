package filters

import "testing"

func TestValidateNativeImageBounds(t *testing.T) {
	if err := validateNativeImageBounds(1024, 512); err != nil {
		t.Fatalf("expected valid bounds, got %v", err)
	}
	if err := validateNativeImageBounds(0, 10); err == nil {
		t.Fatalf("expected error for zero width")
	}
	if err := validateNativeImageBounds(maxNativeImageDimension+1, 4); err == nil {
		t.Fatalf("expected dimension limit error")
	}
	width := 20000
	height := int(maxNativeImagePixels/int64(width)) + 1
	if height > maxNativeImageDimension {
		t.Fatalf("test precondition height %d > dimension limit", height)
	}
	if err := validateNativeImageBounds(width, height); err == nil {
		t.Fatalf("expected pixel limit error")
	}
}
