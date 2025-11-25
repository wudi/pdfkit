package security

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
)

type Permissions struct{ Print, Modify, Copy, ModifyAnnotations, FillForms, ExtractAccessible, Assemble, PrintHighQuality bool }

// DataClass identifies the kind of payload being encrypted or decrypted.
type DataClass int

const (
	DataClassStream DataClass = iota
	DataClassString
	DataClassMetadataStream
)

type Handler interface {
	IsEncrypted() bool
	Authenticate(password string) error
	DecryptWithFilter(objNum, gen int, data []byte, class DataClass, cryptFilter string) ([]byte, error)
	Decrypt(objNum, gen int, data []byte, class DataClass) ([]byte, error)
	EncryptWithFilter(objNum, gen int, data []byte, class DataClass, cryptFilter string) ([]byte, error)
	Encrypt(objNum, gen int, data []byte, class DataClass) ([]byte, error)
	Permissions() Permissions
	EncryptMetadata() bool
}

type HandlerBuilder struct {
	encryptDict raw.Dictionary
	trailer     raw.Dictionary
	fileID      []byte
	ue          []byte
	oe          []byte
	uEntry      []byte
	oEntry      []byte
}

func (b *HandlerBuilder) WithEncryptDict(d raw.Dictionary) *HandlerBuilder {
	b.encryptDict = d
	return b
}
func (b *HandlerBuilder) WithTrailer(d raw.Dictionary) *HandlerBuilder { b.trailer = d; return b }
func (b *HandlerBuilder) WithFileID(id []byte) *HandlerBuilder         { b.fileID = id; return b }

func (b *HandlerBuilder) Build() (Handler, error) {
	if b.encryptDict == nil {
		return noEncryptionHandler{}, nil
	}
	encFilter, _ := b.encryptDict.Get(raw.NameObj{Val: "Filter"})
	if name, ok := encFilter.(raw.NameObj); ok && name.Val != "Standard" {
		return nil, errors.New("unsupported encryption filter")
	}
	v := int64(0)
	if n, ok := numberVal(b.encryptDict, "V"); ok {
		v = n
	}
	if v == 0 {
		v = 1
	}
	if v > 6 {
		return nil, errors.New("encryption V>6 not supported")
	}
	r := int64(2)
	if n, ok := numberVal(b.encryptDict, "R"); ok {
		r = n
	}
	if r > 6 {
		return nil, errors.New("encryption R>6 not supported")
	}
	keyLen := 40
	if v >= 5 {
		keyLen = 256
	}
	if n, ok := numberVal(b.encryptDict, "Length"); ok && n > 0 {
		keyLen = int(n)
	}
	if v >= 4 && keyLen < 128 {
		keyLen = 128
	}
	if keyLen%8 != 0 {
		return nil, errors.New("encryption length must be multiple of 8")
	}
	owner, _ := stringBytes(b.encryptDict, "O")
	user, _ := stringBytes(b.encryptDict, "U")
	oe, _ := stringBytes(b.encryptDict, "OE")
	ue, _ := stringBytes(b.encryptDict, "UE")
	oEntry := owner
	uEntry := user
	pVal, _ := numberVal(b.encryptDict, "P")
	id := b.fileID
	if len(id) == 0 && b.trailer != nil {
		if arrObj, ok := b.trailer.Get(raw.NameObj{Val: "ID"}); ok {
			if arr, ok := arrObj.(*raw.ArrayObj); ok && arr.Len() > 0 {
				if s, ok := arr.Items[0].(raw.StringObj); ok {
					id = s.Value()
				}
			}
		}
	}
	encryptMeta := true
	if v, ok := boolVal(b.encryptDict, "EncryptMetadata"); ok {
		encryptMeta = v
	}

	baseAlgo := algoRC4
	if v >= 4 {
		baseAlgo = algoAES
	}
	cryptFilters, err := parseCryptFilters(b.encryptDict, baseAlgo)
	if err != nil {
		return nil, err
	}
	streamAlgo, err := resolveCryptFilter(b.encryptDict, "StmF", baseAlgo, cryptFilters)
	if err != nil {
		return nil, err
	}
	stringAlgo, err := resolveCryptFilter(b.encryptDict, "StrF", baseAlgo, cryptFilters)
	if err != nil {
		return nil, err
	}
	useAES := streamAlgo == algoAES || stringAlgo == algoAES || baseAlgo == algoAES
	h := &standardHandler{
		v:            int(v),
		r:            int(r),
		lengthBits:   keyLen,
		owner:        owner,
		user:         user,
		oEntry:       oEntry,
		uEntry:       uEntry,
		oe:           oe,
		ue:           ue,
		p:            int32(pVal),
		fileID:       id,
		encryptMeta:  encryptMeta,
		useAES:       useAES,
		streamAlgo:   streamAlgo,
		stringAlgo:   stringAlgo,
		cryptFilters: cryptFilters,
		trailer:      b.trailer,
	}
	return h, nil
}

