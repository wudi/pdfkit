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

	"pdflib/ir/raw"
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
	key, err := deriveKey([]byte(password), h.owner, h.p, h.fileID, h.lengthBits/8, h.r)
	if err != nil {
		return err
	}
	if !h.useAES && h.r <= 3 {
		if !checkUserPassword(key, h.user, h.fileID, h.r) {
			return errors.New("invalid password")
		}
	}
	h.key = key
	h.authed = true
	return nil
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
	return h
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

// BuildStandardEncryption constructs an Encrypt dictionary and primary key for the Standard security handler.
func BuildStandardEncryption(userPwd, ownerPwd string, permissions raw.Permissions, fileID []byte, encryptMetadata bool) (*raw.DictObj, []byte, error) {
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
	oVal := rc4Simple(ownerDigest[:5], userPad)
	pVal := PermissionsValue(permissions)
	fileKey, err := deriveKey([]byte(userPwd), oVal, pVal, fileID, 5, 2)
	if err != nil {
		return nil, nil, err
	}
	uVal := rc4Simple(fileKey, passwordPadding)

	enc := raw.Dict()
	enc.Set(raw.NameObj{Val: "Filter"}, raw.NameObj{Val: "Standard"})
	enc.Set(raw.NameObj{Val: "V"}, raw.NumberObj{I: 1, IsInt: true})
	enc.Set(raw.NameObj{Val: "R"}, raw.NumberObj{I: 2, IsInt: true})
	enc.Set(raw.NameObj{Val: "Length"}, raw.NumberObj{I: 40, IsInt: true})
	enc.Set(raw.NameObj{Val: "O"}, raw.Str(oVal))
	enc.Set(raw.NameObj{Val: "U"}, raw.Str(uVal))
	enc.Set(raw.NameObj{Val: "P"}, raw.NumberObj{I: int64(pVal), IsInt: true})
	if !encryptMetadata {
		enc.Set(raw.NameObj{Val: "EncryptMetadata"}, raw.Bool(encryptMetadata))
	}
	return enc, fileKey, nil
}

func checkUserPassword(key []byte, userEntry []byte, fileID []byte, r int) bool {
	if r <= 2 {
		expect := rc4Simple(key, passwordPadding)
		if len(userEntry) >= 16 {
			exp16 := expect[:16]
			act := userEntry[:16]
			if string(exp16) == string(act) {
				return true
			}
		}
		return false
	}
	// R>=3 (still RC4 for V<=2) simplified check: compute value per spec.
	h := md5.Sum(append(passwordPadding, fileID...))
	val := h[:]
	for i := 0; i < 20; i++ {
		tmpKey := make([]byte, len(key))
		for j := 0; j < len(key); j++ {
			tmpKey[j] = key[j] ^ byte(i)
		}
		val = rc4Simple(tmpKey, val)
	}
	return len(userEntry) >= 16 && comparePrefix(val[:16], userEntry[:16])
}

// AES-256 (R>=5) derivation for user password per ISO 32000-2 (simplified to SHA-256 hash).
func deriveAES256User(pwd []byte, uEntry []byte, ue []byte, fileID []byte) ([]byte, bool, error) {
	if len(uEntry) < 48 || len(ue) < 16 {
		return nil, false, errors.New("user entry too short")
	}
	validationSalt := uEntry[32:40]
	keySalt := uEntry[40:48]
	hashVal := rev6Hash(pwd, validationSalt, fileID)
	if !comparePrefix(hashVal[:32], uEntry[:32]) {
		return nil, false, nil
	}
	keyHash := rev6Hash(pwd, keySalt, fileID)
	fileKey, err := aesCBCNoIV(keyHash[:32], ue, false)
	if err != nil {
		return nil, false, err
	}
	return fileKey, true, nil
}

