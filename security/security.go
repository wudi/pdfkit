package security

import "pdflib/ir/raw"

type Permissions struct { Print, Modify, Copy, ModifyAnnotations, FillForms, ExtractAccessible, Assemble, PrintHighQuality bool }

type Handler interface { IsEncrypted() bool; Authenticate(password string) error; Decrypt(objNum, gen int, data []byte) ([]byte, error); Encrypt(objNum, gen int, data []byte) ([]byte, error); Permissions() Permissions }

type HandlerBuilder struct { encryptDict raw.Dictionary; trailer raw.Dictionary; fileID []byte }

func (b *HandlerBuilder) WithEncryptDict(d raw.Dictionary) *HandlerBuilder { b.encryptDict = d; return b }
func (b *HandlerBuilder) Build() (Handler, error) { if b.encryptDict == nil { return noEncryptionHandler{}, nil }; return noEncryptionHandler{}, nil }

type noEncryptionHandler struct{}
func (noEncryptionHandler) IsEncrypted() bool { return false }
func (noEncryptionHandler) Authenticate(password string) error { return nil }
func (noEncryptionHandler) Decrypt(objNum, gen int, data []byte) ([]byte, error) { return data, nil }
func (noEncryptionHandler) Encrypt(objNum, gen int, data []byte) ([]byte, error) { return data, nil }
func (noEncryptionHandler) Permissions() Permissions { return Permissions{ Print:true, Modify:true, Copy:true } }

// NoopHandler returns a reusable pass-through encryption handler.
func NoopHandler() Handler { return noEncryptionHandler{} }