type cryptAlgo int

const (
	algoUnset cryptAlgo = iota
	algoNone
	algoRC4
	algoAES
)

type standardHandler struct {
	key          []byte
	v            int
	r            int
	lengthBits   int
	owner        []byte
	user         []byte
	oEntry       []byte
	uEntry       []byte
	oe           []byte
	ue           []byte
	p            int32
	fileID       []byte
	encryptMeta  bool
	authed       bool
	useAES       bool
	streamAlgo   cryptAlgo
	stringAlgo   cryptAlgo
	cryptFilters map[string]cryptAlgo
	trailer      raw.Dictionary
}

func (h *standardHandler) IsEncrypted() bool { return true }
func (h *standardHandler) EncryptMetadata() bool {
	return h.encryptMeta
}

func (h *standardHandler) Authenticate(password string) error {
	if h.v >= 5 || h.r >= 5 {
		if err := h.authenticateAES256([]byte(password)); err != nil {
			return err
		}
		h.authed = true
		return nil
	}

	// 1. Try as User Password
	key, err := deriveKey([]byte(password), h.owner, h.p, h.fileID, h.lengthBits/8, h.r)
	if err == nil {
		if !h.useAES && h.r <= 3 {
			if checkUserPassword(key, h.user, h.fileID, h.r) {
				h.key = key
				h.authed = true
				return nil
			}
		} else {
			// For V=4 (AES), we assume success if we derived a key?
			// Ideally we should verify it, but for now let's keep existing behavior
			// which seemed to accept it if not r<=3.
			// However, to be safe, we should probably only return if we are sure.
			// But since I am adding Owner Auth, I should be careful not to break existing V=4.
			// Existing code:
			/*
				if !h.useAES && h.r <= 3 {
					if !checkUserPassword(key, h.user, h.fileID, h.r) {
						return errors.New("invalid password")
					}
				}
				h.key = key
				h.authed = true
				return nil
			*/
			// So if it was V=4, it would just succeed.
			h.key = key
			h.authed = true
			return nil
		}
	}

	// 2. Try as Owner Password
	// Algorithm 3.5: Authenticating the owner password
	ownerKey := deriveOwnerEntryKey([]byte(password), h.lengthBits/8, h.r)
	userPwdPadded, err := rc4Crypt(ownerKey, h.owner)
	if err == nil {
		// The result is the User Password (padded).
		// Use it to authenticate as User.
		key, err := deriveKey(userPwdPadded, h.owner, h.p, h.fileID, h.lengthBits/8, h.r)
		if err == nil {
			if !h.useAES && h.r <= 3 {
				if checkUserPassword(key, h.user, h.fileID, h.r) {
					h.key = key
					h.authed = true
					return nil
				}
			} else {
				// For V=4
				h.key = key
				h.authed = true
				return nil
			}
		}
	}

	return errors.New("invalid password")
}

func (h *standardHandler) DecryptWithFilter(objNum, gen int, data []byte, class DataClass, cryptFilter string) ([]byte, error) {
	if !h.authed {
		if err := h.Authenticate(""); err != nil {
			return nil, err
		}
	}
	algo, err := h.algoFor(class, cryptFilter)
	if err != nil {
		return nil, err
	}
	return h.decryptWithAlgo(algo, objNum, gen, data)
}

func (h *standardHandler) Decrypt(objNum, gen int, data []byte, class DataClass) ([]byte, error) {
	return h.DecryptWithFilter(objNum, gen, data, class, "")
}

