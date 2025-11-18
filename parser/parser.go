package parser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"pdflib/ir/raw"
	"pdflib/recovery"
	"pdflib/security"
	"pdflib/xref"
)

// Config controls high-level PDF parsing (xref resolution + object loading).
type Config struct {
	Recovery    recovery.Strategy
	XRef        xref.ResolverConfig
	MaxIndirect int
	Security    security.Handler
	Cache       Cache
	Password    string
}

// DocumentParser builds a raw.Document using xref tables/streams and the object loader.
type DocumentParser struct {
	cfg Config
}

func NewDocumentParser(cfg Config) *DocumentParser {
	if cfg.MaxIndirect == 0 {
		cfg.MaxIndirect = 32
	}
	return &DocumentParser{cfg: cfg}
}

// SetPassword updates the password for decryption when parsing encrypted PDFs.
func (p *DocumentParser) SetPassword(pwd string) {
	p.cfg.Password = pwd
}

func (p *DocumentParser) Parse(ctx context.Context, r io.ReaderAt) (*raw.Document, error) {
	resolver := xref.NewResolver(p.cfg.XRef)
	table, err := resolver.Resolve(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("resolve xref: %w", err)
	}

	sec, err := p.selectSecurity(ctx, r, table, resolver.Trailer())
	if err != nil {
		return nil, fmt.Errorf("security setup: %w", err)
	}

	builder := &ObjectLoaderBuilder{
		reader:    r,
		xrefTable: table,
		security:  sec,
		maxDepth:  p.cfg.MaxIndirect,
		cache:     p.cfg.Cache,
		recovery:  p.cfg.Recovery,
	}
	loader, err := builder.Build()
	if err != nil {
		return nil, err
	}

	doc := &raw.Document{
		Objects:           make(map[raw.ObjectRef]raw.Object),
		Trailer:           resolver.Trailer(),
		Version:           detectHeaderVersion(r),
		Permissions:       toRawPermissions(sec.Permissions()),
		MetadataEncrypted: encryptsMetadata(sec),
		Encrypted:         sec.IsEncrypted(),
	}

	for _, objNum := range table.Objects() {
		if objNum == 0 {
			continue // free head entry
		}
		_, gen, found := table.Lookup(objNum)
		if !found {
			continue
		}
		ref := raw.ObjectRef{Num: objNum, Gen: gen}
		obj, err := loader.Load(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("load object %d: %w", objNum, err)
		}
		doc.Objects[ref] = obj
	}

	if doc.Trailer != nil {
		p.populateMetadata(ctx, loader, doc)
	}
	return doc, nil
}

func (p *DocumentParser) selectSecurity(ctx context.Context, r io.ReaderAt, table xref.Table, trailer raw.Dictionary) (security.Handler, error) {
	if p.cfg.Security != nil {
		return p.cfg.Security, nil
	}
	trailerDict, _ := trailer.(*raw.DictObj)
	if trailerDict == nil {
		return security.NoopHandler(), nil
	}
	encObj, ok := trailerDict.Get(raw.NameObj{Val: "Encrypt"})
	if !ok {
		return security.NoopHandler(), nil
	}
	var encDict *raw.DictObj
	switch v := encObj.(type) {
	case *raw.DictObj:
		encDict = v
	case raw.RefObj:
		loader, err := (&ObjectLoaderBuilder{
			reader:    r,
			xrefTable: table,
			security:  security.NoopHandler(),
			maxDepth:  p.cfg.MaxIndirect,
			cache:     p.cfg.Cache,
			recovery:  p.cfg.Recovery,
		}).Build()
		if err != nil {
			return nil, err
		}
		obj, err := loader.Load(ctx, v.R)
		if err == nil {
			encDict, _ = obj.(*raw.DictObj)
		}
	}
	if encDict == nil {
		return security.NoopHandler(), nil
	}
	fileID := fileIDFromTrailer(trailerDict)
	handler, err := (&security.HandlerBuilder{}).WithEncryptDict(encDict).WithTrailer(trailerDict).WithFileID(fileID).Build()
	if err != nil {
		return nil, err
	}
	if err := handler.Authenticate(p.cfg.Password); err != nil {
		return nil, err
	}
	return handler, nil
}

func fileIDFromTrailer(trailer *raw.DictObj) []byte {
	if trailer == nil {
		return nil
	}
	if idObj, ok := trailer.Get(raw.NameObj{Val: "ID"}); ok {
		if arr, ok := idObj.(*raw.ArrayObj); ok && arr.Len() > 0 {
			if s, ok := arr.Items[0].(raw.StringObj); ok {
				return s.Value()
			}
			if hx, ok := arr.Items[0].(raw.HexStringObj); ok {
				return hx.Value()
			}
		}
	}
	return nil
}

func (p *DocumentParser) populateMetadata(ctx context.Context, loader ObjectLoader, doc *raw.Document) {
	infoObj, ok := doc.Trailer.Get(raw.NameObj{Val: "Info"})
	if !ok {
		return
	}
	ref, ok := infoObj.(raw.RefObj)
	if !ok {
		return
	}
	info, err := loader.Load(ctx, ref.R)
	if err != nil {
		return
	}
	dict, ok := info.(*raw.DictObj)
	if !ok {
		return
	}
	md := raw.DocumentMetadata{}
	if v, ok := stringValue(dict, "Title"); ok {
		md.Title = v
	}
	if v, ok := stringValue(dict, "Author"); ok {
		md.Author = v
	}
	if v, ok := stringValue(dict, "Creator"); ok {
		md.Creator = v
	}
	if v, ok := stringValue(dict, "Producer"); ok {
		md.Producer = v
	}
	if v, ok := stringValue(dict, "Subject"); ok {
		md.Subject = v
	}
	if v, ok := stringValue(dict, "Keywords"); ok {
		md.Keywords = strings.Split(v, ",")
	}
	doc.Metadata = md
}

func stringValue(dict *raw.DictObj, key string) (string, bool) {
	obj, ok := dict.Get(raw.NameObj{Val: key})
	if !ok {
		return "", false
	}
	str, ok := obj.(raw.StringObj)
	if !ok {
		return "", false
	}
	return string(str.Value()), true
}

func encryptsMetadata(h security.Handler) bool {
	if h == nil {
		return false
	}
	return h.EncryptMetadata()
}

func toRawPermissions(p security.Permissions) raw.Permissions {
	return raw.Permissions{
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

func detectHeaderVersion(r io.ReaderAt) string {
	buf := make([]byte, 64)
	n, err := r.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	line := string(buf[:n])
	for _, sep := range []string{"\r\n", "\n", "\r"} {
		if idx := strings.Index(line, sep); idx >= 0 {
			line = line[:idx]
			break
		}
	}
	if strings.HasPrefix(line, "%PDF-") && len(line) >= 8 {
		return strings.TrimSpace(line[5:])
	}
	return ""
}
