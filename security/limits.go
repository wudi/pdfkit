package security

import "time"

// Limits defines security boundaries for parsing and processing PDFs.
// These limits help prevent resource exhaustion attacks (e.g., zip bombs, stack overflows).
type Limits struct {
	// Maximum decompressed stream size (prevent zip bombs). Default: 100 MB.
	MaxDecompressedSize int64

	// Maximum indirect reference depth (prevent stack overflow). Default: 100.
	MaxIndirectDepth int

	// Maximum XRef chain depth (Prev entries). Default: 50.
	MaxXRefDepth int

	// Maximum XObject nesting depth. Default: 20.
	MaxXObjectDepth int

	// Maximum array size (number of elements). Default: 100,000.
	MaxArraySize int

	// Maximum dictionary size (number of entries). Default: 10,000.
	MaxDictSize int

	// Maximum string length (bytes). Default: 10 MB.
	MaxStringLength int64

	// Maximum raw stream length (bytes). Default: 50 MB.
	MaxStreamLength int64

	// Maximum decode time per stream. Default: 30s.
	MaxDecodeTime time.Duration

	// Maximum total parse time. Default: 5m.
	MaxParseTime time.Duration
}

// DefaultLimits returns a Limits struct with safe default values.
func DefaultLimits() Limits {
	return Limits{
		MaxDecompressedSize: 100 * 1024 * 1024, // 100 MB
		MaxIndirectDepth:    100,
		MaxXRefDepth:        50,
		MaxXObjectDepth:     20,
		MaxArraySize:        100000,
		MaxDictSize:         10000,
		MaxStringLength:     10 * 1024 * 1024, // 10 MB
		MaxStreamLength:     50 * 1024 * 1024, // 50 MB
		MaxDecodeTime:       30 * time.Second,
		MaxParseTime:        5 * time.Minute,
	}
}
