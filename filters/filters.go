package filters

import (
	"bytes"
	"compress/flate"
	"context"
	stdascii85 "encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"time"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/ccitt"
	_ "golang.org/x/image/webp"

	"pdflib/ir/raw"
)

type Decoder interface {
	Name() string
	Decode(ctx context.Context, input []byte, params raw.Dictionary) ([]byte, error)
}

// UnsupportedError reports a filter that is recognized but not implemented.
type UnsupportedError struct{ Filter string }

func (e UnsupportedError) Error() string { return fmt.Sprintf("%s filter not supported", e.Filter) }

type Pipeline struct {
	decoders []Decoder
	limits   Limits
}

type jbig2GlobalsResolverKey struct{}

// StreamResolver resolves referenced streams to decoded bytes for filters that
// need auxiliary data (e.g., JBIG2Globals).
type StreamResolver func(context.Context, raw.ObjectRef) ([]byte, error)

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
	return p.decode(ctx, input, filterNames, params, nil)
}

// DecodeWithResolver decodes a stream using the provided filters and optional resolver.
func (p *Pipeline) DecodeWithResolver(ctx context.Context, input []byte, filterNames []string, params []raw.Dictionary, resolver StreamResolver) ([]byte, error) {
	return p.decode(ctx, input, filterNames, params, resolver)
}

func (p *Pipeline) decode(ctx context.Context, input []byte, filterNames []string, params []raw.Dictionary, resolver StreamResolver) ([]byte, error) {
	data := input
	baseCtx := ctx
	if resolver != nil {
		baseCtx = context.WithValue(ctx, jbig2GlobalsResolverKey{}, resolver)
	}
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
		decodeCtx := baseCtx
		var cancel context.CancelFunc
		if p.limits.MaxDecodeTime > 0 {
			decodeCtx, cancel = context.WithTimeout(baseCtx, p.limits.MaxDecodeTime)
		}
		out, err := dec.Decode(decodeCtx, data, param)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			return nil, err
		}
		data = out
	}
	return data, nil
}

func streamResolverFromContext(ctx context.Context) StreamResolver {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(jbig2GlobalsResolverKey{}); v != nil {
		if resolver, ok := v.(StreamResolver); ok {
			return resolver
		}
	}
	return nil
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

type runLengthDecoder struct{}

func (runLengthDecoder) Name() string { return "RunLengthDecode" }
func (runLengthDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	var out bytes.Buffer
	for i := 0; i < len(in); {
		b := in[i]
		if b == 128 { // EOD marker
			break
		}
		if i+1 >= len(in) {
			return nil, errors.New("runlength truncated")
		}
		i++
		if b <= 127 {
			// length is (n) => copy n+1 literal bytes
			lit := int(b) + 1
			if i+lit > len(in) {
				return nil, errors.New("runlength literal overrun")
			}
			out.Write(in[i : i+lit])
			i += lit
		} else {
			// replicate next byte (257 - length) times
			val := in[i]
			i++
			count := 257 - int(b)
			if count < 0 {
				return nil, errors.New("runlength invalid count")
			}
			for j := 0; j < count; j++ {
				out.WriteByte(val)
			}
		}
	}
	return applyPredictor(out.Bytes(), params)
}
func NewRunLengthDecoder() Decoder { return runLengthDecoder{} }

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

type cryptDecoder struct{}

func (cryptDecoder) Name() string { return "Crypt" }
func (cryptDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	// Crypt filter is applied via the security handler; decoding stage treats it as transparent.
	return in, nil
}
func NewCryptDecoder() Decoder { return cryptDecoder{} }

type dctDecoder struct{}

func (dctDecoder) Name() string { return "DCTDecode" }
func (dctDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	img, err := jpeg.Decode(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	// Normalize to NRGBA pixel data.
	rgba := image.NewNRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba.Pix, nil
}
func NewDCTDecoder() Decoder { return dctDecoder{} }

type jpxDecoder struct{}

func (jpxDecoder) Name() string { return "JPXDecode" }
func (jpxDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if pix, err := decodeImageToNRGBA(in); err == nil {
		return pix, nil
	}
	if pix, err := decodeJPXExternal(ctx, in); err == nil {
		return pix, nil
	}
	return nil, UnsupportedError{Filter: "JPXDecode"}
}
func NewJPXDecoder() Decoder { return jpxDecoder{} }

type ccittFaxDecoder struct{}

func (ccittFaxDecoder) Name() string { return "CCITTFaxDecode" }
func (ccittFaxDecoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if params == nil {
		return nil, errors.New("CCITT params required")
	}
	width := int64(0)
	height := int64(ccitt.AutoDetectHeight)
	if v, ok := params.Get(raw.NameObj{Val: "Columns"}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			width = n.Int()
		}
	}
	if width <= 0 {
		return nil, errors.New("CCITT Columns must be >0")
	}
	if v, ok := params.Get(raw.NameObj{Val: "Rows"}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			height = n.Int()
		}
	}
	k := int64(0)
	if v, ok := params.Get(raw.NameObj{Val: "K"}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			k = n.Int()
		}
	}
	subFmt := ccitt.Group3
	if k < 0 {
		subFmt = ccitt.Group4
	}
	opts := &ccitt.Options{}
	if v, ok := params.Get(raw.NameObj{Val: "EncodedByteAlign"}); ok {
		if b, ok := v.(raw.BoolObj); ok && b.Value() {
			opts.Align = true
		}
	}
	if v, ok := params.Get(raw.NameObj{Val: "BlackIs1"}); ok {
		if b, ok := v.(raw.BoolObj); ok && b.Value() {
			opts.Invert = true
		}
	}
	gray := image.NewGray(image.Rect(0, 0, int(width), int(height)))
	if err := ccitt.DecodeIntoGray(gray, bytes.NewReader(in), ccitt.MSB, subFmt, opts); err != nil {
		return nil, err
	}
	return gray.Pix, nil
}
func NewCCITTFaxDecoder() Decoder { return ccittFaxDecoder{} }

