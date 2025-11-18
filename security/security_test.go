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
	encData, err := h.Encrypt(5, 0, plain, DataClassStream)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decData, err := h.Decrypt(5, 0, encData, DataClassStream)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decData) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", decData, plain)
	}
}

func TestAESRoundTrip(t *testing.T) {
	owner := raw.StringObj{Bytes: []byte("ownerpass")}
	fileID := []byte("fileid1")
	pVal := int32(-4)

	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(4))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(4))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(128))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(128))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(128))
	enc.Set(raw.NameObj{Val: "O"}, owner)
	enc.Set(raw.NameObj{Val: "U"}, raw.StringObj{Bytes: passwordPadding})
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})

	h, err := (&HandlerBuilder{}).WithEncryptDict(enc).WithFileID(fileID).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := h.Authenticate(""); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	plain := []byte("secret aes data 1234")
	encData, err := h.Encrypt(7, 0, plain, DataClassStream)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decData, err := h.Decrypt(7, 0, encData, DataClassStream)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decData) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", decData, plain)
	}
}

func TestIdentityCryptFilterSkipsEncryption(t *testing.T) {
	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(4))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(4))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(128))

	cf := raw.Dict()
	identity := raw.Dict()
	identity.Set(raw.NameObj{Val: "CFM"}, raw.NameObj{Val: "None"})
	cf.Set(raw.NameObj{Val: "Identity"}, identity)
	enc.Set(raw.NameObj{Val: "CF"}, cf)
	enc.Set(raw.NameObj{Val: "StmF"}, raw.NameObj{Val: "Identity"})
	enc.Set(raw.NameObj{Val: "StrF"}, raw.NameObj{Val: "Identity"})

	h, err := (&HandlerBuilder{}).WithEncryptDict(enc).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := h.Authenticate(""); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	data := []byte("plaintext")
	out, err := h.Encrypt(1, 0, data, DataClassStream)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if string(out) != string(data) {
		t.Fatalf("identity crypt filter should leave data unchanged")
	}
}