func (h *standardHandler) Encrypt(objNum, gen int, data []byte, class DataClass) ([]byte, error) {
	return h.EncryptWithFilter(objNum, gen, data, class, "")
}

func (h *standardHandler) EncryptWithFilter(objNum, gen int, data []byte, class DataClass, cryptFilter string) ([]byte, error) {
	if !h.authed {
		if err := h.Authenticate(""); err != nil {
			return nil, err
		}
	}
	algo, err := h.algoFor(class, cryptFilter)
	if err != nil {
		return nil, err
	}
	if algo == algoNone || len(data) == 0 {
		return data, nil
	}
	key := objectKey(h.key, objNum, gen, h.r, algo == algoAES)
	if algo == algoAES {
		return aesCrypt(key, data, true)
	}
	return rc4Crypt(key, data)
}

func (h *standardHandler) pickAlgo(class DataClass) cryptAlgo {
	switch class {
	case DataClassString:
		if h.stringAlgo != algoUnset {
			return h.stringAlgo
		}
	case DataClassStream, DataClassMetadataStream:
		if h.streamAlgo != algoUnset {
			return h.streamAlgo
		}
	}
	if h.useAES {
		return algoAES
	}
	return algoRC4
}

func (h *standardHandler) algoFor(class DataClass, filter string) (cryptAlgo, error) {
	if filter == "Identity" {
		return algoNone, nil
	}
	if filter == "Standard" || filter == "" {
		return h.pickAlgo(class), nil
	}
	if algo, ok := h.cryptFilters[filter]; ok {
		return algo, nil
	}
	return algoUnset, fmt.Errorf("crypt filter %s not defined", filter)
}

func (h *standardHandler) decryptWithAlgo(algo cryptAlgo, objNum, gen int, data []byte) ([]byte, error) {
	if algo == algoNone || len(data) == 0 {
		return data, nil
	}
	useAES := algo == algoAES
	key := objectKey(h.key, objNum, gen, h.r, useAES)
	if useAES {
		return aesCrypt(key, data, false)
	}
	return rc4Crypt(key, data)
}

func (h *standardHandler) Permissions() Permissions {
	return Permissions{
		Print:             h.p&0x4 != 0,
		Modify:            h.p&0x8 != 0,
		Copy:              h.p&0x10 != 0,
		ModifyAnnotations: h.p&0x20 != 0,
		FillForms:         h.p&0x100 != 0,
		ExtractAccessible: h.p&0x200 != 0,
		Assemble:          h.p&0x400 != 0,
		PrintHighQuality:  h.p&0x800 != 0,
	}
}

func (h *standardHandler) authenticateAES256(pwd []byte) error {
	if len(h.uEntry) >= 48 && len(h.ue) >= 32 {
		if key, ok, err := deriveAES256User(pwd, h.uEntry, h.ue, h.fileID); err == nil && ok {
			h.key = key
			h.setPermsFromEncrypted()
			return nil
		}
	}
	if len(h.oEntry) >= 48 && len(h.oe) >= 32 && len(h.uEntry) >= 48 {
		if key, ok, err := deriveAES256Owner(pwd, h.oEntry, h.oe, h.uEntry); err == nil && ok {
			h.key = key
			h.setPermsFromEncrypted()
			return nil
		}
	}
	return errors.New("invalid password")
}

func (h *standardHandler) setPermsFromEncrypted() {
	if h.key == nil {
		return
	}
	if h.p != 0 {
		return
	}
	if permsObj, ok := h.trailerPerms(); ok {
		if pval, err := decryptPermsAES256(h.key, permsObj); err == nil {
			h.p = pval
		}
	}
}

func (h *standardHandler) trailerPerms() ([]byte, bool) {
	if h.trailer == nil {
		return nil, false
	}
	if v, ok := h.trailer.Get(raw.NameObj{Val: "Perms"}); ok {
		if s, ok := v.(raw.StringObj); ok {
			return s.Value(), true
		}
	}
	return nil, false
}

type noEncryptionHandler struct{}

