package decoded

import (
	"context"
	"testing"

	"pdflib/filters"
	"pdflib/ir/raw"
)

type uppercaseDecoder struct{}

func (uppercaseDecoder) Name() string { return "Upper" }
func (uppercaseDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	out := make([]byte, len(in))
	for i, b := range in {
		if b >= 'a' && b <= 'z' {
			out[i] = b - 32
		} else {
			out[i] = b
		}
	}
	return out, nil
}

func TestDecoderAppliesFilters(t *testing.T) {
	dict := raw.Dict()
	dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("Upper"))
	stream := raw.NewStream(dict, []byte("hello"))

	rawDoc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}: stream,
		},
	}

	pipeline := filters.NewPipeline([]filters.Decoder{uppercaseDecoder{}}, filters.Limits{})
	dec := NewDecoder(pipeline)

	doc, err := dec.Decode(context.Background(), rawDoc)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	got := string(doc.Streams[raw.ObjectRef{Num: 1, Gen: 0}].Data())
	if got != "HELLO" {
		t.Fatalf("expected HELLO, got %s", got)
	}
}

func TestDecoderParallel(t *testing.T) {
	rawDoc := &raw.Document{
		Objects: make(map[raw.ObjectRef]raw.Object),
	}
	count := 100
	for i := 1; i <= count; i++ {
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("Upper"))
		stream := raw.NewStream(dict, []byte("hello"))
		rawDoc.Objects[raw.ObjectRef{Num: i, Gen: 0}] = stream
	}

	pipeline := filters.NewPipeline([]filters.Decoder{uppercaseDecoder{}}, filters.Limits{})
	dec := NewDecoder(pipeline)

	doc, err := dec.Decode(context.Background(), rawDoc)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(doc.Streams) != count {
		t.Fatalf("expected %d streams, got %d", count, len(doc.Streams))
	}
	for i := 1; i <= count; i++ {
		got := string(doc.Streams[raw.ObjectRef{Num: i, Gen: 0}].Data())
		if got != "HELLO" {
			t.Errorf("stream %d: expected HELLO, got %s", i, got)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	rawDoc := &raw.Document{
		Objects: make(map[raw.ObjectRef]raw.Object),
	}
	count := 1000
	payload := make([]byte, 10240) // 10KB
	for i := 0; i < len(payload); i++ {
		payload[i] = 'a'
	}

	for i := 1; i <= count; i++ {
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("Upper"))
		stream := raw.NewStream(dict, payload)
		rawDoc.Objects[raw.ObjectRef{Num: i, Gen: 0}] = stream
	}

	pipeline := filters.NewPipeline([]filters.Decoder{uppercaseDecoder{}}, filters.Limits{})
	dec := NewDecoder(pipeline)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := dec.Decode(ctx, rawDoc)
		if err != nil {
			b.Fatalf("decode failed: %v", err)
		}
	}
}
