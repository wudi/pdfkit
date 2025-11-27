package extensions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/scripting"
)

type mockEngine struct {
	executedScripts []string
	failFor         map[string]error
}

func (m *mockEngine) Execute(ctx context.Context, script string) (interface{}, error) {
	m.executedScripts = append(m.executedScripts, script)
	if err := m.failFor[script]; err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *mockEngine) RegisterDOM(dom scripting.PDFDOM) error {
	return nil
}

func TestJavaScriptRunner_OrderAndFormScripts(t *testing.T) {
	engine := &mockEngine{}
	runner := NewJavaScriptRunner(engine)

	fieldOne := &semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Name: "FieldOne",
			AdditionalActions: &semantic.AdditionalActions{
				C: semantic.JavaScriptAction{JS: "field-1"},
			},
		},
	}
	fieldTwo := &semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Name: "FieldTwo",
			AdditionalActions: &semantic.AdditionalActions{
				C: semantic.JavaScriptAction{JS: "field-2"},
			},
		},
	}

	doc := &semantic.Document{
		Names: &semantic.Names{
			JavaScript: map[string]semantic.JavaScriptAction{
				"Zeta":  {JS: "names-z"},
				"Alpha": {JS: "names-a"},
			},
		},
		OpenAction: semantic.JavaScriptAction{JS: "open-action"},
		AcroForm: &semantic.AcroForm{
			Fields:           []semantic.FormField{fieldOne, fieldTwo},
			CalculationOrder: []semantic.FormField{fieldTwo},
		},
	}

	if err := runner.Execute(context.Background(), doc); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	expected := []string{"names-a", "names-z", "open-action", "field-2", "field-1"}
	if len(engine.executedScripts) != len(expected) {
		t.Fatalf("expected %d scripts, got %d", len(expected), len(engine.executedScripts))
	}
	for i, script := range expected {
		if engine.executedScripts[i] != script {
			t.Fatalf("script %d mismatch: expected %q, got %q", i, script, engine.executedScripts[i])
		}
	}
}

func TestJavaScriptRunner_ErrorAggregation(t *testing.T) {
	engine := &mockEngine{
		failFor: map[string]error{
			"names-bad": errors.New("boom"),
		},
	}
	runner := NewJavaScriptRunner(engine)

	doc := &semantic.Document{
		Names: &semantic.Names{
			JavaScript: map[string]semantic.JavaScriptAction{
				"Good": {JS: "names-good"},
				"Bad":  {JS: "names-bad"},
			},
		},
	}

	err := runner.Execute(context.Background(), doc)
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	if want := "Names.JavaScript (Bad)"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to mention %q, got %v", want, err)
	}

	expected := []string{"names-bad", "names-good"}
	if len(engine.executedScripts) != len(expected) {
		t.Fatalf("expected %d scripts, got %d", len(expected), len(engine.executedScripts))
	}
	for i, script := range expected {
		if engine.executedScripts[i] != script {
			t.Fatalf("script %d mismatch: expected %q, got %q", i, script, engine.executedScripts[i])
		}
	}
}
