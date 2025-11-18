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
	resolver := d.makeResolver(rawDoc)

	for ref, obj := range rawDoc.Objects {
		rawStream, ok := obj.(raw.Stream)
		if !ok {
			continue
		}

		data := rawStream.RawData()

		names, params := filters.ExtractFilters(rawStream.Dictionary())
		if d.pipeline != nil && len(names) > 0 {
			decoded, err := d.pipeline.DecodeWithResolver(ctx, data, names, params, resolver)
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
		Raw:               rawDoc,
		Streams:           streams,
		Perms:             rawDoc.Permissions,
		Encrypted:         rawDoc.Encrypted,
		MetadataEncrypted: rawDoc.MetadataEncrypted,
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

func (d *decoderImpl) makeResolver(doc *raw.Document) filters.StreamResolver {
	return func(ctx context.Context, ref raw.ObjectRef) ([]byte, error) {
		obj, ok := doc.Objects[ref]
		if !ok {
			return nil, fmt.Errorf("object %v not found", ref)
		}
		stream, ok := obj.(raw.Stream)
		if !ok {
			return nil, fmt.Errorf("object %v is not a stream", ref)
		}
		data := stream.RawData()
		names, params := filters.ExtractFilters(stream.Dictionary())
		if len(names) == 0 || d.pipeline == nil {
			return data, nil
		}
		decoded, err := d.pipeline.Decode(ctx, data, names, params)
		if err != nil {
			return nil, fmt.Errorf("decode %v: %w", names, err)
		}
		return decoded, nil
	}
}
