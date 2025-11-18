package security

import (
	"testing"

	"pdflib/ir/raw"
)

func TestStandardRC4RoundTrip(t *testing.T) {
	owner := raw.StringObj{Bytes: []byte("ownerpass")}
	fileID := []byte("fileid0")
	pVal := int32(-4)

	key, err := deriveKey([]byte(""), owner.Value(), pVal, fileID, 5, 2)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	user := rc4Simple(key, passwordPadding)

	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(1))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(2))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(40))
	enc.Set(raw.NameObj{Val: "O"}, owner)
	enc.Set(raw.NameObj{Val: "U"}, raw.StringObj{Bytes: user})
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})

	h, err := (&HandlerBuilder{}).WithEncryptDict(enc).WithFileID(fileID).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := h.Authenticate(""); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	plain := []byte("secret data")
	encData, err := h.Encrypt(5, 0, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decData, err := h.Decrypt(5, 0, encData)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decData) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", decData, plain)
	}
}
