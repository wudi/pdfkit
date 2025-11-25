package ocr

import "strconv"

// WithTesseractPSM sets the page segmentation mode (PSM) variable for Tesseract.
// See https://tesseract-ocr.github.io/tessdoc/ImproveQuality.html#page-segmentation-method for values.
func WithTesseractPSM(mode int) InputOption {
	return func(in *Input) {
		if in.Metadata == nil {
			in.Metadata = make(map[string]string)
		}
		in.Metadata["tessedit_pageseg_mode"] = strconv.Itoa(mode)
	}
}

// WithTesseractWhitelist restricts recognition to the provided characters.
func WithTesseractWhitelist(chars string) InputOption {
	return func(in *Input) {
		if in.Metadata == nil {
			in.Metadata = make(map[string]string)
		}
		in.Metadata["tessedit_char_whitelist"] = chars
	}
}
