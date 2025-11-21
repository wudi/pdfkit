package streaming

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
)

func ParseColorSpace(doc *raw.Document, obj raw.Object) semantic.ColorSpace {
	return parseColorSpace(doc, obj)
}