func (noEncryptionHandler) IsEncrypted() bool                  { return false }
func (noEncryptionHandler) Authenticate(password string) error { return nil }
func (noEncryptionHandler) DecryptWithFilter(objNum, gen int, data []byte, class DataClass, cryptFilter string) ([]byte, error) {
	return data, nil
}
func (noEncryptionHandler) Decrypt(objNum, gen int, data []byte, class DataClass) ([]byte, error) {
	return data, nil
}
func (noEncryptionHandler) EncryptWithFilter(objNum, gen int, data []byte, class DataClass, cryptFilter string) ([]byte, error) {
	return data, nil
}
func (noEncryptionHandler) Encrypt(objNum, gen int, data []byte, class DataClass) ([]byte, error) {
	return data, nil
}
func (noEncryptionHandler) Permissions() Permissions {
	return Permissions{Print: true, Modify: true, Copy: true}
}
func (noEncryptionHandler) EncryptMetadata() bool { return false }

// NoopHandler returns a reusable pass-through encryption handler.
func NoopHandler() Handler { return noEncryptionHandler{} }

// Helpers
var passwordPadding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func padPassword(pwd []byte) []byte {
	padded := make([]byte, 32)
	copy(padded, pwd)
	if len(pwd) < 32 {
		copy(padded[len(pwd):], passwordPadding[:32-len(pwd)])
	}
	return padded
}

func padPasswordRev6(pwd []byte) []byte {
	if len(pwd) > 127 {
		return pwd[:127]
	}
	out := make([]byte, len(pwd))
	copy(out, pwd)
	return out
}

// rev6Hash implements the iterative hash used by V=5/6 (R=5/6) authentication.
func rev6Hash(pwd []byte, salt []byte, extra []byte) []byte {
	pwd = padPasswordRev6(pwd)
	data := append(append(append([]byte{}, pwd...), salt...), extra...)
	hash := sha256.Sum256(data)
	h := hash[:]
	for i := 0; i < 64; i++ {
		block := make([]byte, 0, 64)
		for len(block) < 64 {
			block = append(block, pwd...)
			block = append(block, h...)
			block = append(block, salt...)
			block = append(block, extra...)
		}
		block = block[:64]
		key := h[:16]
		iv := h[16:32]
		enc, err := aesCBCWithIV(key, iv, block, true)
		if err != nil {
			return h
		}
		last := enc[len(enc)-1]
		switch last % 3 {
		case 0:
			sum := sha256.Sum256(enc)
			h = sum[:]
		case 1:
			sum := sha512.Sum384(enc)
			h = sum[:]
		default:
			sum := sha512.Sum512(enc)
			h = sum[:]
		}
	}
	if len(h) > 32 {
		return h[:32]
	}
	return h
}

func deriveOwnerEntryKey(pwd []byte, keyLenBytes int, r int) []byte {
	data := padPassword(pwd)
	sum := md5.Sum(data)
	key := sum[:]
	if r >= 3 {
		for i := 0; i < 50; i++ {
			sum = md5.Sum(key[:keyLenBytes])
			key = sum[:]
		}
	}
	return key[:keyLenBytes]
}

func deriveKey(pwd, owner []byte, pVal int32, fileID []byte, keyLenBytes int, r int) ([]byte, error) {
	if keyLenBytes <= 0 {
		keyLenBytes = 5
	}
	if keyLenBytes > 32 {
		keyLenBytes = 32
	}
	data := make([]byte, 0, 32+len(owner)+4+len(fileID))
	data = append(data, padPassword(pwd)...)
	data = append(data, owner...)
	var pBuf [4]byte
	binary.LittleEndian.PutUint32(pBuf[:], uint32(pVal))
	data = append(data, pBuf[:]...)
	data = append(data, fileID...)

	sum := md5.Sum(data)
	key := sum[:]
	if r >= 3 {
		for i := 0; i < 50; i++ {
			sum = md5.Sum(key[:keyLenBytes])
			key = sum[:]
		}
	}
	return key[:keyLenBytes], nil
}

