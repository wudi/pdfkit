package raw

// Concrete implementations for raw objects (minimal subset).

type baseObj struct{}

func (baseObj) IsIndirect() bool { return false }

// Name object
type NameObj struct{ Val string }

func (n NameObj) Type() string     { return "name" }
func (n NameObj) IsIndirect() bool { return false }
func (n NameObj) Value() string    { return n.Val }

// Number object
type NumberObj struct {
	I     int64
	F     float64
	IsInt bool
}

func (n NumberObj) Type() string     { return "number" }
func (n NumberObj) IsIndirect() bool { return false }
func (n NumberObj) Int() int64       { return n.I }
func (n NumberObj) Float() float64 {
	if n.IsInt {
		return float64(n.I)
	}
	return n.F
}
func (n NumberObj) IsInteger() bool { return n.IsInt }

// Boolean object
type BoolObj struct{ V bool }

func (b BoolObj) Type() string     { return "boolean" }
func (b BoolObj) IsIndirect() bool { return false }
func (b BoolObj) Value() bool      { return b.V }

// Null object
type NullObj struct{}

func (n NullObj) Type() string     { return "null" }
func (n NullObj) IsIndirect() bool { return false }

// String object (literal only)
type StringObj struct{ Bytes []byte }

func (s StringObj) Type() string     { return "string" }
func (s StringObj) IsIndirect() bool { return false }
func (s StringObj) Value() []byte    { return s.Bytes }
func (s StringObj) IsHex() bool      { return false }

// Array object
type ArrayObj struct{ Items []Object }

func (a *ArrayObj) Type() string     { return "array" }
func (a *ArrayObj) IsIndirect() bool { return false }
func (a *ArrayObj) Get(i int) (Object, bool) {
	if i < 0 || i >= len(a.Items) {
		return nil, false
	}
	return a.Items[i], true
}
func (a *ArrayObj) Len() int        { return len(a.Items) }
func (a *ArrayObj) Append(o Object) { a.Items = append(a.Items, o) }

// Dictionary object
type DictObj struct{ KV map[string]Object }

func (d *DictObj) Type() string                { return "dict" }
func (d *DictObj) IsIndirect() bool            { return false }
func (d *DictObj) Get(key Name) (Object, bool) { o, ok := d.KV[key.Value()]; return o, ok }
func (d *DictObj) Set(key Name, value Object) {
	if d.KV == nil {
		d.KV = make(map[string]Object)
	}
	d.KV[key.Value()] = value
}
func (d *DictObj) Keys() []Name {
	keys := make([]Name, 0, len(d.KV))
	for k := range d.KV {
		keys = append(keys, NameObj{Val: k})
	}
	return keys
}
func (d *DictObj) Len() int { return len(d.KV) }

// Stream object
type StreamObj struct {
	Dict *DictObj
	Data []byte
}

func (s *StreamObj) Type() string           { return "stream" }
func (s *StreamObj) IsIndirect() bool       { return false }
func (s *StreamObj) Dictionary() Dictionary { return s.Dict }
func (s *StreamObj) RawData() []byte        { return s.Data }
func (s *StreamObj) Length() int64          { return int64(len(s.Data)) }

// Reference object
type RefObj struct{ R ObjectRef }

func (r RefObj) Type() string     { return "ref" }
func (r RefObj) IsIndirect() bool { return true }
func (r RefObj) Ref() ObjectRef   { return r.R }

// Helpers
func NameLiteral(v string) NameObj                 { return NameObj{Val: v} }
func NumberInt(i int64) NumberObj                  { return NumberObj{I: i, IsInt: true} }
func NumberFloat(f float64) NumberObj              { return NumberObj{F: f, IsInt: false} }
func Bool(v bool) BoolObj                          { return BoolObj{V: v} }
func Str(bytes []byte) StringObj                   { return StringObj{Bytes: bytes} }
func NewArray(items ...Object) *ArrayObj           { return &ArrayObj{Items: items} }
func Dict() *DictObj                               { return &DictObj{KV: make(map[string]Object)} }
func NewStream(dict *DictObj, data []byte) *StreamObj { return &StreamObj{Dict: dict, Data: data} }
func Ref(num, gen int) RefObj                      { return RefObj{R: ObjectRef{Num: num, Gen: gen}} }
