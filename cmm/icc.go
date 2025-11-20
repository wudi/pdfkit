package cmm

import "errors"

// ICCProfile implements Profile for ICC data.
type ICCProfile struct {
	data []byte
}

// NewICCProfile creates a new ICCProfile from bytes.
func NewICCProfile(data []byte) (*ICCProfile, error) {
	if len(data) < 128 {
		return nil, errors.New("invalid ICC profile data")
	}
	return &ICCProfile{data: data}, nil
}

func (p *ICCProfile) Name() string {
	// In a real implementation, parse the 'desc' tag.
	return "ICC Profile"
}

func (p *ICCProfile) ColorSpace() string {
	// In a real implementation, parse header bytes 16-20.
	if len(p.data) > 20 {
		return string(p.data[16:20])
	}
	return ""
}

func (p *ICCProfile) Class() string {
	// In a real implementation, parse header bytes 12-16.
	if len(p.data) > 16 {
		return string(p.data[12:16])
	}
	return ""
}

func (p *ICCProfile) Data() []byte {
	return p.data
}
