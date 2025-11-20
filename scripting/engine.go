package scripting

import (
	"context"
)

// Engine represents a scripting engine (e.g., JavaScript).
type Engine interface {
	// Execute executes a script in the context of the document.
	Execute(ctx context.Context, script string) (interface{}, error)

	// RegisterDOM registers the PDF Document Object Model with the engine.
	RegisterDOM(dom PDFDOM) error
}

// PDFDOM exposes the PDF document structure to the scripting engine.
// It provides a safe, controlled API for scripts to interact with the PDF.
type PDFDOM interface {
	// GetField returns a form field by name.
	GetField(name string) (FormFieldProxy, error)

	// GetPage returns a page by index (0-based).
	GetPage(index int) (PageProxy, error)

	// Alert shows an alert dialog (if supported by the viewer/runner).
	Alert(message string)
}

// FormFieldProxy represents a form field exposed to scripts.
type FormFieldProxy interface {
	GetValue() interface{}
	SetValue(value interface{})
}

// PageProxy represents a page exposed to scripts.
type PageProxy interface {
	GetIndex() int
}
