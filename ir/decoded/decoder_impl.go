package decoded

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/wudi/pdfkit/filters"
	"github.com/wudi/pdfkit/ir/raw"
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

	// Collect all stream tasks
	type task struct {
		ref raw.ObjectRef
		obj raw.Stream
	}
	var tasks []task
	for ref, obj := range rawDoc.Objects {
		if s, ok := obj.(raw.Stream); ok {
			tasks = append(tasks, task{ref: ref, obj: s})
		}
	}

	if len(tasks) == 0 {
		return &DecodedDocument{
			Raw:               rawDoc,
			Streams:           streams,
			Perms:             rawDoc.Permissions,
			Encrypted:         rawDoc.Encrypted,
			MetadataEncrypted: rawDoc.MetadataEncrypted,
		}, nil
	}

	// Determine concurrency
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}

	// Use a buffered channel as a semaphore to limit concurrency
	sem := make(chan struct{}, workers)
	// Channel to collect results
	type result struct {
		ref    raw.ObjectRef
		stream Stream
		err    error
	}
	results := make(chan result, len(tasks))

	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			// Check context again
			select {
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			default:
			}

			data := t.obj.RawData()
			names, params := filters.ExtractFilters(t.obj.Dictionary())

			if d.pipeline != nil && len(names) > 0 {
				decodedData, err := d.pipeline.DecodeWithResolver(ctx, data, names, params, resolver)
				if err != nil {
					results <- result{err: fmt.Errorf("decode filters %v for %v: %w", names, t.ref, err)}
					return
				}
				data = decodedData
			}

			results <- result{
				ref: t.ref,
				stream: decodedStream{
					raw:     t.obj,
					data:    data,
					filters: names,
				},
			}
		}(t)
	}

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for res := range results {
		if res.err != nil {
			return nil, res.err
		}
		streams[res.ref] = res.stream
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
