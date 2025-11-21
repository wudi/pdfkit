package layout

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/parser"
	"github.com/wudi/pdfkit/writer"
)

func TestHTMLForms(t *testing.T) {
	htmlContent := `
		<h1>Form Test</h1>
		<form>
			<input type="text" name="fullname" value="John Doe">
			<input type="checkbox" name="subscribe" checked>
			<input type="radio" name="gender" value="male" checked>
			<input type="radio" name="gender" value="female">
			<textarea name="comments">Great PDF lib!</textarea>
			<select name="country">
				<option value="us">USA</option>
				<option value="ca" selected>Canada</option>
			</select>
		</form>
	`

	b := builder.NewBuilder()
	engine := NewEngine(b)
	if err := engine.RenderHTML(htmlContent); err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify fields in semantic document
	if doc.AcroForm == nil {
		t.Fatal("AcroForm is nil")
	}

	fields := doc.AcroForm.Fields
	if len(fields) != 6 { // fullname, subscribe, gender(male), gender(female), comments, country
		t.Errorf("Expected 6 fields, got %d", len(fields))
	}

	fieldMap := make(map[string]semantic.FormField)
	for _, f := range fields {
		// For radio buttons with same name, they are separate widgets but might share a parent field in full hierarchy.
		// Here we just check if they exist.
		// Note: In our simple implementation, we add them as separate fields with same name.
		// A real implementation would group them.
		// We'll use a key that includes value for radios if needed, or just count.
		fieldMap[f.FieldName()] = f
	}

	// Check Fullname
	if f, ok := fieldMap["fullname"]; !ok {
		t.Error("Missing fullname field")
	} else if tf, ok := f.(*semantic.TextFormField); !ok {
		t.Error("fullname is not a text field")
	} else if tf.Value != "John Doe" {
		t.Errorf("fullname value mismatch: got %s", tf.Value)
	}

	// Check Subscribe
	if f, ok := fieldMap["subscribe"]; !ok {
		t.Error("Missing subscribe field")
	} else if bf, ok := f.(*semantic.ButtonFormField); !ok {
		t.Error("subscribe is not a button field")
	} else if !bf.IsCheck {
		t.Error("subscribe is not a checkbox")
	} else if !bf.Checked {
		t.Error("subscribe is not checked")
	}

	// Check Comments
	if f, ok := fieldMap["comments"]; !ok {
		t.Error("Missing comments field")
	} else if tf, ok := f.(*semantic.TextFormField); !ok {
		t.Error("comments is not a text field")
	} else if tf.Value != "Great PDF lib!" {
		t.Errorf("comments value mismatch: got %s", tf.Value)
	}

	// Check Country
	if f, ok := fieldMap["country"]; !ok {
		t.Error("Missing country field")
	} else if cf, ok := f.(*semantic.ChoiceFormField); !ok {
		t.Error("country is not a choice field")
	} else if len(cf.Selected) != 1 || cf.Selected[0] != "ca" {
		t.Errorf("country selection mismatch: got %v", cf.Selected)
	}

	// Write and Parse back to verify end-to-end
	var buf bytes.Buffer
	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, &buf, writer.Config{Deterministic: true}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	rawParser := parser.NewDocumentParser(parser.Config{})
	_, err = rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
}
