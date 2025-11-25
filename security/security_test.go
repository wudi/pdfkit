package security

import (
	"crypto/aes"
	"encoding/binary"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
)

func TestEncryptionVariantsRoundTrip(t *testing.T) {
	fileID := []byte("fileid-variants")
	payload := []byte("secret aes/rc4 payload")
	cases := []struct {
		name            string
		rawPerms        raw.Permissions
		encryptMetadata bool
		build           func([]byte) (*raw.DictObj, string, error)
	}{
		{
			name: "RC4_40bit_DefaultPerms",
			rawPerms: raw.Permissions{
				Print:             true,
				Copy:              false,
				Modify:            true,
				ExtractAccessible: true,
			},
			encryptMetadata: true,
			build: func(fileID []byte) (*raw.DictObj, string, error) {
				enc, _, err := BuildStandardEncryption("user40", "owner40", raw.Permissions{
					Print:             true,
					Copy:              false,
					Modify:            true,
					ExtractAccessible: true,
				}, fileID, true)
				return enc, "user40", err
			},
		},
		{
			name: "RC4_128bit_MetadataOff",
			rawPerms: raw.Permissions{
				Print:     false,
				Copy:      false,
				Modify:    false,
				FillForms: true,
				Assemble:  true,
			},
			encryptMetadata: false,
			build: func(fileID []byte) (*raw.DictObj, string, error) {
				enc, _, err := BuildRC4Encryption("user128", "owner128", raw.Permissions{
					Print:     false,
					Copy:      false,
					Modify:    false,
					FillForms: true,
					Assemble:  true,
				}, fileID, 128, false)
				return enc, "user128", err
			},
		},
		{
			name: "AES_128bit",
			rawPerms: raw.Permissions{
				Print:             true,
				Copy:              true,
				Modify:            false,
				ModifyAnnotations: false,
				PrintHighQuality:  false,
			},
			encryptMetadata: true,
			build: func(fileID []byte) (*raw.DictObj, string, error) {
				return buildAES128Dict(raw.Permissions{
					Print:             true,
					Copy:              true,
					Modify:            false,
					ModifyAnnotations: false,
					PrintHighQuality:  false,
				}, true), "user-aes", nil
			},
		},
		{
			name: "AES_256bit_Permissions",
			rawPerms: raw.Permissions{
				Print:             true,
				Copy:              false,
				Modify:            true,
				FillForms:         true,
				ExtractAccessible: true,
				PrintHighQuality:  true,
			},
			encryptMetadata: false,
			build: func(fileID []byte) (*raw.DictObj, string, error) {
				fileKey := []byte("0123456789abcdef0123456789abcdef")
				userPwd := []byte("user256")
				ownerPwd := []byte("owner256")
				uEntry, ue := buildUserEntries(userPwd, fileID, fileKey)
				oEntry, oe := buildOwnerEntries(ownerPwd, uEntry, fileKey)

				enc := raw.Dict()
				enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
				enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(5))
				enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(6))
				enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(256))
				enc.Set(raw.NameObj{Val: "U"}, raw.StringObj{Bytes: uEntry})
				enc.Set(raw.NameObj{Val: "UE"}, raw.StringObj{Bytes: ue})
				enc.Set(raw.NameObj{Val: "O"}, raw.StringObj{Bytes: oEntry})
				enc.Set(raw.NameObj{Val: "OE"}, raw.StringObj{Bytes: oe})
				pVal := PermissionsValue(raw.Permissions{
					Print:             true,
					Copy:              false,
					Modify:            true,
					FillForms:         true,
					ExtractAccessible: true,
					PrintHighQuality:  true,
				})
				enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})
				perms := make([]byte, 16)
				binary.LittleEndian.PutUint32(perms[:4], uint32(pVal))
				perms[8] = 0x46 // EncryptMetadata = false
				perms[9] = 0x61
				perms[10] = 0x64
				perms[11] = 0x62
				for i := 4; i < 8; i++ {
					perms[i] = 0xFF
				}
				for i := 12; i < 16; i++ {
					perms[i] = 0xFF
				}
				block, _ := aes.NewCipher(fileKey)
				var encPerms [16]byte
				block.Encrypt(encPerms[:], perms)
				enc.Set(raw.NameObj{Val: "Perms"}, raw.StringObj{Bytes: encPerms[:]})
				enc.Set(raw.NameObj{Val: "EncryptMetadata"}, raw.Bool(false))

				return enc, "user256", nil
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			encDict, pwd, err := tc.build(fileID)
			if err != nil {
				t.Fatalf("build encrypt dictionary: %v", err)
			}
			h, err := (&HandlerBuilder{}).WithEncryptDict(encDict).WithFileID(fileID).Build()
			if err != nil {
				t.Fatalf("build handler: %v", err)
			}
			if err := h.Authenticate(pwd); err != nil {
				t.Fatalf("authenticate: %v", err)
			}
			if got := h.Permissions(); got != toSecurityPerms(tc.rawPerms) {
				t.Fatalf("permissions mismatch: got %+v want %+v", got, toSecurityPerms(tc.rawPerms))
			}
			if h.EncryptMetadata() != tc.encryptMetadata {
				t.Fatalf("encrypt metadata mismatch: got %v want %v", h.EncryptMetadata(), tc.encryptMetadata)
			}
			encData, err := h.Encrypt(5, 0, payload, DataClassStream)
			if err != nil {
				t.Fatalf("encrypt stream: %v", err)
			}
			decData, err := h.Decrypt(5, 0, encData, DataClassStream)
			if err != nil {
				t.Fatalf("decrypt stream: %v", err)
			}
			if string(decData) != string(payload) {
				t.Fatalf("stream roundtrip mismatch: got %q want %q", decData, payload)
			}
			strEnc, err := h.Encrypt(7, 0, payload, DataClassString)
			if err != nil {
				t.Fatalf("encrypt string: %v", err)
			}
			strDec, err := h.Decrypt(7, 0, strEnc, DataClassString)
			if err != nil {
				t.Fatalf("decrypt string: %v", err)
			}
			if string(strDec) != string(payload) {
				t.Fatalf("string roundtrip mismatch: got %q want %q", strDec, payload)
			}
		})
	}
}

