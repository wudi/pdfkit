package optimize

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestCombineIdenticalIndirectObjects(t *testing.T) {
	// Create raw objects
	obj1 := raw.NewArray(raw.NumberInt(1), raw.NumberInt(2))
	obj2 := raw.NewArray(raw.NumberInt(1), raw.NumberInt(2)) // Identical to obj1
	obj3 := raw.NewArray(raw.NumberInt(3))

	rawDoc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}: obj1,
			{Num: 2, Gen: 0}: obj2,
			{Num: 3, Gen: 0}: obj3,
			{Num: 4, Gen: 0}: raw.NewArray(raw.Ref(1, 0), raw.Ref(2, 0)), // References both
		},
	}

	decDoc := &decoded.DecodedDocument{
		Raw:     rawDoc,
		Streams: make(map[raw.ObjectRef]decoded.Stream),
	}

	builder := semantic.NewBuilder()
	semDoc, err := builder.Build(context.Background(), decDoc)
	if err != nil {
		t.Fatalf("Failed to build semantic doc: %v", err)
	}

	opt := New(Config{
		CombineIdenticalIndirectObjects: true,
	})

	if err := opt.Optimize(context.Background(), semDoc); err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	// Check results
	// Obj 1 and 2 should be merged.
	// One of them should be gone.
	// We expect 3 objects: the merged one, obj3, and obj4.
	if len(rawDoc.Objects) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(rawDoc.Objects))
	}

	// Check references in obj4
	obj4 := rawDoc.Objects[raw.ObjectRef{Num: 4, Gen: 0}].(*raw.ArrayObj)
	ref1 := obj4.Items[0].(raw.Reference).Ref()
	ref2 := obj4.Items[1].(raw.Reference).Ref()

	if ref1 != ref2 {
		t.Errorf("Expected references to be identical, got %v and %v", ref1, ref2)
	}
}

func TestCombineDuplicateDirectObjects(t *testing.T) {
	// Create raw objects with duplicate direct objects
	// Obj1: [ [1 2] ]
	// Obj2: [ [1 2] ]
	// [1 2] is duplicated direct object.

	inner1 := raw.NewArray(raw.NumberInt(1), raw.NumberInt(2))
	inner2 := raw.NewArray(raw.NumberInt(1), raw.NumberInt(2))

	obj1 := raw.NewArray(inner1)
	obj2 := raw.NewArray(inner2)

	rawDoc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}: obj1,
			{Num: 2, Gen: 0}: obj2,
		},
	}

	decDoc := &decoded.DecodedDocument{
		Raw:     rawDoc,
		Streams: make(map[raw.ObjectRef]decoded.Stream),
	}

	builder := semantic.NewBuilder()
	semDoc, err := builder.Build(context.Background(), decDoc)
	if err != nil {
		t.Fatalf("Failed to build semantic doc: %v", err)
	}

	opt := New(Config{
		CombineDuplicateDirectObjects: true,
	})

	if err := opt.Optimize(context.Background(), semDoc); err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	// We expect a new object to be created for [1 2]
	// And obj1, obj2 to refer to it.
	// Total objects: 1, 2, and new one (3).
	if len(rawDoc.Objects) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(rawDoc.Objects))
	}

	// Check obj1 content
	o1 := rawDoc.Objects[raw.ObjectRef{Num: 1, Gen: 0}].(*raw.ArrayObj)
	if _, ok := o1.Items[0].(raw.Reference); !ok {
		t.Errorf("Expected obj1 to contain reference, got %T", o1.Items[0])
	}
}
