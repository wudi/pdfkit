package observability

type Logger interface { Debug(msg string, fields ...Field); Info(msg string, fields ...Field); Warn(msg string, fields ...Field); Error(msg string, fields ...Field); With(fields ...Field) Logger }

type Field interface { Key() string; Value() interface{} }

type stringField struct{ key, val string }; func (f stringField) Key() string { return f.key }; func (f stringField) Value() interface{} { return f.val }

type intField struct{ key string; val int }; func (f intField) Key() string { return f.key }; func (f intField) Value() interface{} { return f.val }

type int64Field struct{ key string; val int64 }; func (f int64Field) Key() string { return f.key }; func (f int64Field) Value() interface{} { return f.val }

type errorField struct{ key string; err error }; func (f errorField) Key() string { return f.key }; func (f errorField) Value() interface{} { return f.err }

func String(key, value string) Field { return stringField{key, value} }
func Int(key string, value int) Field { return intField{key, value} }
func Int64(key string, value int64) Field { return int64Field{key, value} }
func Error(key string, err error) Field { return errorField{key, err} }

type NopLogger struct{}
func (NopLogger) Debug(string, ...Field) {}
func (NopLogger) Info(string, ...Field) {}
func (NopLogger) Warn(string, ...Field) {}
func (NopLogger) Error(string, ...Field) {}
func (NopLogger) With(...Field) Logger { return NopLogger{} }
