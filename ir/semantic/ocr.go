package semantic

// OCRResult carries recognized text for a single image/page fragment.
type OCRResult struct {
	InputID   string
	PlainText string
}
