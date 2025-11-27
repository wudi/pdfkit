package resources

import (
	"context"
	"reflect"
	"testing"
	"unsafe"

	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func setDecoded(doc *semantic.Document, dec *decoded.DecodedDocument) {
	// Use reflection to set private field 'decoded'
	v := reflect.ValueOf(doc).Elem()
	f := v.FieldByName("decoded")
	// This requires the field to be addressable, which it is since doc is a pointer.
	// We need to bypass the package check.
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(dec))
}

func TestResolveWithInheritance(t *testing.T) {
	// Setup raw objects
	fontRef := raw.ObjectRef{Num: 10, Gen: 0}
	fontObj := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type": raw.NameObj{Val: "Font"},
		},
	}

	rawDoc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			fontRef: fontObj,
		},
	}

	decodedDoc := &decoded.DecodedDocument{
		Raw: rawDoc,
	}

	semDoc := &semantic.Document{}
	setDecoded(semDoc, decodedDoc)

	// Setup semantic resources
	fontRes := &semantic.Font{
		OriginalRef: fontRef,
	}

	page := &semantic.Page{
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"F1": fontRes,
			},
		},
	}

	resolver := NewResolver(semDoc)

	// Test finding the font
	obj, err := resolver.ResolveWithInheritance(context.Background(), CategoryFont, "F1", page)
	if err != nil {
		t.Fatalf("ResolveWithInheritance failed: %v", err)
	}

	if obj == nil {
		t.Fatal("Expected object, got nil")
	}

	dict, ok := obj.(raw.Dictionary)
	if !ok {
		t.Fatalf("Expected dictionary, got %T", obj)
	}

	if val, ok := dict.Get(raw.NameObj{Val: "Type"}); !ok || val.(raw.NameObj).Val != "Font" {
		t.Errorf("Unexpected object content: %v", dict)
	}

	// Test not finding a font
	_, err = resolver.ResolveWithInheritance(context.Background(), CategoryFont, "F2", page)
	if err == nil {
		t.Error("Expected error for missing font, got nil")
	}
}