// AES-256 owner derivation uses U entry as additional data.
func deriveAES256Owner(pwd []byte, oEntry []byte, oe []byte, uEntry []byte) ([]byte, bool, error) {
	if len(oEntry) < 48 || len(oe) < 16 || len(uEntry) < 48 {
		return nil, false, errors.New("owner entry too short")
	}
	validationSalt := oEntry[32:40]
	keySalt := oEntry[40:48]
	hashVal := rev6Hash(pwd, validationSalt, uEntry[:48])
	if !comparePrefix(hashVal[:32], oEntry[:32]) {
		return nil, false, nil
	}
	keyHash := rev6Hash(pwd, keySalt, uEntry[:48])
	fileKey, err := aesCBCNoIV(keyHash[:32], oe, false)
	if err != nil {
		return nil, false, err
	}
	return fileKey, true, nil
}

func parseCryptFilters(dict raw.Dictionary, base cryptAlgo) (map[string]cryptAlgo, error) {
	out := make(map[string]cryptAlgo)
	if dict == nil {
		return out, nil
	}
	cfObj, ok := dict.Get(raw.NameObj{Val: "CF"})
	if !ok {
		return out, nil
	}
	cfDict, ok := cfObj.(*raw.DictObj)
	if !ok {
		return nil, errors.New("CF must be a dictionary")
	}
	for name, obj := range cfDict.KV {
		entry, ok := obj.(*raw.DictObj)
		if !ok {
			return nil, errors.New("crypt filter entry must be a dictionary")
		}
		algo := base
		if cfmObj, ok := entry.Get(raw.NameObj{Val: "CFM"}); ok {
			if cfmName, ok := cfmObj.(raw.NameObj); ok {
				switch cfmName.Val {
				case "V2":
					algo = algoRC4
				case "AESV2":
					algo = algoAES
				case "None":
					algo = algoNone
				default:
					return nil, fmt.Errorf("unsupported crypt filter method %s", cfmName.Val)
				}
			}
		}
		out[name] = algo
	}
	return out, nil
}

func resolveCryptFilter(dict raw.Dictionary, key string, base cryptAlgo, filters map[string]cryptAlgo) (cryptAlgo, error) {
	name := nameVal(dict, key)
	if name == "" || name == "Standard" {
		if algo, ok := filters["Standard"]; ok {
			return algo, nil
		}
		return base, nil
	}
	if name == "Identity" {
		return algoNone, nil
	}
	if algo, ok := filters[name]; ok {
		return algo, nil
	}
	return algoUnset, fmt.Errorf("crypt filter %s not defined", name)
}

func objectKey(fileKey []byte, objNum, gen int, r int, useAES bool) []byte {
	if r >= 5 {
		return fileKey
	}
	key := append([]byte{}, fileKey...)
	var objBuf [3]byte
	objBuf[0] = byte(objNum & 0xFF)
	objBuf[1] = byte((objNum >> 8) & 0xFF)
	objBuf[2] = byte((objNum >> 16) & 0xFF)
	key = append(key, objBuf[:]...)
	var genBuf [2]byte
	genBuf[0] = byte(gen & 0xFF)
	genBuf[1] = byte((gen >> 8) & 0xFF)
	key = append(key, genBuf[:]...)
	if useAES {
		key = append(key, 0x73, 0x41, 0x6C, 0x54) // "sAlT"
	}
	hashLen := len(fileKey) + 5
	if hashLen > 16 {
		hashLen = 16
	}
	hash := md5.Sum(key)
	out := hash[:]
	if useAES && hashLen > 16 {
		hashLen = 16
	}
	if hashLen < len(out) {
		out = out[:hashLen]
	}
	return out
}

func rc4Simple(key []byte, data []byte) []byte {
	out := make([]byte, len(data))
	c, _ := rc4.NewCipher(key)
	c.XORKeyStream(out, data)
	return out
}

func rc4Crypt(key []byte, data []byte) ([]byte, error) {
	c, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out, nil
}

