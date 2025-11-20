package semantic

import "pdflib/ir/raw"

// AcroForm represents form-level information.
type AcroForm struct {
	NeedAppearances bool
	XFA             []byte // XML Data Stream
	Fields          []FormField
	DefaultResources *Resources // DR entry
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

// FormField is the interface for all form fields.
type FormField interface {
	FieldType() string
	FieldName() string
	FieldFlags() int
	FieldRect() Rectangle
	FieldPageIndex() int
	SetFieldRect(Rectangle)
	SetFieldPageIndex(int)
	SetFieldFlags(int)

	// Common accessors for serialization
	GetAppearance() []byte
	GetAppearanceState() string
	GetBorder() []float64
	GetColor() []float64

	// Reference management
	Reference() raw.ObjectRef
	SetReference(raw.ObjectRef)
	IsDirty() bool
	SetDirty(bool)
}

// BaseFormField provides common fields for form fields.
type BaseFormField struct {
	Name            string
	PageIndex       int
	Rect            Rectangle
	Flags           int
	Appearance      []byte
	AppearanceState string
	Border          []float64
	Color           []float64
	DefaultAppearance string // DA entry
	Quadding          int    // Q entry: 0=Left, 1=Center, 2=Right
	Ref             raw.ObjectRef
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

func (f *BaseFormField) FieldName() string            { return f.Name }
func (f *BaseFormField) FieldPageIndex() int          { return f.PageIndex }
func (f *BaseFormField) FieldRect() Rectangle         { return f.Rect }
func (f *BaseFormField) FieldFlags() int              { return f.Flags }
func (f *BaseFormField) SetFieldRect(r Rectangle)     { f.Rect = r }
func (f *BaseFormField) SetFieldPageIndex(i int)      { f.PageIndex = i }
func (f *BaseFormField) SetFieldFlags(flags int)      { f.Flags = flags }
func (f *BaseFormField) GetAppearance() []byte        { return f.Appearance }
func (f *BaseFormField) GetAppearanceState() string   { return f.AppearanceState }
func (f *BaseFormField) GetBorder() []float64         { return f.Border }
func (f *BaseFormField) GetColor() []float64          { return f.Color }
func (f *BaseFormField) GetDefaultAppearance() string { return f.DefaultAppearance }
func (f *BaseFormField) GetQuadding() int             { return f.Quadding }
func (f *BaseFormField) Reference() raw.ObjectRef     { return f.Ref }
func (f *BaseFormField) SetReference(r raw.ObjectRef) { f.Ref = r }
func (f *BaseFormField) IsDirty() bool                { return f.Dirty }
func (f *BaseFormField) SetDirty(d bool)              { f.Dirty = d }

// TextFormField represents a text field (Tx).
type TextFormField struct {
	BaseFormField
	Value  string
	MaxLen int
}

func (f *TextFormField) FieldType() string { return "Tx" }

// ChoiceFormField represents a choice field (Ch) - Combo box or List box.
type ChoiceFormField struct {
	BaseFormField
	Options       []string
	Selected      []string
	IsCombo       bool
	IsMultiSelect bool
}

func (f *ChoiceFormField) FieldType() string { return "Ch" }

// ButtonFormField represents a button field (Btn) - Push, Check, Radio.
type ButtonFormField struct {
	BaseFormField
	IsPush  bool
	IsRadio bool
	IsCheck bool
	Checked bool   // For check/radio
	OnState string // The name of the "on" state (e.g., "Yes")
}

func (f *ButtonFormField) FieldType() string { return "Btn" }

// SignatureFormField represents a signature field (Sig).
type SignatureFormField struct {
	BaseFormField
	Signature *Signature // To be implemented in Phase 3
}

func (f *SignatureFormField) FieldType() string { return "Sig" }

// GenericFormField for unknown types or simple generic usage
type GenericFormField struct {
	BaseFormField
	Type  string
	Value string
}

func (f *GenericFormField) FieldType() string { return f.Type }
