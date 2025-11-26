package extensions

import (
	"context"

	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/scripting"
)

// JavaScriptRunner is a Transformer extension that executes JavaScript actions.
// This is primarily used for form calculations and document initialization scripts.
type JavaScriptRunner struct {
	engine scripting.Engine
}

func NewJavaScriptRunner(engine scripting.Engine) *JavaScriptRunner {
	return &JavaScriptRunner{engine: engine}
}

func (r *JavaScriptRunner) Name() string {
	return "JavaScriptRunner"
}

func (r *JavaScriptRunner) Phase() Phase {
	return PhaseTransform
}

func (r *JavaScriptRunner) Priority() int {
	return 100 // Run after basic structure is ready
}

func (r *JavaScriptRunner) Execute(ctx context.Context, doc *semantic.Document) error {
	return r.Transform(ctx, doc)
}

func (r *JavaScriptRunner) Transform(ctx context.Context, doc *semantic.Document) error {
	if r.engine == nil {
		return nil
	}

	// 1. Register DOM
	// In a real implementation, we would wrap doc in a PDFDOM implementation
	// r.engine.RegisterDOM(NewPDFDOM(doc))

	// 2. Execute Document-level scripts (Names -> JavaScript)
	if doc.Names != nil && len(doc.Names.JavaScript) > 0 {
		for name, action := range doc.Names.JavaScript {
			if _, err := r.engine.Execute(ctx, action.JS); err != nil {
				// Log error but continue? Or fail?
				// For now, we continue to execute other scripts.
				_ = name // ignore unused
			}
		}
	}

	// 3. Execute OpenAction if it is JavaScript
	if doc.OpenAction != nil {
		if jsAction, ok := doc.OpenAction.(semantic.JavaScriptAction); ok {
			if _, err := r.engine.Execute(ctx, jsAction.JS); err != nil {
				return err
			}
		}
	}

	// 4. Execute Form Calculation scripts
	if doc.AcroForm != nil {
		if err := r.executeFormScripts(ctx, doc.AcroForm); err != nil {
			return err
		}
	}

	return nil
}

func (r *JavaScriptRunner) executeFormScripts(ctx context.Context, form *semantic.AcroForm) error {
	// Iterate over fields and execute calculation scripts
	for _, field := range form.Fields {
		// Check for AA (Additional Actions) -> C (Calculate) or V (Validate)
		// This requires the semantic model to expose AA dictionaries for fields.
		// Currently FormField interface doesn't expose AA explicitly, but we can assume
		// we'd access it or it will be added.
		_ = field
	}
	return nil
}