func aesCrypt(key []byte, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if encrypt {
		iv := make([]byte, aes.BlockSize)
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}
		padLen := aes.BlockSize - (len(data) % aes.BlockSize)
		if padLen == 0 {
			padLen = aes.BlockSize
		}
		pad := bytes.Repeat([]byte{byte(padLen)}, padLen)
		plain := append(data, pad...)
		out := make([]byte, aes.BlockSize+len(plain))
		copy(out[:aes.BlockSize], iv)
		mode := cipher.NewCBCEncrypter(block, iv)
		mode.CryptBlocks(out[aes.BlockSize:], plain)
		return out, nil
	}
	if len(data) < aes.BlockSize {
		return nil, errors.New("aes ciphertext too short")
	}
	iv := data[:aes.BlockSize]
	ct := data[aes.BlockSize:]
	if len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("aes ciphertext not multiple of blocksize")
	}
	out := make([]byte, len(ct))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(out, ct)
	if len(out) == 0 {
		return out, nil
	}
	pad := int(out[len(out)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(out) {
		return nil, errors.New("invalid aes padding")
	}
	return out[:len(out)-pad], nil
}

func aesCBCNoIV(key []byte, data []byte, encrypt bool) ([]byte, error) {
	iv := make([]byte, aes.BlockSize)
	return aesCBCWithIV(key, iv, data, encrypt)
}

func aesCBCWithIV(key []byte, iv []byte, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize {
		return nil, errors.New("invalid iv size")
	}
	if encrypt {
		padLen := aes.BlockSize - (len(data) % aes.BlockSize)
		if padLen == 0 {
			padLen = aes.BlockSize
		}
		pad := bytes.Repeat([]byte{byte(padLen)}, padLen)
		plain := append(data, pad...)
		out := make([]byte, len(plain))
		mode := cipher.NewCBCEncrypter(block, iv)
		mode.CryptBlocks(out, plain)
		return out, nil
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.New("aes data not multiple of blocksize")
	}
	out := make([]byte, len(data))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(out, data)
	if len(out) == 0 {
		return out, nil
	}
	pad := int(out[len(out)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(out) {
		return nil, errors.New("invalid aes padding")
	}
	return out[:len(out)-pad], nil
}

func decryptPermsAES256(key []byte, perms []byte) (int32, error) {
	if len(key) == 0 {
		return 0, errors.New("missing key")
	}
	if len(perms) != 16 {
		return 0, errors.New("perms length must be 16")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	out := make([]byte, 16)
	block.Decrypt(out, perms)
	if !comparePrefix([]byte("perm"), out[12:16]) {
		return 0, errors.New("invalid perms signature")
	}
	p := int32(binary.LittleEndian.Uint32(out[0:4]))
	return p, nil
}

func comparePrefix(a, b []byte) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func numberVal(dict raw.Dictionary, key string) (int64, bool) {
	if dict == nil {
		return 0, false
	}
	if v, ok := dict.Get(raw.NameObj{Val: key}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			return n.Int(), true
		}
	}
	return 0, false
}

func stringBytes(dict raw.Dictionary, key string) ([]byte, bool) {
	if dict == nil {
		return nil, false
	}
	if v, ok := dict.Get(raw.NameObj{Val: key}); ok {
		if s, ok := v.(raw.StringObj); ok {
			return s.Value(), true
		}
	}
	return nil, false
}

func boolVal(dict raw.Dictionary, key string) (bool, bool) {
	if dict == nil {
		return false, false
	}
	if v, ok := dict.Get(raw.NameObj{Val: key}); ok {
		if b, ok := v.(raw.BoolObj); ok {
			return b.V, true
		}
	}
	return false, false
}

func nameVal(dict raw.Dictionary, key string) string {
	if dict == nil {
		return ""
	}
	if v, ok := dict.Get(raw.NameObj{Val: key}); ok {
		if n, ok := v.(raw.NameObj); ok {
			return n.Val
		}
	}
	return ""
}
