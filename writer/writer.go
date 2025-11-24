package writer

import (
	"context"

	"github.com/wudi/pdfkit/compliance/pdfa"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/optimize"
)

type PDFVersion string

const (
	PDF17 PDFVersion = "1.7"
)

// PDF/A level aliases point to the shared pdfa.Level type.
type PDFALevel = pdfa.Level

const (
	PDFA1B = pdfa.PDFA1B
	PDFA3B = pdfa.PDFA3B
)

type ContentFilter int

const (
	FilterNone ContentFilter = iota
	FilterFlate
	FilterASCIIHex
	FilterASCII85
	FilterRunLength
	FilterLZW
	FilterJPX
	FilterJBIG2
)

type Config struct {
	Version       PDFVersion
	Compression   int
	ContentFilter ContentFilter
	Linearize     bool
	Incremental   bool
	Deterministic bool
	XRefStreams   bool
	ObjectStreams bool
	SubsetFonts   bool
	PDFALevel     pdfa.Level
	Optimizer     *optimize.Optimizer
}

type Writer interface {
	Write(ctx context.Context, doc *semantic.Document, w WriterAt, cfg Config) error
	SerializeObject(ref raw.ObjectRef, obj raw.Object) ([]byte, error)
}

// NewWriter creates a new Writer with default configuration.
func NewWriter() Writer {
	return (&WriterBuilder{}).Build()
}

type Interceptor interface {
	BeforeWrite(ctx context.Context, obj raw.Object) error
	AfterWrite(ctx context.Context, obj raw.Object, bytesWritten int64) error
}

type WriterBuilder struct {
	interceptors     []Interceptor
	annotSerializer  AnnotationSerializer
	actionSerializer ActionSerializer
	csSerializer     ColorSpaceSerializer
	funcSerializer   FunctionSerializer
}

func (b *WriterBuilder) WithInterceptor(i Interceptor) *WriterBuilder {
	b.interceptors = append(b.interceptors, i)
	return b
}

func (b *WriterBuilder) WithAnnotationSerializer(s AnnotationSerializer) *WriterBuilder {
	b.annotSerializer = s
	return b
}

func (b *WriterBuilder) WithActionSerializer(s ActionSerializer) *WriterBuilder {
	b.actionSerializer = s
	return b
}

func (b *WriterBuilder) WithColorSpaceSerializer(s ColorSpaceSerializer) *WriterBuilder {
	b.csSerializer = s
	return b
}

func (b *WriterBuilder) WithFunctionSerializer(s FunctionSerializer) *WriterBuilder {
	b.funcSerializer = s
	return b
}

func (b *WriterBuilder) Build() Writer {
	return &impl{
		interceptors:     b.interceptors,
		annotSerializer:  b.annotSerializer,
		actionSerializer: b.actionSerializer,
		csSerializer:     b.csSerializer,
		funcSerializer:   b.funcSerializer,
	}
}

type WriterAt interface {
	Write(p []byte) (n int, err error)
}

type Context interface{ Done() <-chan struct{} }
