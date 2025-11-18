package ir

import (
	"context"
	"fmt"
	"io"

	"pdflib/filters"
	"pdflib/ir/decoded"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/observability"
	"pdflib/parser"
	"pdflib/recovery"
	"pdflib/security"
)

type Pipeline struct {
	rawParser        raw.Parser
	filterPipeline   *filters.Pipeline
	securityOverride security.Handler
	semanticBuilder  semantic.Builder
	recovery         recovery.Strategy
	logger           observability.Logger
	password         string
	tracer           observability.Tracer
}

// NewDefault constructs a pipeline with basic components (raw parser, filter decoder, no-op security, minimal semantic builder).
func NewDefault() *Pipeline {
	fp := filters.NewPipeline(
		[]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewRunLengthDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
			filters.NewCryptDecoder(),
		},
		filters.Limits{},
	)
	return &Pipeline{
		rawParser:       parser.NewDocumentParser(parser.Config{}),
		filterPipeline:  fp,
		semanticBuilder: semantic.NewBuilder(),
	}
}

// WithPassword sets the password used to open encrypted PDFs.
func (p *Pipeline) WithPassword(pwd string) *Pipeline {
	p.password = pwd
	return p
}

// WithTracer sets the tracer used for parsing spans.
func (p *Pipeline) WithTracer(tracer observability.Tracer) *Pipeline {
	p.tracer = tracer
	return p
}

// Parse orchestrates Raw -> Decoded -> Semantic pipeline.
func (p *Pipeline) Parse(ctx context.Context, r io.ReaderAt) (doc *semantic.Document, err error) {
	if dp, ok := p.rawParser.(*parser.DocumentParser); ok {
		dp.SetPassword(p.password)
	}
	tracer := pipelineTracer(p.tracer)
	ctx, pipeSpan := tracer.StartSpan(ctx, "pipeline.parse")
	defer func() {
		if err != nil {
			pipeSpan.SetError(err)
		}
		pipeSpan.Finish()
	}()

	ctx, rawSpan := tracer.StartSpan(ctx, "pipeline.raw_parse")
	rawDoc, err := p.rawParser.Parse(ctx, r)
	if err != nil {
		rawSpan.SetError(err)
		rawSpan.Finish()
		return nil, fmt.Errorf("raw parsing failed: %w", err)
	}
	rawSpan.Finish()

	decoder := decoded.NewDecoder(p.filterPipeline)

	ctx, decodeSpan := tracer.StartSpan(ctx, "pipeline.decode")
	decodedDoc, err := decoder.Decode(ctx, rawDoc)
	if err != nil {
		decodeSpan.SetError(err)
		decodeSpan.Finish()
		return nil, fmt.Errorf("decoding failed: %w", err)
	}
	decodeSpan.Finish()

	ctx, buildSpan := tracer.StartSpan(ctx, "pipeline.semantic_build")
	semDoc, err := p.semanticBuilder.Build(ctx, decodedDoc)
	if err != nil {
		buildSpan.SetError(err)
		buildSpan.Finish()
		return nil, fmt.Errorf("semantic building failed: %w", err)
	}
	buildSpan.Finish()
	pipeSpan.SetTag("pages", len(semDoc.Pages))

	return semDoc, nil
}

func pipelineTracer(t observability.Tracer) observability.Tracer {
	if t != nil {
		return t
	}
	return observability.NopTracer()
}