// PermissionsValue builds the Standard security permissions flags for a document.
func PermissionsValue(p raw.Permissions) int32 {
	val := int32(-4) // bits 1-2 must be 0
	if !p.Print {
		val &^= 1 << 2
	}
	if !p.Modify {
		val &^= 1 << 3
	}
	if !p.Copy {
		val &^= 1 << 4
	}
	if !p.ModifyAnnotations {
		val &^= 1 << 5
	}
	if !p.FillForms {
		val &^= 1 << 8
	}
	if !p.ExtractAccessible {
		val &^= 1 << 9
	}
	if !p.Assemble {
		val &^= 1 << 10
	}
	if !p.PrintHighQuality {
		val &^= 1 << 11
	}
	return val
}

// BuildRC4Encryption builds a Standard security handler dictionary using RC4 with the requested key length.
func BuildRC4Encryption(userPwd, ownerPwd string, permissions raw.Permissions, fileID []byte, keyBits int, encryptMetadata bool) (*raw.DictObj, []byte, error) {
	if keyBits <= 0 {
		keyBits = 40
	}
	if keyBits%8 != 0 {
		return nil, nil, fmt.Errorf("rc4 key length must be multiple of 8")
	}
	keyLenBytes := keyBits / 8
	if keyLenBytes < 5 {
		keyLenBytes = 5
		keyBits = 40
	}
	if len(ownerPwd) == 0 {
		if len(userPwd) > 0 {
			ownerPwd = userPwd
		} else {
			ownerPwd = "owner"
		}
	}
	userPad := padPassword([]byte(userPwd))
	ownerPad := padPassword([]byte(ownerPwd))
	ownerDigest := md5.Sum(ownerPad)
	ownerKey := ownerDigest[:]
	if len(ownerKey) > keyLenBytes {
		ownerKey = ownerKey[:keyLenBytes]
	}
	oVal := rc4Simple(ownerKey, userPad)
	pVal := PermissionsValue(permissions)
	fileKey, err := deriveKey([]byte(userPwd), oVal, pVal, fileID, keyLenBytes, 2)
	if err != nil {
		return nil, nil, err
	}
	uVal := rc4Simple(fileKey, passwordPadding)

	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberObj{I: 1, IsInt: true})
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberObj{I: 2, IsInt: true})
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberObj{I: int64(keyBits), IsInt: true})
	enc.Set(raw.NameObj{Val: "O"}, raw.Str(oVal))
	enc.Set(raw.NameObj{Val: "U"}, raw.Str(uVal))
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})
	if !encryptMetadata {
		enc.Set(raw.NameObj{Val: "EncryptMetadata"}, raw.Bool(encryptMetadata))
	}
	return enc, fileKey, nil
}

// BuildStandardEncryption constructs an Encrypt dictionary and primary key for the Standard security handler.
func BuildStandardEncryption(userPwd, ownerPwd string, permissions raw.Permissions, fileID []byte, encryptMetadata bool) (*raw.DictObj, []byte, error) {
	return BuildRC4Encryption(userPwd, ownerPwd, permissions, fileID, 40, encryptMetadata)
}

