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
		},
		filters.Limits{},
	)
	return &Pipeline{
		rawParser:       parser.NewDocumentParser(parser.Config{}),
		filterPipeline:  fp,
		semanticBuilder: semantic.NewBuilder(),
	}
}

// Parse orchestrates Raw -> Decoded -> Semantic pipeline.
func (p *Pipeline) Parse(ctx context.Context, r io.ReaderAt) (*semantic.Document, error) {
	rawDoc, err := p.rawParser.Parse(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("raw parsing failed: %w", err)
	}

	sec := p.buildSecurity(rawDoc)
	decoder := decoded.NewDecoder(p.filterPipeline, sec)

	decodedDoc, err := decoder.Decode(ctx, rawDoc)
	if err != nil {
		return nil, fmt.Errorf("decoding failed: %w", err)
	}

	semDoc, err := p.semanticBuilder.Build(ctx, decodedDoc)
	if err != nil {
		return nil, fmt.Errorf("semantic building failed: %w", err)
	}

	return semDoc, nil
}

func (p *Pipeline) buildSecurity(rawDoc *raw.Document) security.Handler {
	if p.securityOverride != nil {
		return p.securityOverride
	}
	trailerDict, _ := rawDoc.Trailer.(*raw.DictObj)
	if trailerDict == nil {
		return security.NoopHandler()
	}
	encObj, ok := trailerDict.Get(raw.NameObj{Val: "Encrypt"})
	if !ok {
		return security.NoopHandler()
	}
	var encDict *raw.DictObj
	switch v := encObj.(type) {
	case *raw.DictObj:
		encDict = v
	case raw.RefObj:
		if obj, ok := rawDoc.Objects[v.R]; ok {
			if d, ok := obj.(*raw.DictObj); ok {
				encDict = d
			}
		}
	}
	if encDict == nil {
		return security.NoopHandler()
	}
	var fileID []byte
	if idObj, ok := trailerDict.Get(raw.NameObj{Val: "ID"}); ok {
		if arr, ok := idObj.(*raw.ArrayObj); ok && arr.Len() > 0 {
			if s, ok := arr.Items[0].(raw.StringObj); ok {
				fileID = s.Value()
			}
		}
	}
	builder := &security.HandlerBuilder{}
	handler, err := builder.WithEncryptDict(encDict).WithTrailer(trailerDict).WithFileID(fileID).Build()
	if err != nil {
		return security.NoopHandler()
	}
	_ = handler.Authenticate("") // default empty password
	return handler
}
