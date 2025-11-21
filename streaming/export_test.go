package streaming

import (
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func ParseColorSpace(doc *raw.Document, obj raw.Object) semantic.ColorSpace {
	return parseColorSpace(doc, obj)
}
