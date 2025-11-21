package cmm

import (
	"encoding/binary"
	"errors"
)

// ICCProfile implements Profile for ICC data.
type ICCProfile struct {
	data   []byte
	header iccHeader
	tags   map[string]iccTag
}

type iccHeader struct {
	Size       uint32
	CMMType    uint32
	Version    uint32
	Class      uint32
	ColorSpace uint32
	PCS        uint32
	Date       [12]byte
	Signature  uint32
	Platform   uint32
	Flags      uint32
	DevMfgr    uint32
	DevModel   uint32
	Attributes uint64
	Intent     uint32
	Illuminant [12]byte
	Creator    uint32
	ID         [16]byte
}

type iccTag struct {
	Sig    uint32
	Offset uint32
	Size   uint32
}

// NewICCProfile creates a new ICCProfile from bytes.
func NewICCProfile(data []byte) (*ICCProfile, error) {
	if len(data) < 128 {
		return nil, errors.New("invalid ICC profile data: too short")
	}
	p := &ICCProfile{data: data}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *ICCProfile) parse() error {
	// Parse Header
	p.header.Size = binary.BigEndian.Uint32(p.data[0:4])
	p.header.CMMType = binary.BigEndian.Uint32(p.data[4:8])
	p.header.Version = binary.BigEndian.Uint32(p.data[8:12])
	p.header.Class = binary.BigEndian.Uint32(p.data[12:16])
	p.header.ColorSpace = binary.BigEndian.Uint32(p.data[16:20])
	p.header.PCS = binary.BigEndian.Uint32(p.data[20:24])
	// ... skip date ...
	p.header.Signature = binary.BigEndian.Uint32(p.data[36:40])
	if p.header.Signature != 0x61637370 { // 'acsp'
		return errors.New("invalid ICC signature")
	}

	// Parse Tag Table
	tagCount := binary.BigEndian.Uint32(p.data[128:132])
	p.tags = make(map[string]iccTag)
	offset := 132
	for i := uint32(0); i < tagCount; i++ {
		if offset+12 > len(p.data) {
			return errors.New("ICC tag table truncated")
		}
		sig := binary.BigEndian.Uint32(p.data[offset : offset+4])
		tagOffset := binary.BigEndian.Uint32(p.data[offset+4 : offset+8])
		tagSize := binary.BigEndian.Uint32(p.data[offset+8 : offset+12])

		tagStr := uint32ToString(sig)
		p.tags[tagStr] = iccTag{Sig: sig, Offset: tagOffset, Size: tagSize}
		offset += 12
	}
	return nil
}

func (p *ICCProfile) Name() string {
	// Try to find 'desc' tag (textDescriptionType or multiLocalizedUnicodeType)
	if tag, ok := p.tags["desc"]; ok {
		return p.readDescription(tag)
	}
	// Try 'dscm' (V4)
	if tag, ok := p.tags["dscm"]; ok {
		return p.readDescription(tag)
	}
	return "ICC Profile"
}

func (p *ICCProfile) readDescription(tag iccTag) string {
	if int(tag.Offset+tag.Size) > len(p.data) {
		return ""
	}
	raw := p.data[tag.Offset : tag.Offset+tag.Size]
	if len(raw) < 8 {
		return ""
	}
	sig := binary.BigEndian.Uint32(raw[0:4])
	if sig == 0x64657363 { // 'desc' - textDescriptionType
		// 8-12: ASCII count
		count := binary.BigEndian.Uint32(raw[8:12])
		if 12+count <= uint32(len(raw)) {
			return string(raw[12 : 12+count-1]) // null terminated
		}
	} else if sig == 0x6D6C7563 { // 'mluc' - multiLocalizedUnicodeType
		// Complex parsing, skip for now or implement basic
		// 8-12: Number of records
		// 12-16: Record size (12)
		numRecs := binary.BigEndian.Uint32(raw[8:12])
		if numRecs > 0 && len(raw) >= 28 {
			// First record: ISO-639 (2), ISO-3166 (2), Len (4), Off (4)
			// nameLen := binary.BigEndian.Uint32(raw[20:24])
			nameOff := binary.BigEndian.Uint32(raw[24:28])
			// Offset is relative to tag start
			if nameOff < tag.Size {
				// Read UTF-16BE... simplified
				return "Localized Profile"
			}
		}
	}
	return ""
}

