package optimize

import (
	"context"
	"fmt"

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

	if o.config.CombineIdenticalIndirectObjects {
		if err := o.combineIdenticalIndirectObjects(ctx, doc); err != nil {
			return fmt.Errorf("failed to combine identical indirect objects: %w", err)
		}
	}

	if o.config.CombineDuplicateStreams {
		if err := o.combineDuplicateStreams(ctx, doc); err != nil {
			return fmt.Errorf("failed to combine duplicate streams: %w", err)
		}
	}

	if o.config.CombineDuplicateDirectObjects {
		if err := o.combineDuplicateDirectObjects(ctx, doc); err != nil {
			return fmt.Errorf("failed to combine duplicate direct objects: %w", err)
		}
	}

	if o.config.CompressStreams {
		if err := o.compressStreams(ctx, doc); err != nil {
			return fmt.Errorf("failed to compress streams: %w", err)
		}
	}

	if o.config.ImageQuality > 0 || o.config.ImageUpperPPI > 0 {
		if err := o.optimizeImages(ctx, doc); err != nil {
			return fmt.Errorf("failed to optimize images: %w", err)
		}
	}

	return nil
}
