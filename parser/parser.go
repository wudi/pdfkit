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

func (p *DocumentParser) Parse(ctx context.Context, r io.ReaderAt) (*raw.Document, error) {
	resolver := xref.NewResolver(p.cfg.XRef)
	table, err := resolver.Resolve(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("resolve xref: %w", err)
	}

	builder := &ObjectLoaderBuilder{
		reader:    r,
		xrefTable: table,
		security:  p.cfg.Security,
		maxDepth:  p.cfg.MaxIndirect,
		cache:     p.cfg.Cache,
		recovery:  p.cfg.Recovery,
	}
	loader, err := builder.Build()
	if err != nil {
		return nil, err
	}

	doc := &raw.Document{
		Objects: make(map[raw.ObjectRef]raw.Object),
		Trailer: resolver.Trailer(),
		Version: detectHeaderVersion(r),
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
