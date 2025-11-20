package extensions

import (
	"pdflib/ir/semantic"
	"pdflib/scripting"
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

func (r *JavaScriptRunner) Execute(ctx Context, doc *semantic.Document) error {
	return r.Transform(ctx, doc)
}

func (r *JavaScriptRunner) Transform(ctx Context, doc *semantic.Document) error {
	if r.engine == nil {
		return nil
	}

	// 1. Register DOM
	// In a real implementation, we would wrap doc in a PDFDOM implementation
	// r.engine.RegisterDOM(NewPDFDOM(doc))

	// 2. Execute Document-level scripts (Names -> JavaScript)
	// TODO: Iterate over Names.JavaScript and execute them

	// 3. Execute OpenAction if it is JavaScript
	if doc.Catalog != nil {
		// Check OpenAction (not currently in semantic.Catalog, need to add or access via raw)
		// Assuming we might have it in the future or via some other way.
	}

	// 4. Execute Form Calculation scripts
	if doc.AcroForm != nil {
		if err := r.executeFormScripts(ctx, doc.AcroForm); err != nil {
			return err
		}
	}

	return nil
}

func (r *JavaScriptRunner) executeFormScripts(ctx Context, form *semantic.AcroForm) error {
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
