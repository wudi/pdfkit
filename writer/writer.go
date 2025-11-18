package writer

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
)

type PDFVersion string

const (
	PDF17 PDFVersion = "1.7"
)

type PDFALevel int

const (
	PDFA1B PDFALevel = iota
)

type ContentFilter int

const (
	FilterNone ContentFilter = iota
	FilterFlate
	FilterASCIIHex
	FilterASCII85
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
	PDFALevel     PDFALevel
}

type Writer interface {
	Write(ctx Context, doc *semantic.Document, w WriterAt, cfg Config) error
	SerializeObject(ref raw.ObjectRef, obj raw.Object) ([]byte, error)
}

type Interceptor interface {
	BeforeWrite(ctx Context, obj raw.Object) error
	AfterWrite(ctx Context, obj raw.Object, bytesWritten int64) error
}

type WriterBuilder struct{ interceptors []Interceptor }

func (b *WriterBuilder) WithInterceptor(i Interceptor) *WriterBuilder {
	b.interceptors = append(b.interceptors, i)
	return b
}
func (b *WriterBuilder) Build() Writer { return &impl{interceptors: b.interceptors} }

type WriterAt interface {
	Write(p []byte) (n int, err error)
}

type Context interface{ Done() <-chan struct{} }