func (p *ICCProfile) ColorSpace() string {
	return uint32ToString(p.header.ColorSpace)
}

func (p *ICCProfile) Class() string {
	return uint32ToString(p.header.Class)
}

func (p *ICCProfile) Data() []byte {
	return p.data
}

// GetTag returns the raw data for a tag if it exists.
func (p *ICCProfile) GetTag(sig string) ([]byte, bool) {
	tag, ok := p.tags[sig]
	if !ok {
		return nil, false
	}
	if int(tag.Offset+tag.Size) > len(p.data) {
		return nil, false
	}
	return p.data[tag.Offset : tag.Offset+tag.Size], true
}

// ReadXYZTag reads an XYZType tag (array of 3 XYZ numbers).
func (p *ICCProfile) ReadXYZTag(sig string) ([3]float64, error) {
	data, ok := p.GetTag(sig)
	if !ok {
		return [3]float64{}, errors.New("tag not found")
	}
	if len(data) < 20 { // 8 byte header + 12 bytes data
		return [3]float64{}, errors.New("tag too short")
	}
	typeSig := binary.BigEndian.Uint32(data[0:4])
	if typeSig != 0x58595A20 { // 'XYZ '
		return [3]float64{}, errors.New("invalid tag type")
	}

	x := s15Fixed16ToFloat(binary.BigEndian.Uint32(data[8:12]))
	y := s15Fixed16ToFloat(binary.BigEndian.Uint32(data[12:16]))
	z := s15Fixed16ToFloat(binary.BigEndian.Uint32(data[16:20]))
	return [3]float64{x, y, z}, nil
}

// ReadCurveTag reads a curveType or parametricCurveType tag.
// Returns a gamma value (if simple gamma) or error if complex/LUT.
func (p *ICCProfile) ReadCurveTag(sig string) (float64, error) {
	data, ok := p.GetTag(sig)
	if !ok {
		return 0, errors.New("tag not found")
	}
	if len(data) < 12 {
		return 0, errors.New("tag too short")
	}
	typeSig := binary.BigEndian.Uint32(data[0:4])
	if typeSig == 0x63757276 { // 'curv'
		count := binary.BigEndian.Uint32(data[8:12])
		if count == 0 {
			return 1.0, nil // Identity
		}
		if count == 1 {
			// u8Fixed8
			val := binary.BigEndian.Uint16(data[12:14])
			return float64(val) / 256.0, nil
		}
		// Simple LUT support: if count > 1, we can't return a single gamma.
		// But the interface returns float64. We need to change the interface or return error.
		// For now, let's approximate or return error.
		// Ideally, ReadCurveTag should return a Curve interface or struct.
		return 0, errors.New("LUT curves not supported in simple gamma interface")
	}

	if typeSig == 0x70617261 { // 'para'
		// Parametric curve
		// 8-10: Function type
		// 10-12: Reserved
		funcType := binary.BigEndian.Uint16(data[8:10])

		// Parameters start at 12
		// We only support type 0 (gamma) for this simple interface
		if funcType == 0 {
			g := s15Fixed16ToFloat(binary.BigEndian.Uint32(data[12:16]))
			return g, nil
		}
		return 0, errors.New("unsupported parametric curve type")
	}

	return 0, errors.New("unsupported curve type")
}

func s15Fixed16ToFloat(v uint32) float64 {
	// Signed 15.16 fixed point number
	// We treat it as int32 to handle sign correctly
	i := int32(v)
	return float64(i) / 65536.0
}

func uint32ToString(v uint32) string {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return string(b)
}
