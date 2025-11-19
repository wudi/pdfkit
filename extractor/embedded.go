package extractor

import "pdflib/ir/raw"

// EmbeddedFile captures attached file specs surfaced via the Names tree.
type EmbeddedFile struct {
	Name         string
	Description  string
	Relationship string
	Subtype      string
	Data         []byte
}

// ExtractEmbeddedFiles walks the EmbeddedFiles name tree and decodes associated streams.
func (e *Extractor) ExtractEmbeddedFiles() []EmbeddedFile {
	names := derefDict(e.raw, valueFromDict(e.catalog, "Names"))
	if names == nil {
		return nil
	}
	embedded := derefDict(e.raw, valueFromDict(names, "EmbeddedFiles"))
	if embedded == nil {
		return nil
	}
	entries := derefArray(e.raw, valueFromDict(embedded, "Names"))
	if entries == nil {
		return nil
	}
	var files []EmbeddedFile
	for i := 0; i+1 < len(entries.Items); i += 2 {
		displayName, _ := stringFromObject(entries.Items[i])
		spec := derefDict(e.raw, entries.Items[i+1])
		if spec == nil {
			continue
		}
		file := EmbeddedFile{Name: displayName}
		file.Description, _ = stringFromDict(spec, "Desc")
		file.Relationship, _ = nameFromDict(spec, "AFRelationship")
		file.Subtype, _ = nameFromDict(spec, "Subtype")
		file.Data = extractEmbeddedStream(e, spec)
		files = append(files, file)
	}
	return files
}

func extractEmbeddedStream(e *Extractor, spec *raw.DictObj) []byte {
	ef := derefDict(e.raw, valueFromDict(spec, "EF"))
	if ef == nil {
		if data, _ := e.streamBytes(valueFromDict(spec, "F")); len(data) > 0 {
			return data
		}
		return nil
	}
	if data, _ := e.streamBytes(valueFromDict(ef, "F")); len(data) > 0 {
		return data
	}
	if data, _ := e.streamBytes(valueFromDict(ef, "UF")); len(data) > 0 {
		return data
	}
	return nil
}
