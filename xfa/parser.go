package xfa

import (
	"encoding/xml"
	"io"
)

// Parser parses XFA XML streams into an XFA DOM.
type Parser interface {
	Parse(r io.Reader) (*Form, error)
}

type ParserImpl struct{}

func NewParser() *ParserImpl {
	return &ParserImpl{}
}

func (p *ParserImpl) Parse(r io.Reader) (*Form, error) {
	var form Form
	decoder := xml.NewDecoder(r)
	if err := decoder.Decode(&form); err != nil {
		return nil, err
	}
	return &form, nil
}

// Form represents the root of the XFA DOM.
// Note: This is a simplified representation. XFA is huge.
type Form struct {
	XMLName       xml.Name       `xml:"xdp:xdp"`
	Config        *Config        `xml:"config"`
	Template      *Template      `xml:"template"`
	Datasets      *Datasets      `xml:"datasets"`
	LocaleSet     *LocaleSet     `xml:"localeSet"`
	ConnectionSet *ConnectionSet `xml:"connectionSet"`
}

type Config struct {
	Present   *Present   `xml:"present"`
	Acrobat   *Acrobat   `xml:"acrobat"`
	Trace     *Trace     `xml:"trace"`
	Agent     *Agent     `xml:"agent"`
	Log       *Log       `xml:"log"`
	Messaging *Messaging `xml:"messaging"`
}

type Present struct {
	Destination string `xml:"destination"` // e.g. "pdf"
	// ...
}

type Acrobat struct {
	// ...
}

type Trace struct {
	// ...
}

type Agent struct {
	// ...
}

type Log struct {
	// ...
}

type Messaging struct {
	// ...
}

type Template struct {
	Subform *Subform `xml:"subform"`
}

type Subform struct {
	Name     string    `xml:"name,attr"`
	Layout   string    `xml:"layout,attr"` // e.g., "tb" (top-to-bottom)
	W        string    `xml:"w,attr"`
	H        string    `xml:"h,attr"`
	X        string    `xml:"x,attr"`
	Y        string    `xml:"y,attr"`
	Fields   []Field   `xml:"field"`
	Subforms []Subform `xml:"subform"`
	Draws    []Draw    `xml:"draw"`
	Content  []Area    `xml:"area"`
	Bind     *Bind     `xml:"bind"`
	Occur    *Occur    `xml:"occur"`
}

type Field struct {
	Name    string   `xml:"name,attr"`
	W       string   `xml:"w,attr"`
	H       string   `xml:"h,attr"`
	X       string   `xml:"x,attr"`
	Y       string   `xml:"y,attr"`
	UI      *UI      `xml:"ui"`
	Value   *Value   `xml:"value"`
	Caption *Caption `xml:"caption"`
	Bind    *Bind    `xml:"bind"`
	Font    *Font    `xml:"font"`
	Para    *Para    `xml:"para"`
}

type UI struct {
	TextEdit     *TextEdit     `xml:"textEdit"`
	CheckButton  *CheckButton  `xml:"checkButton"`
	ChoiceList   *ChoiceList   `xml:"choiceList"`
	NumericEdit  *NumericEdit  `xml:"numericEdit"`
	DateTimeEdit *DateTimeEdit `xml:"dateTimeEdit"`
	ImageEdit    *ImageEdit    `xml:"imageEdit"`
	Signature    *Signature    `xml:"signature"`
	Button       *Button       `xml:"button"`
}

type TextEdit struct {
	MultiLine string `xml:"multiLine,attr"` // "0" or "1"
}

type CheckButton struct {
	Shape string `xml:"shape,attr"` // "square", "round"
}

type ChoiceList struct {
	Open string `xml:"open,attr"` // "userControl", "always", "multiSelect"
}

type NumericEdit struct{}
type DateTimeEdit struct{}
type ImageEdit struct{}
type Signature struct{}
type Button struct {
	Highlight string `xml:"highlight,attr"` // "inverted", "push", "outline"
}

type Draw struct {
	Name  string `xml:"name,attr"`
	W     string `xml:"w,attr"`
	H     string `xml:"h,attr"`
	X     string `xml:"x,attr"`
	Y     string `xml:"y,attr"`
	Value *Value `xml:"value"`
	Font  *Font  `xml:"font"`
	Para  *Para  `xml:"para"`
}

type Value struct {
	Text     string    `xml:"text"`
	Integer  string    `xml:"integer"`
	Decimal  string    `xml:"decimal"`
	Float    string    `xml:"float"`
	Boolean  string    `xml:"boolean"`
	Date     string    `xml:"date"`
	Time     string    `xml:"time"`
	DateTime string    `xml:"dateTime"`
	Image    *ImageVal `xml:"image"`
	ExData   *ExData   `xml:"exData"`
}

type ImageVal struct {
	Href        string `xml:"href,attr"`
	ContentType string `xml:"contentType,attr"`
	Content     string `xml:",chardata"` // Base64
}

type ExData struct {
	ContentType string `xml:"contentType,attr"`
	Content     string `xml:",chardata"`
}

type Caption struct {
	Value *Value `xml:"value"`
	Font  *Font  `xml:"font"`
	Para  *Para  `xml:"para"`
}

type Font struct {
	Typeface string `xml:"typeface,attr"`
	Size     string `xml:"size,attr"`
	Weight   string `xml:"weight,attr"`  // "bold"
	Posture  string `xml:"posture,attr"` // "italic"
}

type Para struct {
	HAlign string `xml:"hAlign,attr"` // "left", "center", "right", "justify"
	VAlign string `xml:"vAlign,attr"` // "top", "middle", "bottom"
}

type Bind struct {
	Match string `xml:"match,attr"` // "dataRef", "none"
	Ref   string `xml:"ref,attr"`
}

type Occur struct {
	Min string `xml:"min,attr"`
	Max string `xml:"max,attr"`
}

type Area struct {
	Name string `xml:"name,attr"`
	X    string `xml:"x,attr"`
	Y    string `xml:"y,attr"`
	// ...
}

type Datasets struct {
	Data *Data `xml:"data"`
}

type Data struct {
	// XML data structure matching the form
	// This is dynamic, so we might need to use xml.Node or similar if we want to traverse it generically.
	// For now, we leave it as a placeholder or use a generic map/struct if possible.
	// But standard encoding/xml doesn't support generic "any XML" well without custom UnmarshalXML.
	Raw []byte `xml:",innerxml"`
}

type LocaleSet struct {
	Locale []Locale `xml:"locale"`
}

type Locale struct {
	Name string `xml:"name,attr"`
	Desc string `xml:"desc,attr"`
	// ... date patterns, number patterns ...
}

type ConnectionSet struct {
	WsdlConnection []WsdlConnection `xml:"wsdlConnection"`
	XsdConnection  []XsdConnection  `xml:"xsdConnection"`
}

type WsdlConnection struct {
	Name string `xml:"name,attr"`
	// ...
}

type XsdConnection struct {
	Name    string `xml:"name,attr"`
	DataRef string `xml:"dataDescription,attr"`
}
