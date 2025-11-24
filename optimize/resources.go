package optimize

import (
	"context"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func (o *Optimizer) cleanUnusedResources(ctx context.Context, doc *semantic.Document) error {
	rawDoc := doc.Decoded().Raw
	if rawDoc == nil {
		return nil
	}

	// 1. Mark reachable objects
	reachable := make(map[raw.ObjectRef]bool)

	// Start from Trailer
	if rawDoc.Trailer != nil {
		o.markReachable(rawDoc, rawDoc.Trailer, reachable)
	}

	// 2. Sweep
	for ref := range rawDoc.Objects {
		if !reachable[ref] {
			delete(rawDoc.Objects, ref)
		}
	}

	return nil
}

func (o *Optimizer) markReachable(doc *raw.Document, obj raw.Object, reachable map[raw.ObjectRef]bool) {
	if obj == nil {
		return
	}

	switch t := obj.(type) {
	case raw.Reference:
		ref := t.Ref()
		if reachable[ref] {
			return
		}
		reachable[ref] = true
		if target, ok := doc.Objects[ref]; ok {
			o.markReachable(doc, target, reachable)
		}
	case raw.Array:
		for i := 0; i < t.Len(); i++ {
			v, _ := t.Get(i)
			o.markReachable(doc, v, reachable)
		}
	case raw.Dictionary:
		for _, k := range t.Keys() {
			v, _ := t.Get(k)
			o.markReachable(doc, v, reachable)
		}
	case raw.Stream:
		o.markReachable(doc, t.Dictionary(), reachable)
	}
}
