package observability

import "context"

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

type Field interface {
	Key() string
	Value() interface{}
}

type stringField struct{ key, val string }

func (f stringField) Key() string        { return f.key }
func (f stringField) Value() interface{} { return f.val }

type intField struct {
	key string
	val int
}

func (f intField) Key() string        { return f.key }
func (f intField) Value() interface{} { return f.val }

type int64Field struct {
	key string
	val int64
}

func (f int64Field) Key() string        { return f.key }
func (f int64Field) Value() interface{} { return f.val }

type errorField struct {
	key string
	err error
}

func (f errorField) Key() string        { return f.key }
func (f errorField) Value() interface{} { return f.err }

func String(key, value string) Field      { return stringField{key, value} }
func Int(key string, value int) Field     { return intField{key, value} }
func Int64(key string, value int64) Field { return int64Field{key, value} }
func Error(key string, err error) Field   { return errorField{key, err} }

type NopLogger struct{}

func (NopLogger) Debug(string, ...Field) {}
func (NopLogger) Info(string, ...Field)  {}
func (NopLogger) Warn(string, ...Field)  {}
func (NopLogger) Error(string, ...Field) {}
func (NopLogger) With(...Field) Logger   { return NopLogger{} }

// Tracer provides distributed tracing hooks for library operations.
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// Span represents a tracing span.
type Span interface {
	SetTag(key string, value interface{})
	SetError(err error)
	Finish()
}

type nopTracer struct{}

func (nopTracer) StartSpan(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, nopSpan{}
}

// NopTracer returns a tracer that does nothing.
func NopTracer() Tracer { return nopTracer{} }

type nopSpan struct{}

func (nopSpan) SetTag(string, interface{}) {}
func (nopSpan) SetError(error)             {}
func (nopSpan) Finish()                    {}

// Standard metric names emitted by the library.
const (
	MetricParseTime      = "pdf.parse.duration"
	MetricObjectCount    = "pdf.objects.count"
	MetricPageCount      = "pdf.pages.count"
	MetricDecodedBytes   = "pdf.decoded.bytes"
	MetricFilterTime     = "pdf.filter.duration"
	MetricWriteTime      = "pdf.write.duration"
	MetricSubsettingTime = "pdf.subsetting.duration"
)
