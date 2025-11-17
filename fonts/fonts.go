package fonts

import "pdflib/ir/semantic"

type GlyphUsage struct { Font *semantic.Font; GlyphIDs []uint16 }

type GlyphAnalyzer interface { Analyze(ctx Context, doc *semantic.Document) (map[string]GlyphUsage, error) }

type SubsetPlan struct { OriginalFont *semantic.Font; OldToNew map[uint16]uint16; NewToOld map[uint16]uint16; GlyphCount int; SubsetTag string }

type SubsetPlanner interface { Plan(ctx Context, usage GlyphUsage) (*SubsetPlan, error) }

type SubsetGenerator interface { Generate(ctx Context, plan *SubsetPlan, fontData []byte) ([]byte, error) }

type FontEmbedder interface { Embed(ctx Context, doc *semantic.Document, plan *SubsetPlan, subsetData []byte) error }

type SubsettingPipeline struct { analyzer GlyphAnalyzer; planner SubsetPlanner; generator SubsetGenerator; embedder FontEmbedder }

func (p *SubsettingPipeline) Subset(ctx Context, doc *semantic.Document) error { return nil }

type Context interface{ Done() <-chan struct{} }
