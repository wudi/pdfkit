package filters

import (
	"bytes"
	"compress/flate"
	"context"
	stdascii85 "encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
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
	earlyChange := int64(1) // default
	if params != nil {
		if v, ok := params.Get(raw.NameObj{Val: "EarlyChange"}); ok {
			if n, ok := v.(raw.NumberObj); ok {
				earlyChange = n.Int()
			}
		}
	}
	out, err := lzwDecompress(in, earlyChange != 0)
	if err != nil {
		return nil, err
	}
	return applyPredictor(out, params)
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
	return applyPredictor(out.Bytes(), params)
}

// lzwDecompress implements PDF LZW (MSB, 9-12 bits) with optional early change.
func lzwDecompress(src []byte, earlyChange bool) ([]byte, error) {
	const (
		clearCode = 256
		eodCode   = 257
		maxBits   = 12
	)
	type entry []byte

	dict := make([]entry, 4096)
	resetDict := func() {
		for i := 0; i < 256; i++ {
			dict[i] = entry{byte(i)}
		}
	}
	resetDict()
	bits := 9
	nextCode := 258
	br := newBitReader(src)
	var out bytes.Buffer

	var prev entry
	for {
		code, err := br.readBits(bits)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch code {
		case clearCode:
			resetDict()
			bits = 9
			nextCode = 258
			prev = nil
			continue
		case eodCode:
			return out.Bytes(), nil
		}
		var cur entry
		if int(code) < len(dict) && dict[code] != nil {
			cur = dict[code]
		} else if code == nextCode && prev != nil {
			cur = append(entry(nil), prev...)
			cur = append(cur, prev[0])
		} else {
			return nil, fmt.Errorf("invalid LZW code %d", code)
		}
		out.Write(cur)
		if prev != nil {
			if nextCode < len(dict) {
				dict[nextCode] = append(entry(nil), append(prev, cur[0])...)
				nextCode++
				threshold := 1 << bits
				if earlyChange {
					threshold--
				}
				if nextCode > threshold && bits < maxBits {
					bits++
				}
			}
		}
		prev = cur
	}
	return out.Bytes(), nil
}

type bitReader struct {
	data []byte
	pos  int
	bits uint64
	nb   int
}

func newBitReader(data []byte) *bitReader { return &bitReader{data: data} }

func (r *bitReader) readBits(n int) (int, error) {
	for r.nb < n {
		if r.pos >= len(r.data) {
			return 0, io.EOF
		}
		r.bits = (r.bits << 8) | uint64(r.data[r.pos])
		r.pos++
		r.nb += 8
	}
	shift := r.nb - n
	val := int((r.bits >> shift) & (uint64(1)<<n - 1))
	r.nb -= n
	r.bits &= (uint64(1) << r.nb) - 1
	return val, nil
}

// applyPredictor handles TIFF/PNG predictors (PDF 7.4.4) after decompression.
func applyPredictor(data []byte, params raw.Dictionary) ([]byte, error) {
	if params == nil {
		return data, nil
	}
	pred := int64(1)
	colors := int64(1)
	bpc := int64(8)
	cols := int64(1)

	if v, ok := params.Get(raw.NameObj{Val: "Predictor"}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			pred = n.Int()
		}
	}
	if pred <= 1 {
		return data, nil
	}
	if v, ok := params.Get(raw.NameObj{Val: "Colors"}); ok {
		if n, ok := v.(raw.NumberObj); ok && n.Int() > 0 {
			colors = n.Int()
		}
	}
	if v, ok := params.Get(raw.NameObj{Val: "BitsPerComponent"}); ok {
		if n, ok := v.(raw.NumberObj); ok && n.Int() > 0 {
			bpc = n.Int()
		}
	}
	if v, ok := params.Get(raw.NameObj{Val: "Columns"}); ok {
		if n, ok := v.(raw.NumberObj); ok && n.Int() > 0 {
			cols = n.Int()
		}
	}
	if bpc%8 != 0 {
		return nil, errors.New("predictor with non-8-bit components not supported")
	}
	bytesPerPixel := int(colors * (bpc / 8))
	rowBytes := int(math.Ceil(float64(cols*colors*bpc) / 8.0))
	if rowBytes <= 0 {
		return nil, errors.New("invalid predictor row size")
	}

	switch pred {
	case 2: // TIFF (no per-row filter byte)
		out := make([]byte, len(data))
		copy(out, data)
		for i := rowBytes; i < len(out); i++ {
			out[i] = byte(int(out[i]+out[i-bytesPerPixel]) & 0xFF)
		}
		return out, nil
	case 10, 11, 12, 13, 14, 15: // PNG predictors
		rowLen := rowBytes + 1
		if len(data)%rowLen != 0 {
			return nil, errors.New("predictor data does not align to rows")
		}
		rows := len(data) / rowLen
		out := make([]byte, rows*rowBytes)
		var prev []byte
		for r := 0; r < rows; r++ {
			filter := data[r*rowLen]
			rowData := data[r*rowLen+1 : (r+1)*rowLen]
			dst := out[r*rowBytes : (r+1)*rowBytes]
			switch filter {
			case 0: // None
				copy(dst, rowData)
			case 1: // Sub
				for i := 0; i < rowBytes; i++ {
					left := byte(0)
					if i >= bytesPerPixel {
						left = dst[i-bytesPerPixel]
					}
					dst[i] = byte(int(rowData[i]+left) & 0xFF)
				}
			case 2: // Up
				for i := 0; i < rowBytes; i++ {
					up := byte(0)
					if prev != nil {
						up = prev[i]
					}
					dst[i] = byte(int(rowData[i]+up) & 0xFF)
				}
			case 3: // Average
				for i := 0; i < rowBytes; i++ {
					left := byte(0)
					if i >= bytesPerPixel {
						left = dst[i-bytesPerPixel]
					}
					up := byte(0)
					if prev != nil {
						up = prev[i]
					}
					dst[i] = byte(int(rowData[i]+byte((int(left)+int(up))/2)) & 0xFF)
				}
			case 4: // Paeth
				for i := 0; i < rowBytes; i++ {
					left := byte(0)
					up := byte(0)
					upLeft := byte(0)
					if i >= bytesPerPixel {
						left = dst[i-bytesPerPixel]
						if prev != nil {
							upLeft = prev[i-bytesPerPixel]
						}
					}
					if prev != nil {
						up = prev[i]
					}
					dst[i] = byte(int(rowData[i]+paeth(left, up, upLeft)) & 0xFF)
				}
			default:
				return nil, fmt.Errorf("unknown PNG predictor %d", filter)
			}
			prev = dst
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported predictor %d", pred)
	}
}

func paeth(a, b, c byte) byte {
	pa := int(a)
	pb := int(b)
	pc := int(c)
	p := pa + pb - pc
	da := abs(p - pa)
	db := abs(p - pb)
	dc := abs(p - pc)
	switch {
	case da <= db && da <= dc:
		return a
	case db <= dc:
		return b
	default:
		return c
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
