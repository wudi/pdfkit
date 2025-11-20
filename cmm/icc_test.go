package cmm

import (
"encoding/binary"
"testing"
)

func TestICCProfileParse(t *testing.T) {
	data := make([]byte, 132)
	binary.BigEndian.PutUint32(data[0:4], 132)
	binary.BigEndian.PutUint32(data[12:16], 0x6D6E7472) // mntr
	binary.BigEndian.PutUint32(data[16:20], 0x52474220) // RGB 
	binary.BigEndian.PutUint32(data[36:40], 0x61637370) // acsp
	binary.BigEndian.PutUint32(data[128:132], 0)

	p, err := NewICCProfile(data)
	if err != nil {
		t.Fatalf("NewICCProfile failed: %v", err)
	}

	if p.Class() != "mntr" {
		t.Errorf("expected class 'mntr', got '%s'", p.Class())
	}
	if p.ColorSpace() != "RGB " {
		t.Errorf("expected color space 'RGB ', got '%s'", p.ColorSpace())
	}
}