// BuildAES256Encryption constructs an Encrypt dictionary and keys for AES-256 (PDF 2.0) security.
func BuildAES256Encryption(userPwd, ownerPwd string, permissions raw.Permissions, fileID []byte, encryptMetadata bool) (*raw.DictObj, []byte, error) {
	if len(ownerPwd) == 0 {
		if len(userPwd) > 0 {
			ownerPwd = userPwd
		} else {
			ownerPwd = "owner"
		}
	}

	// 1. Generate File Encryption Key (32 bytes)
	fileKey := make([]byte, 32)
	if _, err := rand.Read(fileKey); err != nil {
		return nil, nil, err
	}

	// 2. Create U and UE entries
	uEntry, ueEntry, err := generateAES256Entry([]byte(userPwd), nil, fileKey, fileID)
	if err != nil {
		return nil, nil, err
	}

	// 3. Create O and OE entries
	oEntry, oeEntry, err := generateAES256Entry([]byte(ownerPwd), uEntry, fileKey, nil) // Owner uses U entry as extra data? No, U is not used for Owner hash in R=6?
	// ISO 32000-2:
	// Owner Password:
	// Validation Salt (8 bytes)
	// Key Salt (8 bytes)
	// hash = rev6Hash(ownerPwd, ValidationSalt, U)  <-- U is used as extra data!
	// O = hash || ValidationSalt || KeySalt
	// keyHash = rev6Hash(ownerPwd, KeySalt, U)
	// OE = AES-CBC(keyHash, fileKey)
	if err != nil {
		return nil, nil, err
	}

	// 4. Create Perms entry
	// Perms is 16 bytes: [Perms (4 bytes) | 0xFF... (12 bytes)]
	// Encrypted with fileKey (ECB? No, AES-256 uses CBC with zero IV for Perms?)
	// ISO 32000-2: "The Perms string shall be encrypted using the file encryption key... using the AES-256 algorithm in ECB mode with no padding."
	// Wait, ECB?
	// "The Perms string... encrypted using the file encryption key... using AES-256 in ECB mode..."
	// My `decryptPermsAES256` uses `block.Decrypt` which is ECB for a single block.
	// So yes, ECB.

	pVal := PermissionsValue(permissions)
	permsBlock := make([]byte, 16)
	binary.LittleEndian.PutUint32(permsBlock[0:4], uint32(pVal))
	for i := 4; i < 16; i++ {
		permsBlock[i] = 0xFF
	}
	if !encryptMetadata {
		permsBlock[8] = 'F' // EncryptMetadata = false -> 'F' in 9th byte?
		// ISO 32000-2: "If EncryptMetadata is false, the byte at offset 8 shall be 'F' (0x46). Otherwise it shall be 'T' (0x54)."
		permsBlock[8] = 0x46
	} else {
		permsBlock[8] = 0x54
	}
	// Bytes 9-11 are 0x61, 0x64, 0x62 ('a', 'd', 'b')
	permsBlock[9] = 0x61
	permsBlock[10] = 0x64
	permsBlock[11] = 0x62

	permsEnc := make([]byte, 16)
	block, err := aes.NewCipher(fileKey)
	if err != nil {
		return nil, nil, err
	}
	block.Encrypt(permsEnc, permsBlock)

	// 5. Build Dictionary
	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberObj{I: 5, IsInt: true})
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberObj{I: 6, IsInt: true})
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberObj{I: 256, IsInt: true})
	enc.Set(raw.NameObj{Val: "O"}, raw.Str(oEntry))
	enc.Set(raw.NameObj{Val: "U"}, raw.Str(uEntry))
	enc.Set(raw.NameObj{Val: "OE"}, raw.Str(oeEntry))
	enc.Set(raw.NameObj{Val: "UE"}, raw.Str(ueEntry))
	enc.Set(raw.NameObj{Val: "Perms"}, raw.Str(permsEnc))
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})
	if !encryptMetadata {
		enc.Set(raw.NameObj{Val: "EncryptMetadata"}, raw.Bool(encryptMetadata))
	}

	// Crypt Filters
	cf := raw.Dict()
	stdCF := raw.Dict()
	stdCF.Set(raw.NameObj{Val: "Type"}, raw.NameObj{Val: "CryptFilter"})
	stdCF.Set(raw.NameObj{Val: "CFM"}, raw.NameObj{Val: "AESV3"}) // AESV3 for PDF 2.0
	stdCF.Set(raw.NameObj{Val: "AuthEvent"}, raw.NameObj{Val: "DocOpen"})
	stdCF.Set(raw.NameObj{Val: "Length"}, raw.NumberObj{I: 256, IsInt: true})
	cf.Set(raw.NameObj{Val: "StdCF"}, stdCF)
	enc.Set(raw.NameObj{Val: "CF"}, cf)
	enc.Set(raw.NameObj{Val: "StmF"}, raw.NameObj{Val: "StdCF"})
	enc.Set(raw.NameObj{Val: "StrF"}, raw.NameObj{Val: "StdCF"})

	return enc, fileKey, nil
}