type jbig2Decoder struct{}

func (jbig2Decoder) Name() string { return "JBIG2Decode" }
func (jbig2Decoder) Decode(ctx context.Context, in []byte, params raw.Dictionary) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	globals, err := resolveJBIG2Globals(ctx, params)
	if err != nil {
		return nil, err
	}
	native, nativeErr := jbig2NativeDecode(ctx, in, globals)
	if nativeErr == nil {
		return native, nil
	}
	if pix, err := decodeImageToNRGBA(in); err == nil {
		return pix, nil
	}
	if pix, err := decodeJBIG2External(ctx, in); err == nil {
		return pix, nil
	}
	if nativeErr != nil && !errors.Is(nativeErr, errJBIG2NativeUnsupported) {
		return nil, nativeErr
	}
	return nil, UnsupportedError{Filter: "JBIG2Decode"}
}
func NewJBIG2Decoder() Decoder { return jbig2Decoder{} }

func resolveJBIG2Globals(ctx context.Context, params raw.Dictionary) ([]byte, error) {
	if params == nil {
		return nil, nil
	}
	globalsObj, ok := params.Get(raw.NameObj{Val: "JBIG2Globals"})
	if !ok {
		return nil, nil
	}
	switch v := globalsObj.(type) {
	case raw.Stream:
		return v.RawData(), nil
	case raw.Reference:
		resolver := streamResolverFromContext(ctx)
		if resolver == nil {
			return nil, errors.New("JBIG2Globals resolver unavailable")
		}
		return resolver(ctx, v.Ref())
	default:
		return nil, fmt.Errorf("unsupported JBIG2Globals type %T", globalsObj)
	}
}

// decodeImageToNRGBA attempts to decode arbitrary encoded image bytes into NRGBA pixels.
func decodeImageToNRGBA(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	rgba := image.NewNRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba.Pix, nil
}

// decodeJPXExternal calls opj_decompress (OpenJPEG) when available to decode JPX streams.
func decodeJPXExternal(ctx context.Context, data []byte) ([]byte, error) {
	tool, err := exec.LookPath("opj_decompress")
	if err != nil {
		return nil, err
	}
	input, err := os.CreateTemp("", "pdflib-jpx-*.jpx")
	if err != nil {
		return nil, err
	}
	defer os.Remove(input.Name())
	if _, err := input.Write(data); err != nil {
		input.Close()
		return nil, err
	}
	input.Close()

	output, err := os.CreateTemp("", "pdflib-jpx-out-*.png")
	if err != nil {
		return nil, err
	}
	outName := output.Name()
	output.Close()
	defer os.Remove(outName)

	cmd := exec.CommandContext(ctx, tool, "-i", input.Name(), "-o", outName)
	if runErr := cmd.Run(); runErr != nil {
		return nil, runErr
	}
	outBytes, err := os.ReadFile(outName)
	if err != nil {
		return nil, err
	}
	return decodeImageToNRGBA(outBytes)
}

// decodeJBIG2External calls jbig2dec when available to decode JBIG2 streams.
func decodeJBIG2External(ctx context.Context, data []byte) ([]byte, error) {
	tool, err := exec.LookPath("jbig2dec")
	if err != nil {
		return nil, err
	}
	input, err := os.CreateTemp("", "pdflib-jb2-*.jb2")
	if err != nil {
		return nil, err
	}
	defer os.Remove(input.Name())
	if _, err := input.Write(data); err != nil {
		input.Close()
		return nil, err
	}
	input.Close()

	output, err := os.CreateTemp("", "pdflib-jb2-out-*.png")
	if err != nil {
		return nil, err
	}
	outName := output.Name()
	output.Close()
	defer os.Remove(outName)

	cmd := exec.CommandContext(ctx, tool, "-o", outName, input.Name())
	if runErr := cmd.Run(); runErr != nil {
		return nil, runErr
	}
	outBytes, err := os.ReadFile(outName)
	if err != nil {
		return nil, err
	}
	return decodeImageToNRGBA(outBytes)
}

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
				if nextCode >= threshold && bits < maxBits {
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
