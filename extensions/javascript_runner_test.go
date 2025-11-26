package extensions

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/scripting"
)

type mockEngine struct {
	executedScripts []string
}

func (m *mockEngine) Execute(ctx context.Context, script string) (interface{}, error) {
	m.executedScripts = append(m.executedScripts, script)
	return nil, nil
}

func (m *mockEngine) RegisterDOM(dom scripting.PDFDOM) error {
	return nil
}

func TestJavaScriptRunner_Execute(t *testing.T) {
	engine := &mockEngine{}
	runner := NewJavaScriptRunner(engine)

	doc := &semantic.Document{
		Names: &semantic.Names{
			JavaScript: map[string]semantic.JavaScriptAction{
				"Init": {JS: "console.log('Init')"},
			},
		},
		OpenAction: semantic.JavaScriptAction{JS: "app.alert('Open')"},
	}

	if err := runner.Execute(context.Background(), doc); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(engine.executedScripts) != 2 {
		t.Fatalf("expected 2 scripts executed, got %d", len(engine.executedScripts))
	}

	// Order of map iteration is random, so check existence
	foundInit := false
	foundOpen := false
	for _, s := range engine.executedScripts {
		if s == "console.log('Init')" {
			foundInit = true
		}
		if s == "app.alert('Open')" {
			foundOpen = true
		}
	}

	if !foundInit {
		t.Error("Init script not executed")
	}
	if !foundOpen {
		t.Error("OpenAction script not executed")
	}
}
