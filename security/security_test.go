package security

import (
	"crypto/aes"
	"encoding/binary"
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

func TestAES256RejectsWrongPassword(t *testing.T) {
	fileKey := []byte("0123456789abcdef0123456789abcdef")
	fileID := []byte("fileid-aes256")
	password := []byte("pass123")

	uEntry, ue := buildUserEntries(password, fileID, fileKey)
	oEntry, oe := buildOwnerEntries(password, uEntry, fileKey)

	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(5))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(6))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(256))
	enc.Set(raw.NameObj{Val: "U"}, raw.StringObj{Bytes: uEntry})
	enc.Set(raw.NameObj{Val: "UE"}, raw.StringObj{Bytes: ue})
	enc.Set(raw.NameObj{Val: "O"}, raw.StringObj{Bytes: oEntry})
	enc.Set(raw.NameObj{Val: "OE"}, raw.StringObj{Bytes: oe})
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberInt(-4))

	h, err := (&HandlerBuilder{}).WithEncryptDict(enc).WithFileID(fileID).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := h.Authenticate("wrong"); err == nil {
		t.Fatalf("expected authentication failure with wrong password")
	}
}

func TestAES256Authentication(t *testing.T) {
	fileKey := []byte("0123456789abcdef0123456789abcdef")
	fileID := []byte("fileid-aes256")
	password := []byte("pass123")

	uEntry, ue := buildUserEntries(password, fileID, fileKey)
	oEntry, oe := buildOwnerEntries(password, uEntry, fileKey)

	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(5))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(6))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(256))
	enc.Set(raw.NameObj{Val: "U"}, raw.StringObj{Bytes: uEntry})
	enc.Set(raw.NameObj{Val: "UE"}, raw.StringObj{Bytes: ue})
	enc.Set(raw.NameObj{Val: "O"}, raw.StringObj{Bytes: oEntry})
	enc.Set(raw.NameObj{Val: "OE"}, raw.StringObj{Bytes: oe})
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberInt(-4))

	perms := make([]byte, 16)
	binary.LittleEndian.PutUint32(perms[:4], uint32(0xFFFFFFFC))
	copy(perms[12:], []byte("perm"))
	block, _ := aes.NewCipher(fileKey)
	var encPerms [16]byte
	block.Encrypt(encPerms[:], perms)
	enc.Set(raw.NameObj{Val: "Perms"}, raw.StringObj{Bytes: encPerms[:]})

	trailer := raw.Dict()
	idArr := raw.NewArray(raw.StringObj{Bytes: fileID}, raw.StringObj{Bytes: fileID})
	trailer.Set(raw.NameObj{Val: "ID"}, idArr)
	trailer.Set(raw.NameObj{Val: "Encrypt"}, raw.RefObj{})

	h, err := (&HandlerBuilder{}).WithEncryptDict(enc).WithTrailer(trailer).WithFileID(fileID).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := h.Authenticate(string(password)); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	stream := []byte("secret data")
	encData, err := h.Encrypt(10, 0, stream, DataClassStream)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	dec, err := h.Decrypt(10, 0, encData, DataClassStream)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(dec) != string(stream) {
		t.Fatalf("aes256 roundtrip mismatch: got %q want %q", dec, stream)
	}
}

func buildUserEntries(pwd []byte, fileID []byte, fileKey []byte) ([]byte, []byte) {
	uSalt := []byte("usersalt")
	ukSalt := []byte("ukeysalt")
	hashVal := rev6Hash(pwd, uSalt, fileID)
	keyHash := rev6Hash(pwd, ukSalt, fileID)
	ue, _ := aesCBCNoIV(keyHash[:32], fileKey, true)
	entry := make([]byte, 0, 48)
	entry = append(entry, hashVal[:]...)
	entry = append(entry, uSalt...)
	entry = append(entry, ukSalt...)
	return entry, ue
}

func buildOwnerEntries(pwd []byte, uEntry []byte, fileKey []byte) ([]byte, []byte) {
	oSalt := []byte("ownerslt")
	okSalt := []byte("okeysalt")
	hashVal := rev6Hash(pwd, oSalt, uEntry[:48])
	keyHash := rev6Hash(pwd, okSalt, uEntry[:48])
	oe, _ := aesCBCNoIV(keyHash[:32], fileKey, true)
	entry := make([]byte, 0, 48)
	entry = append(entry, hashVal[:]...)
	entry = append(entry, oSalt...)
	entry = append(entry, okSalt...)
	return entry, oe
}
