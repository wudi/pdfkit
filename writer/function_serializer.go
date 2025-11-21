package writer

import (
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

// FunctionSerializer serializes semantic functions into raw objects.
type FunctionSerializer interface {
	Serialize(f semantic.Function, ctx SerializationContext) raw.ObjectRef
}

type defaultFunctionSerializer struct{}

func newFunctionSerializer() FunctionSerializer {
	return &defaultFunctionSerializer{}
}

func (s *defaultFunctionSerializer) Serialize(f semantic.Function, ctx SerializationContext) raw.ObjectRef {
	if f == nil {
		return raw.ObjectRef{}
	}

	// Check if already serialized (if the function object has a reference)
	// Note: The semantic model's Reference() method returns the original reference if it was parsed.
	// For new objects, we might need a way to cache them during serialization to avoid duplication.
	// However, SerializationContext doesn't provide a cache.
	// For now, we'll always create a new object.

	ref := ctx.NextRef()
	dict := raw.Dict()

	dict.Set(raw.NameLiteral("FunctionType"), raw.NumberInt(int64(f.FunctionType())))

	if domain := f.FunctionDomain(); len(domain) > 0 {
		arr := raw.NewArray()
		for _, v := range domain {
			arr.Append(raw.NumberFloat(v))
		}
		dict.Set(raw.NameLiteral("Domain"), arr)
	}

	if rng := f.FunctionRange(); len(rng) > 0 {
		arr := raw.NewArray()
		for _, v := range rng {
			arr.Append(raw.NumberFloat(v))
		}
		dict.Set(raw.NameLiteral("Range"), arr)
	}

	switch t := f.(type) {
	case *semantic.SampledFunction:
		if len(t.Size) > 0 {
			arr := raw.NewArray()
			for _, v := range t.Size {
				arr.Append(raw.NumberInt(int64(v)))
			}
			dict.Set(raw.NameLiteral("Size"), arr)
		}
		dict.Set(raw.NameLiteral("BitsPerSample"), raw.NumberInt(int64(t.BitsPerSample)))
		if t.Order != 0 {
			dict.Set(raw.NameLiteral("Order"), raw.NumberInt(int64(t.Order)))
		}
		if len(t.Encode) > 0 {
			arr := raw.NewArray()
			for _, v := range t.Encode {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("Encode"), arr)
		}
		if len(t.Decode) > 0 {
			arr := raw.NewArray()
			for _, v := range t.Decode {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("Decode"), arr)
		}
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.Samples))))
		ctx.AddObject(ref, raw.NewStream(dict, t.Samples))

	case *semantic.ExponentialFunction:
		if len(t.C0) > 0 {
			arr := raw.NewArray()
			for _, v := range t.C0 {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("C0"), arr)
		}
		if len(t.C1) > 0 {
			arr := raw.NewArray()
			for _, v := range t.C1 {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("C1"), arr)
		}
		dict.Set(raw.NameLiteral("N"), raw.NumberFloat(t.N))
		ctx.AddObject(ref, dict)

	case *semantic.StitchingFunction:
		if len(t.Functions) > 0 {
			arr := raw.NewArray()
			for _, subFunc := range t.Functions {
				subRef := s.Serialize(subFunc, ctx)
				arr.Append(raw.Ref(subRef.Num, subRef.Gen))
			}
			dict.Set(raw.NameLiteral("Functions"), arr)
		}
		if len(t.Bounds) > 0 {
			arr := raw.NewArray()
			for _, v := range t.Bounds {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("Bounds"), arr)
		}
		if len(t.Encode) > 0 {
			arr := raw.NewArray()
			for _, v := range t.Encode {
				arr.Append(raw.NumberFloat(v))
			}
			dict.Set(raw.NameLiteral("Encode"), arr)
		}
		ctx.AddObject(ref, dict)

	case *semantic.PostScriptFunction:
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(t.Code))))
		ctx.AddObject(ref, raw.NewStream(dict, t.Code))
	}

	return ref
}
