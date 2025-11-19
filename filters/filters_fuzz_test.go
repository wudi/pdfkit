package filters

import (
	"context"
	"pdflib/ir/raw"
	"testing"
)

func FuzzFilters(f *testing.F) {
	f.Add([]byte("some compressed data"), "FlateDecode")
	f.Add([]byte("some ascii85 data"), "ASCII85Decode")
	f.Add([]byte("some hex data"), "ASCIIHexDecode")

	f.Fuzz(func(t *testing.T, data []byte, filterName string) {
		// Limit filterName to known filters to avoid "unknown filter" errors spamming
		known := map[string]bool{
			"FlateDecode":     true,
			"ASCII85Decode":   true,
			"ASCIIHexDecode":  true,
			"RunLengthDecode": true,
			"LZWDecode":       true,
		}
		if !known[filterName] {
			return
		}

		p := NewPipeline([]Decoder{
			NewFlateDecoder(),
			NewASCII85Decoder(),
			NewASCIIHexDecoder(),
			NewRunLengthDecoder(),
			NewLZWDecoder(),
		}, Limits{MaxDecompressedSize: 1024 * 1024, MaxDecodeTime: 0})

		// We fuzz with empty params for now
		_, _ = p.Decode(context.Background(), data, []string{filterName}, []raw.Dictionary{nil})
	})
}
