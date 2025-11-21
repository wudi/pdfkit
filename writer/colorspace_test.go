package writer

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type mockSerializationContext struct {
	objects map[raw.ObjectRef]raw.Object
	nextID  int
}

func (m *mockSerializationContext) NextRef() raw.ObjectRef {
	ref := raw.ObjectRef{Num: m.nextID, Gen: 0}
	m.nextID++
	return ref
}

func (m *mockSerializationContext) AddObject(ref raw.ObjectRef, obj raw.Object) {
	m.objects[ref] = obj
}

func (m *mockSerializationContext) PageRef(index int) *raw.ObjectRef {
	return nil
}

func TestColorSpaceSerialization(t *testing.T) {
	ctx := &mockSerializationContext{
		objects: make(map[raw.ObjectRef]raw.Object),
		nextID:  1,
	}
	fs := newFunctionSerializer()
	csSerializer := newColorSpaceSerializer(fs)

	t.Run("Separation", func(t *testing.T) {
		sep := &semantic.SeparationColorSpace{
			Name:      "PANTONE 123 C",
			Alternate: &semantic.DeviceColorSpace{Name: "DeviceCMYK"},
			TintTransform: &semantic.ExponentialFunction{
				BaseFunction: semantic.BaseFunction{
					Type:   2,
					Domain: []float64{0, 1},
					Range:  []float64{0, 1, 0, 1, 0, 1, 0, 1},
				},
				C0: []float64{0, 0, 0, 0},
				C1: []float64{0, 0.2, 0.8, 0},
				N:  1,
			},
		}

		obj := csSerializer.Serialize(sep, ctx)
		arr, ok := obj.(*raw.ArrayObj)
		if !ok {
			t.Fatalf("expected array, got %T", obj)
		}
		if arr.Len() != 4 {
			t.Fatalf("expected 4 elements, got %d", arr.Len())
		}
		if name, ok := arr.Items[0].(raw.NameObj); !ok || name.Value() != "Separation" {
			t.Errorf("expected Separation, got %v", arr.Items[0])
		}
		if name, ok := arr.Items[1].(raw.NameObj); !ok || name.Value() != "PANTONE 123 C" {
			t.Errorf("expected PANTONE 123 C, got %v", arr.Items[1])
		}
		if alt, ok := arr.Items[2].(raw.NameObj); !ok || alt.Value() != "DeviceCMYK" {
			t.Errorf("expected DeviceCMYK, got %v", arr.Items[2])
		}
		if _, ok := arr.Items[3].(raw.RefObj); !ok {
			t.Errorf("expected function ref, got %v", arr.Items[3])
		}
	})

	t.Run("DeviceN", func(t *testing.T) {
		dn := &semantic.DeviceNColorSpace{
			Names:     []string{"Orange", "Green"},
			Alternate: &semantic.DeviceColorSpace{Name: "DeviceCMYK"},
			TintTransform: &semantic.ExponentialFunction{
				BaseFunction: semantic.BaseFunction{
					Type:   2,
					Domain: []float64{0, 1},
					Range:  []float64{0, 1, 0, 1, 0, 1, 0, 1},
				},
				C0: []float64{0, 0, 0, 0},
				C1: []float64{0, 0.5, 1, 0},
				N:  1,
			},
			Attributes: &semantic.DeviceNAttributes{
				Subtype: "DeviceN",
			},
		}

		obj := csSerializer.Serialize(dn, ctx)
		arr, ok := obj.(*raw.ArrayObj)
		if !ok {
			t.Fatalf("expected array, got %T", obj)
		}
		if arr.Len() != 5 {
			t.Fatalf("expected 5 elements, got %d", arr.Len())
		}
		if name, ok := arr.Items[0].(raw.NameObj); !ok || name.Value() != "DeviceN" {
			t.Errorf("expected DeviceN, got %v", arr.Items[0])
		}
		namesArr, ok := arr.Items[1].(*raw.ArrayObj)
		if !ok {
			t.Fatalf("expected names array, got %T", arr.Items[1])
		}
		if namesArr.Len() != 2 {
			t.Errorf("expected 2 names, got %d", namesArr.Len())
		}
		if alt, ok := arr.Items[2].(raw.NameObj); !ok || alt.Value() != "DeviceCMYK" {
			t.Errorf("expected DeviceCMYK, got %v", arr.Items[2])
		}
		if _, ok := arr.Items[3].(raw.RefObj); !ok {
			t.Errorf("expected function ref, got %v", arr.Items[3])
		}
		if attr, ok := arr.Items[4].(*raw.DictObj); !ok {
			t.Errorf("expected attributes dict, got %T", arr.Items[4])
		} else {
			if s, ok := attr.Get(raw.NameLiteral("Subtype")); !ok {
				t.Error("expected Subtype in attributes")
			} else if n, ok := s.(raw.NameObj); !ok || n.Value() != "DeviceN" {
				t.Errorf("expected Subtype DeviceN, got %v", s)
			}
		}
	})
}
