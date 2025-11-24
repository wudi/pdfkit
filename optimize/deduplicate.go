package optimize

import (
	"context"
	"sort"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func (o *Optimizer) combineIdenticalIndirectObjects(ctx context.Context, doc *semantic.Document) error {
	return o.combineObjects(doc.Decoded().Raw, true, true)
}

func (o *Optimizer) combineDuplicateStreams(ctx context.Context, doc *semantic.Document) error {
	if o.config.CombineIdenticalIndirectObjects {
		return nil // Already handled
	}
	return o.combineObjects(doc.Decoded().Raw, true, false)
}

func (o *Optimizer) combineObjects(rawDoc *raw.Document, includeStreams, includeOthers bool) error {
	if rawDoc == nil {
		return nil
	}

	changed := true
	for changed {
		changed = false
		seen := make(map[string]raw.ObjectRef)
		replacements := make(map[raw.ObjectRef]raw.ObjectRef)

		var refs []raw.ObjectRef
		for ref := range rawDoc.Objects {
			refs = append(refs, ref)
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Num != refs[j].Num {
				return refs[i].Num < refs[j].Num
			}
			return refs[i].Gen < refs[j].Gen
		})

		for _, ref := range refs {
			obj := rawDoc.Objects[ref]
			isStream := obj.Type() == "stream"

			if isStream && !includeStreams {
				continue
			}
			if !isStream && !includeOthers {
				continue
			}

			h := hashObject(obj)
			if original, ok := seen[h]; ok {
				replacements[ref] = original
				changed = true
			} else {
				seen[h] = ref
			}
		}

		if len(replacements) > 0 {
			o.applyReplacements(rawDoc, replacements)
			for dup := range replacements {
				delete(rawDoc.Objects, dup)
			}
		}
	}
	return nil
}

func (o *Optimizer) combineDuplicateDirectObjects(ctx context.Context, doc *semantic.Document) error {
	rawDoc := doc.Decoded().Raw
	if rawDoc == nil {
		return nil
	}
	return o.combineDuplicateDirectObjectsRaw(ctx, rawDoc)
}

func (o *Optimizer) combineDuplicateDirectObjectsRaw(ctx context.Context, rawDoc *raw.Document) error {
	if rawDoc == nil {
		return nil
	}

	// 1. Count occurrences
	counts := make(map[string]int)
	samples := make(map[string]raw.Object)

	var countVisitor func(obj raw.Object)
	countVisitor = func(obj raw.Object) {
		if obj == nil {
			return
		}
		if obj.IsIndirect() {
			return
		}

		switch t := obj.(type) {
		case raw.Array, raw.Dictionary:
			h := hashObject(obj)
			counts[h]++
			if counts[h] == 1 {
				samples[h] = obj
			}

			if arr, ok := t.(raw.Array); ok {
				for i := 0; i < arr.Len(); i++ {
					v, _ := arr.Get(i)
					countVisitor(v)
				}
			} else if dict, ok := t.(raw.Dictionary); ok {
				for _, k := range dict.Keys() {
					v, _ := dict.Get(k)
					countVisitor(v)
				}
			}
		}
	}

	for _, obj := range rawDoc.Objects {
		// Don't count the top-level object itself, as it is already indirect.
		// Only count its children.
		switch t := obj.(type) {
		case raw.Array:
			for i := 0; i < t.Len(); i++ {
				v, _ := t.Get(i)
				countVisitor(v)
			}
		case raw.Dictionary:
			for _, k := range t.Keys() {
				v, _ := t.Get(k)
				countVisitor(v)
			}
		case raw.Stream:
			countVisitor(t.Dictionary())
		}
	}
	if rawDoc.Trailer != nil {
		countVisitor(rawDoc.Trailer)
	}

	// 2. Identify candidates
	candidates := make(map[string]raw.ObjectRef)
	nextID := 1
	for ref := range rawDoc.Objects {
		if ref.Num >= nextID {
			nextID = ref.Num + 1
		}
	}

	for h, count := range counts {
		if count > 1 {
			ref := raw.ObjectRef{Num: nextID, Gen: 0}
			nextID++
			rawDoc.Objects[ref] = samples[h]
			candidates[h] = ref
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// 3. Replace
	o.replaceDirectObjects(rawDoc, candidates)

	return nil
}

func (o *Optimizer) replaceDirectObjects(doc *raw.Document, candidates map[string]raw.ObjectRef) {
	for _, obj := range doc.Objects {
		o.replaceDirectInObject(obj, candidates)
	}
	if doc.Trailer != nil {
		o.replaceDirectInObject(doc.Trailer, candidates)
	}
}

func (o *Optimizer) replaceDirectInObject(obj raw.Object, candidates map[string]raw.ObjectRef) {
	switch t := obj.(type) {
	case *raw.ArrayObj:
		for i, val := range t.Items {
			if val.IsIndirect() {
				continue
			}
			if _, ok := val.(raw.Array); ok {
				h := hashObject(val)
				if ref, ok := candidates[h]; ok {
					t.Items[i] = raw.Ref(ref.Num, ref.Gen)
					continue
				}
			}
			if _, ok := val.(raw.Dictionary); ok {
				h := hashObject(val)
				if ref, ok := candidates[h]; ok {
					t.Items[i] = raw.Ref(ref.Num, ref.Gen)
					continue
				}
			}
			o.replaceDirectInObject(val, candidates)
		}
	case raw.Dictionary:
		for _, key := range t.Keys() {
			val, _ := t.Get(key)
			if val.IsIndirect() {
				continue
			}

			replaced := false
			if _, ok := val.(raw.Array); ok {
				h := hashObject(val)
				if ref, ok := candidates[h]; ok {
					t.Set(key, raw.Ref(ref.Num, ref.Gen))
					replaced = true
				}
			} else if _, ok := val.(raw.Dictionary); ok {
				h := hashObject(val)
				if ref, ok := candidates[h]; ok {
					t.Set(key, raw.Ref(ref.Num, ref.Gen))
					replaced = true
				}
			}

			if !replaced {
				o.replaceDirectInObject(val, candidates)
			}
		}
	case raw.Stream:
		o.replaceDirectInObject(t.Dictionary(), candidates)
	}
}

func (o *Optimizer) applyReplacements(doc *raw.Document, replacements map[raw.ObjectRef]raw.ObjectRef) {
	for _, obj := range doc.Objects {
		o.replaceRefsInObject(obj, replacements)
	}
	if doc.Trailer != nil {
		o.replaceRefsInObject(doc.Trailer, replacements)
	}
}

func (o *Optimizer) replaceRefsInObject(obj raw.Object, replacements map[raw.ObjectRef]raw.ObjectRef) {
	switch t := obj.(type) {
	case *raw.ArrayObj:
		for i, val := range t.Items {
			if ref, ok := val.(raw.Reference); ok {
				if newRef, found := replacements[ref.Ref()]; found {
					t.Items[i] = raw.Ref(newRef.Num, newRef.Gen)
				}
			} else {
				o.replaceRefsInObject(val, replacements)
			}
		}
	case raw.Dictionary:
		for _, key := range t.Keys() {
			val, _ := t.Get(key)
			if ref, ok := val.(raw.Reference); ok {
				if newRef, found := replacements[ref.Ref()]; found {
					t.Set(key, raw.Ref(newRef.Num, newRef.Gen))
				}
			} else {
				o.replaceRefsInObject(val, replacements)
			}
		}
	case raw.Stream:
		o.replaceRefsInObject(t.Dictionary(), replacements)
	}
}