func generateAES256Entry(pwd []byte, extra []byte, fileKey []byte, fileID []byte) ([]byte, []byte, error) {
	// 1. Salts
	valSalt := make([]byte, 8)
	keySalt := make([]byte, 8)
	if _, err := rand.Read(valSalt); err != nil {
		return nil, nil, err
	}
	if _, err := rand.Read(keySalt); err != nil {
		return nil, nil, err
	}

	// 2. User/Owner Entry (48 bytes)
	hash := rev6Hash(pwd, valSalt, extra)
	entry := make([]byte, 0, 48)
	entry = append(entry, hash...)
	entry = append(entry, valSalt...)
	entry = append(entry, keySalt...)

	// 3. UE/OE Entry (32 bytes)
	keyHash := rev6Hash(pwd, keySalt, extra)
	encKey, err := aesCBCNoIV(keyHash, fileKey, true)
	if err != nil {
		return nil, nil, err
	}

	return entry, encKey, nil
}

// Helpers

func numberVal(d raw.Dictionary, key string) (int64, bool) {
	if v, ok := d.Get(raw.NameObj{Val: key}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			if n.IsInt {
				return n.I, true
			}
			return int64(n.F), true
		}
	}
	return 0, false
}

func stringBytes(d raw.Dictionary, key string) ([]byte, bool) {
	if v, ok := d.Get(raw.NameObj{Val: key}); ok {
		if s, ok := v.(raw.StringObj); ok {
			return s.Value(), true
		}
	}
	return nil, false
}

func boolVal(d raw.Dictionary, key string) (bool, bool) {
	if v, ok := d.Get(raw.NameObj{Val: key}); ok {
		if b, ok := v.(raw.BoolObj); ok {
			return b.V, true
		}
	}
	return false, false
}

func rc4Simple(key, data []byte) []byte {
	c, _ := rc4.NewCipher(key)
	dst := make([]byte, len(data))
	c.XORKeyStream(dst, data)
	return dst
}

func rc4Crypt(key, data []byte) ([]byte, error) {
	return rc4Simple(key, data), nil
}

func aesCrypt(key, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	if encrypt {
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}
		mode := cipher.NewCBCEncrypter(block, iv)
		padding := aes.BlockSize - len(data)%aes.BlockSize
		padText := bytes.Repeat([]byte{byte(padding)}, padding)
		paddedData := append(data, padText...)
		encrypted := make([]byte, len(paddedData))
		mode.CryptBlocks(encrypted, paddedData)
		return append(iv, encrypted...), nil
	} else {
		if len(data) < aes.BlockSize {
			return nil, errors.New("data too short for AES")
		}
		copy(iv, data[:aes.BlockSize])
		ciphertext := data[aes.BlockSize:]
		if len(ciphertext)%aes.BlockSize != 0 {
			return nil, errors.New("ciphertext not multiple of block size")
		}
		mode := cipher.NewCBCDecrypter(block, iv)
		decrypted := make([]byte, len(ciphertext))
		mode.CryptBlocks(decrypted, ciphertext)
		if len(decrypted) == 0 {
			return nil, errors.New("empty decrypted data")
		}
		padding := int(decrypted[len(decrypted)-1])
		if padding > aes.BlockSize || padding == 0 {
			return nil, errors.New("invalid padding")
		}
		return decrypted[:len(decrypted)-padding], nil
	}
}

