package ocr

import "testing"

func TestTesseractOptions(t *testing.T) {
	in := Input{}
	WithTesseractPSM(6)(&in)
	if got := in.Metadata["tessedit_pageseg_mode"]; got != "6" {
		t.Fatalf("expected PSM to be set, got %q", got)
	}
	WithTesseractWhitelist("ABC")(&in)
	if got := in.Metadata["tessedit_char_whitelist"]; got != "ABC" {
		t.Fatalf("expected whitelist to be set, got %q", got)
	}
}
