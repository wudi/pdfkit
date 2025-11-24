package optimize

import (
	"context"
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type Config struct {
	CombineDuplicateDirectObjects   bool
	CombineIdenticalIndirectObjects bool
	CombineDuplicateStreams         bool
	CompressStreams                 bool
	UseObjectStreams                bool
	ImageQuality                    int // 0-100, 0 means no change
	ImageUpperPPI                   float64
	CleanUnusedResources            bool
}

type Optimizer struct {
	config Config
}

func New(config Config) *Optimizer {
	return &Optimizer{config: config}
}

func (o *Optimizer) Optimize(ctx context.Context, doc *semantic.Document) error {
	if o.config.CleanUnusedResources {
		if err := o.cleanUnusedResources(ctx, doc); err != nil {
			return fmt.Errorf("failed to clean unused resources: %w", err)
		}
	}

	if o.config.ImageQuality > 0 || o.config.ImageUpperPPI > 0 {
		if err := o.optimizeImages(ctx, doc); err != nil {
			return fmt.Errorf("failed to optimize images: %w", err)
		}
	}

	// Only run raw optimizations if we have a decoded document
	if doc.Decoded() != nil && doc.Decoded().Raw != nil {
		return o.OptimizeRaw(ctx, doc.Decoded().Raw)
	}

	return nil
}

func (o *Optimizer) OptimizeRaw(ctx context.Context, doc *raw.Document) error {
	if o.config.CombineIdenticalIndirectObjects {
		if err := o.combineObjects(doc, true, true); err != nil {
			return fmt.Errorf("failed to combine identical indirect objects: %w", err)
		}
	}

	if o.config.CombineDuplicateStreams {
		if err := o.combineObjects(doc, true, false); err != nil {
			return fmt.Errorf("failed to combine duplicate streams: %w", err)
		}
	}

	if o.config.CombineDuplicateDirectObjects {
		if err := o.combineDuplicateDirectObjectsRaw(ctx, doc); err != nil {
			return fmt.Errorf("failed to combine duplicate direct objects: %w", err)
		}
	}

	return nil
}
