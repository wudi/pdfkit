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
	rawParser       raw.Parser
	decoder         decoded.Decoder
	semanticBuilder semantic.Builder
	recovery        recovery.Strategy
	logger          observability.Logger
}

// NewDefault constructs a pipeline with basic components (raw parser, filter decoder, no-op security, minimal semantic builder).
func NewDefault() *Pipeline {
	fp := filters.NewPipeline(
		[]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
		},
		filters.Limits{},
	)
	return &Pipeline{
		rawParser:       parser.NewDocumentParser(parser.Config{}),
		decoder:         decoded.NewDecoder(fp, security.NoopHandler()),
		semanticBuilder: semantic.NewBuilder(),
	}
}

// Parse orchestrates Raw -> Decoded -> Semantic pipeline.
func (p *Pipeline) Parse(ctx context.Context, r io.ReaderAt) (*semantic.Document, error) {
	rawDoc, err := p.rawParser.Parse(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("raw parsing failed: %w", err)
	}

	decodedDoc, err := p.decoder.Decode(ctx, rawDoc)
	if err != nil {
		return nil, fmt.Errorf("decoding failed: %w", err)
	}

	semDoc, err := p.semanticBuilder.Build(ctx, decodedDoc)
	if err != nil {
		return nil, fmt.Errorf("semantic building failed: %w", err)
	}

	return semDoc, nil
}