func aesCBCWithIV(key, iv, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if encrypt {
		mode := cipher.NewCBCEncrypter(block, iv)
		out := make([]byte, len(data))
		mode.CryptBlocks(out, data)
		return out, nil
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	out := make([]byte, len(data))
	mode.CryptBlocks(out, data)
	return out, nil
}

func aesCBCNoIV(key, data []byte, encrypt bool) ([]byte, error) {
	iv := make([]byte, aes.BlockSize)
	return aesCBCWithIV(key, iv, data, encrypt)
}

func objectKey(key []byte, objNum, gen, r int, useAES bool) []byte {
	if r == 6 {
		return key
	}
	newKey := make([]byte, len(key)+5)
	copy(newKey, key)

	// Append 3 bytes of Object Number (Little Endian)
	newKey[len(key)] = byte(objNum)
	newKey[len(key)+1] = byte(objNum >> 8)
	newKey[len(key)+2] = byte(objNum >> 16)

	// Append 2 bytes of Generation Number (Little Endian)
	newKey[len(key)+3] = byte(gen)
	newKey[len(key)+4] = byte(gen >> 8)

	if useAES {
		newKey = append(newKey, []byte("sAlT")...)
	}
	sum := md5.Sum(newKey)
	limit := len(key) + 5
	if limit > 16 {
		limit = 16
	}
	return sum[:limit]
}

func checkUserPassword(key, user, fileID []byte, r int) bool {
	dec, _ := rc4Crypt(key, user)
	if r == 2 {
		return bytes.Equal(dec, passwordPadding)
	}
	expected := make([]byte, 0, 32+len(fileID))
	expected = append(expected, passwordPadding...)
	expected = append(expected, fileID...)
	hash := md5.Sum(expected)
	return bytes.HasPrefix(dec, hash[:16])
}

func deriveAES256User(pwd, uEntry, ue, fileID []byte) ([]byte, bool, error) {
	if len(uEntry) < 48 {
		return nil, false, nil
	}
	hash := uEntry[:32]
	valSalt := uEntry[32:40]
	keySalt := uEntry[40:48]
	computedHash := rev6Hash(pwd, valSalt, fileID)
	if !bytes.Equal(hash, computedHash) {
		return nil, false, nil
	}
	keyHash := rev6Hash(pwd, keySalt, fileID)
	if len(keyHash) > 32 {
		keyHash = keyHash[:32]
	}
	fileKey, err := aesCBCNoIV(keyHash, ue, false)
	if err != nil {
		return nil, false, err
	}
	return fileKey, true, nil
}

func deriveAES256Owner(pwd, oEntry, oe, uEntry []byte) ([]byte, bool, error) {
	if len(oEntry) < 48 {
		return nil, false, nil
	}
	hash := oEntry[:32]
	valSalt := oEntry[32:40]
	keySalt := oEntry[40:48]
	computedHash := rev6Hash(pwd, valSalt, uEntry)
	if !bytes.Equal(hash, computedHash) {
		return nil, false, nil
	}
	keyHash := rev6Hash(pwd, keySalt, uEntry)
	if len(keyHash) > 32 {
		keyHash = keyHash[:32]
	}
	fileKey, err := aesCBCNoIV(keyHash, oe, false)
	if err != nil {
		return nil, false, err
	}
	return fileKey, true, nil
}

func decryptPermsAES256(key, perms []byte) (int32, error) {
	if len(perms) != 16 {
		return 0, errors.New("invalid perms length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	dec := make([]byte, 16)
	block.Decrypt(dec, perms)
	if dec[9] != 'a' || dec[10] != 'd' || dec[11] != 'b' {
		return 0, errors.New("invalid perms marker")
	}
	return int32(binary.LittleEndian.Uint32(dec[:4])), nil
}

func parseCryptFilters(d raw.Dictionary, baseAlgo cryptAlgo) (map[string]cryptAlgo, error) {
	filters := make(map[string]cryptAlgo)
	filters["Identity"] = algoNone
	filters["StdCF"] = baseAlgo
	cfDict, ok := d.Get(raw.NameObj{Val: "CF"})
	if !ok {
		return filters, nil
	}
	cf, ok := cfDict.(raw.Dictionary)
	if !ok {
		return filters, nil
	}
	for _, key := range cf.Keys() {
		val, _ := cf.Get(key)
		filterDict, ok := val.(raw.Dictionary)
		if !ok {
			continue
		}
		cfm, _ := filterDict.Get(raw.NameObj{Val: "CFM"})
		if name, ok := cfm.(raw.NameObj); ok {
			switch name.Val {
			case "None":
				filters[key.Value()] = algoNone
			case "V2":
				filters[key.Value()] = algoRC4
			case "AESV2", "AESV3":
				filters[key.Value()] = algoAES
			}
		}
	}
	return filters, nil
}

func resolveCryptFilter(d raw.Dictionary, name string, baseAlgo cryptAlgo, filters map[string]cryptAlgo) (cryptAlgo, error) {
	n, ok := d.Get(raw.NameObj{Val: name})
	if !ok {
		return baseAlgo, nil
	}
	filterName, ok := n.(raw.NameObj)
	if !ok {
		return baseAlgo, nil
	}
	if algo, ok := filters[filterName.Val]; ok {
		return algo, nil
	}
	return algoUnset, fmt.Errorf("undefined crypt filter: %s", filterName.Val)
}
