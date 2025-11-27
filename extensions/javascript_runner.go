package extensions

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/wudi/pdfkit/extensions/dom"
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

	collector := &scriptCollector{}

	// 1. Register DOM
	if err := r.engine.RegisterDOM(dom.New(doc)); err != nil {
		return err
	}

	// 2. Execute Document-level scripts (Names -> JavaScript)
	if err := r.executeNamedScripts(ctx, doc, collector); err != nil {
		return err
	}

	// 3. Execute OpenAction if it is JavaScript
	if err := r.executeOpenAction(ctx, doc, collector); err != nil {
		return err
	}

	// 4. Execute Form Calculation scripts
	if doc.AcroForm != nil {
		if err := r.executeFormScripts(ctx, doc.AcroForm, collector); err != nil {
			return err
		}
	}

	return collector.Err()
}

func (r *JavaScriptRunner) executeNamedScripts(ctx context.Context, doc *semantic.Document, collector *scriptCollector) error {
	if doc.Names == nil || len(doc.Names.JavaScript) == 0 {
		return nil
	}

	keys := make([]string, 0, len(doc.Names.JavaScript))
	for name := range doc.Names.JavaScript {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	for _, name := range keys {
		action := doc.Names.JavaScript[name]
		if err := r.runScript(ctx, action.JS); err != nil {
			if isContextError(err) {
				return err
			}
			collector.add("Names.JavaScript", name, err)
		}
	}

	return nil
}

func (r *JavaScriptRunner) executeOpenAction(ctx context.Context, doc *semantic.Document, collector *scriptCollector) error {
	if doc.OpenAction == nil {
		return nil
	}

	jsAction, ok := doc.OpenAction.(semantic.JavaScriptAction)
	if !ok {
		return nil
	}

	if err := r.runScript(ctx, jsAction.JS); err != nil {
		if isContextError(err) {
			return err
		}
		collector.add("OpenAction", "", err)
	}

	return nil
}

func (r *JavaScriptRunner) executeFormScripts(ctx context.Context, form *semantic.AcroForm, collector *scriptCollector) error {
	if form == nil {
		return nil
	}

	ordered := make([]semantic.FormField, 0, len(form.Fields))
	seen := make(map[semantic.FormField]struct{})

	for _, field := range form.CalculationOrder {
		if field == nil {
			continue
		}
		ordered = append(ordered, field)
		seen[field] = struct{}{}
	}

	for _, field := range form.Fields {
		if field == nil {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		ordered = append(ordered, field)
	}

	for _, field := range ordered {
		if field == nil {
			continue
		}
		additional := field.GetAdditionalActions()
		if additional == nil || additional.C == nil {
			continue
		}
		jsAction, ok := additional.C.(semantic.JavaScriptAction)
		if !ok || jsAction.JS == "" {
			continue
		}
		if err := r.runScript(ctx, jsAction.JS); err != nil {
			if isContextError(err) {
				return err
			}
			collector.add("AcroForm.Calculate", field.FieldName(), err)
		}
	}

	return nil
}

func (r *JavaScriptRunner) runScript(ctx context.Context, script string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if script == "" {
		return nil
	}
	_, err := r.engine.Execute(ctx, script)
	return err
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

type scriptCollector struct {
	errs []error
}

func (c *scriptCollector) add(stage, target string, err error) {
	if err == nil {
		return
	}
	label := stage
	if target != "" {
		label = fmt.Sprintf("%s (%s)", stage, target)
	}
	c.errs = append(c.errs, fmt.Errorf("%s: %w", label, err))
}

func (c *scriptCollector) Err() error {
	if len(c.errs) == 0 {
		return nil
	}
	return errors.Join(c.errs...)
}
