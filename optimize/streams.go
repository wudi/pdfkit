package optimize

import (
	"bytes"
	"compress/zlib"
	"context"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func (o *Optimizer) compressStreams(ctx context.Context, doc *semantic.Document) error {
	decodedDoc := doc.Decoded()
	if decodedDoc == nil || decodedDoc.Raw == nil {
		return nil
	}

	for ref, decodedStream := range decodedDoc.Streams {
		// We want to replace the raw stream content with Flate compressed version of decoded data.
		rawObj, ok := decodedDoc.Raw.Objects[ref]
		if !ok {
			continue
		}
		rawStream, ok := rawObj.(*raw.StreamObj)
		if !ok {
			continue
		}

		// Compress decoded data
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		if _, err := w.Write(decodedStream.Data()); err != nil {
			w.Close()
			continue // Skip if compression fails
		}
		w.Close()
		compressedData := b.Bytes()

		// Update raw stream
		rawStream.Data = compressedData
		dict := rawStream.Dictionary()
		dict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("FlateDecode"))
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(compressedData))))

		// Remove DecodeParms as we are re-encoding with standard Flate
		if d, ok := dict.(*raw.DictObj); ok {
			delete(d.KV, "DecodeParms")
			delete(d.KV, "F")
			delete(d.KV, "FDecodeParms")
		}
	}
	return nil
}
