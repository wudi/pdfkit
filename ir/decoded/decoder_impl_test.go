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
