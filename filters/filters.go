package filters

import (
	"bytes"
	"compress/flate"
	"context"
	stdascii85 "encoding/ascii85"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"pdflib/ir/raw"
)

type Decoder interface {
	Name() string
	Decode(ctx context.Context, input []byte, params raw.Dictionary) ([]byte, error)
}

type Pipeline struct {
	decoders []Decoder
	limits   Limits
}

// NewPipeline constructs a pipeline with provided decoders and limits.
func NewPipeline(decoders []Decoder, limits Limits) *Pipeline {
	return &Pipeline{decoders: decoders, limits: limits}
}

type Limits struct {
	MaxDecompressedSize int64
	MaxDecodeTime       time.Duration
}

func (p *Pipeline) findDecoder(name string) Decoder {
	for _, d := range p.decoders {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

func (p *Pipeline) Decode(ctx context.Context, input []byte, filterNames []string, params []raw.Dictionary) ([]byte, error) {
	data := input
	for i, name := range filterNames {
		dec := p.findDecoder(name)
		if dec == nil {
			return nil, errors.New("unknown filter: " + name)
		}
		if p.limits.MaxDecompressedSize > 0 && int64(len(data)) > p.limits.MaxDecompressedSize {
			return nil, errors.New("decompressed size exceeds limit")
		}
		var param raw.Dictionary
		if i < len(params) {
			param = params[i]
		}
		out, err := dec.Decode(ctx, data, param)
		if err != nil {
			return nil, err
		}
		data = out
	}
	return data, nil
}

type Registry struct{ decoders map[string]Decoder }

func (r *Registry) Register(d Decoder) {
	if r.decoders == nil {
		r.decoders = make(map[string]Decoder)
	}
	r.decoders[d.Name()] = d
}
func (r *Registry) Get(name string) (Decoder, bool) { d, ok := r.decoders[name]; return d, ok }

// Stub decoders
type flateDecoder struct{}

func (flateDecoder) Name() string { return "FlateDecode" }
func NewFlateDecoder() Decoder    { return flateDecoder{} }

type lzwDecoder struct{}

func (lzwDecoder) Name() string { return "LZWDecode" }
func (lzwDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	return in, nil
}
func NewLZWDecoder() Decoder { return lzwDecoder{} }

type ascii85Decoder struct{}

func (ascii85Decoder) Name() string { return "ASCII85Decode" }
func (ascii85Decoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	trimmed := bytes.TrimSpace(in)
	if bytes.HasPrefix(trimmed, []byte("<~")) && bytes.HasSuffix(trimmed, []byte("~>")) {
		trimmed = trimmed[2 : len(trimmed)-2]
	}
	out := make([]byte, len(trimmed)*2)
	n, _, err := stdascii85.Decode(out, trimmed, true)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}
func NewASCII85Decoder() Decoder { return ascii85Decoder{} }

type asciiHexDecoder struct{}

func (asciiHexDecoder) Name() string { return "ASCIIHexDecode" }
func (asciiHexDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	trimmed := bytes.TrimSpace(in)
	if i := bytes.IndexByte(trimmed, '>'); i >= 0 {
		trimmed = trimmed[:i]
	}
	// if odd length, pad with 0 per spec
	if len(trimmed)%2 == 1 {
		trimmed = append(trimmed, '0')
	}
	result := make([]byte, hex.DecodedLen(len(trimmed)))
	n, err := hex.Decode(result, trimmed)
	if err != nil {
		return nil, err
	}
	return result[:n], nil
}
func NewASCIIHexDecoder() Decoder { return asciiHexDecoder{} }

// Flate, LZW left intentionally minimal; ASCII decoders above, Flate below.

// flateDecoder implements FlateDecode using the standard library.
func (flateDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(in))
	defer r.Close()

	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
