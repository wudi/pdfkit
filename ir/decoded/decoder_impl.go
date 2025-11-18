package decoded

import (
	"context"
	"fmt"

	"pdflib/filters"
	"pdflib/ir/raw"
)

// NewDecoder constructs a basic Decoder that applies filter decoding to streams.
func NewDecoder(p *filters.Pipeline) Decoder {
	return &decoderImpl{pipeline: p}
}

type decoderImpl struct {
	pipeline *filters.Pipeline
}

func (d *decoderImpl) Decode(ctx context.Context, rawDoc *raw.Document) (*DecodedDocument, error) {
	streams := make(map[raw.ObjectRef]Stream)

	for ref, obj := range rawDoc.Objects {
		rawStream, ok := obj.(raw.Stream)
		if !ok {
			continue
		}

		data := rawStream.RawData()

		names, params := extractFilters(rawStream.Dictionary())
		if d.pipeline != nil && len(names) > 0 {
			decoded, err := d.pipeline.Decode(ctx, data, names, params)
			if err != nil {
				return nil, fmt.Errorf("decode filters %v: %w", names, err)
			}
			data = decoded
		}

		streams[ref] = decodedStream{
			raw:     rawStream,
			data:    data,
			filters: names,
		}
	}

	return &DecodedDocument{
		Raw:     rawDoc,
		Streams: streams,
	}, nil
}

type decodedStream struct {
	raw     raw.Stream
	data    []byte
	filters []string
}

func (s decodedStream) Raw() raw.Object            { return s.raw }
func (s decodedStream) Type() string               { return s.raw.Type() }
func (s decodedStream) Dictionary() raw.Dictionary { return s.raw.Dictionary() }
func (s decodedStream) Data() []byte               { return s.data }
func (s decodedStream) Filters() []string          { return s.filters }

func extractFilters(dict raw.Dictionary) ([]string, []raw.Dictionary) {
	var names []string
	var params []raw.Dictionary

	filterObj, ok := dict.Get(raw.NameLiteral("Filter"))
	if !ok {
		return names, params
	}

	switch f := filterObj.(type) {
	case raw.Name:
		names = append(names, f.Value())
	case *raw.ArrayObj:
		for _, item := range f.Items {
			if n, ok := item.(raw.Name); ok {
				names = append(names, n.Value())
			}
		}
	}

	// DecodeParms optional
	if len(names) > 0 {
		if pObj, ok := dict.Get(raw.NameLiteral("DecodeParms")); ok {
			switch p := pObj.(type) {
			case raw.Dictionary:
				params = append(params, p)
			case *raw.ArrayObj:
				for _, item := range p.Items {
					if d, ok := item.(raw.Dictionary); ok {
						params = append(params, d)
					}
				}
			}
		}
	}

	return names, params
}