func TestBuildEncryptionHelpers(t *testing.T) {
	fileID := []byte("fileid-build-encryption")
	payload := []byte("secret payload")
	perms := raw.Permissions{Print: true, Copy: true}

	cases := []struct {
		name  string
		opts  EncryptionOptions
		user  string
		owner string
	}{
		{name: "AES128", opts: EncryptionOptions{Algorithm: EncryptionAlgorithmAES, KeyLength: 128}, user: "user-aes", owner: "owner-aes"},
		{name: "AES256", opts: EncryptionOptions{Algorithm: EncryptionAlgorithmAES, KeyLength: 256}, user: "user-aes256", owner: "owner-aes256"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			encDict, _, err := BuildEncryption(tc.user, tc.owner, perms, fileID, tc.opts, true)
			if err != nil {
				t.Fatalf("build encryption: %v", err)
			}
			if tc.opts.KeyLength >= 256 {
				uVal, _ := encDict.Get(raw.NameObj{Val: "U"})
				ueVal, _ := encDict.Get(raw.NameObj{Val: "UE"})
				uEntry, _ := uVal.(raw.StringObj)
				ueEntry, _ := ueVal.(raw.StringObj)
				if key, ok, err := deriveAES256User([]byte(tc.user), uEntry.Value(), ueEntry.Value(), fileID); err != nil {
					t.Fatalf("manual user derivation failed: %v", err)
				} else if !ok {
					t.Fatalf("manual user derivation returned invalid")
				} else if len(key) == 0 {
					t.Fatalf("manual user key empty")
				}
			}
			handler, err := (&HandlerBuilder{}).WithEncryptDict(encDict).WithFileID(fileID).Build()
			if err != nil {
				t.Fatalf("build handler: %v", err)
			}
			if err := handler.Authenticate(tc.user); err != nil {
				t.Fatalf("authenticate: %v", err)
			}
			enc, err := handler.Encrypt(1, 0, payload, DataClassStream)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			dec, err := handler.Decrypt(1, 0, enc, DataClassStream)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if string(dec) != string(payload) {
				t.Fatalf("roundtrip mismatch: got %q want %q", dec, payload)
			}
		})
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

func TestAES256RejectsMalformedEntries(t *testing.T) {
	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(5))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(6))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(256))
	enc.Set(raw.NameObj{Val: "U"}, raw.StringObj{Bytes: []byte("short")})
	enc.Set(raw.NameObj{Val: "UE"}, raw.StringObj{Bytes: []byte("short")})

	h, err := (&HandlerBuilder{}).WithEncryptDict(enc).WithFileID([]byte("id")).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := h.Authenticate("pass"); err == nil {
		t.Fatalf("expected authentication failure for malformed UE/U")
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

func buildAES128Dict(perms raw.Permissions, encryptMetadata bool) *raw.DictObj {
	pVal := PermissionsValue(perms)
	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberInt(4))
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberInt(4))
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(128))
	enc.Set(raw.NameObj{Val: "O"}, raw.Str(padPassword([]byte("owner-aes"))))
	enc.Set(raw.NameObj{Val: "U"}, raw.Str(passwordPadding))
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})

	cf := raw.Dict()
	std := raw.Dict()
	std.Set(raw.NameObj{Val: "Type"}, raw.NameObj{Val: "CryptFilter"})
	std.Set(raw.NameObj{Val: "CFM"}, raw.NameObj{Val: "AESV2"})
	std.Set(raw.NameObj{Val: "AuthEvent"}, raw.NameObj{Val: "DocOpen"})
	std.Set(raw.NameObj{Val: "Length"}, raw.NumberInt(128))
	cf.Set(raw.NameObj{Val: "StdCF"}, std)
	enc.Set(raw.NameObj{Val: "CF"}, cf)
	enc.Set(raw.NameObj{Val: "StmF"}, raw.NameObj{Val: "StdCF"})
	enc.Set(raw.NameObj{Val: "StrF"}, raw.NameObj{Val: "StdCF"})
	if !encryptMetadata {
		enc.Set(raw.NameObj{Val: "EncryptMetadata"}, raw.Bool(false))
	}
	return enc
}

func toSecurityPerms(p raw.Permissions) Permissions {
	return Permissions{
		Print:             p.Print,
		Modify:            p.Modify,
		Copy:              p.Copy,
		ModifyAnnotations: p.ModifyAnnotations,
		FillForms:         p.FillForms,
		ExtractAccessible: p.ExtractAccessible,
		Assemble:          p.Assemble,
		PrintHighQuality:  p.PrintHighQuality,
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
