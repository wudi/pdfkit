package parser

import (
	"errors"
	"io"

	"pdflib/ir/raw"
)

// HintTable and PageOffsetHint are now in ir/raw

// ParseHintStream parses the hint stream data.
// The dict is the dictionary of the hint stream object (containing S, T, O, etc.).
// npages is the number of pages in the document (from Linearization dict).
func ParseHintStream(data []byte, dict *raw.DictObj, npages int) (*raw.HintTable, error) {
	// 1. Get Shared Object Hint Table offset (S)
	sVal, ok := dict.Get(raw.NameObj{Val: "S"})
	if !ok {
		return nil, errors.New("hint stream missing S (shared object offset)")
	}
	sharedOffset := int(toInt64(sVal))

	// 2. Parse Page Offset Hint Table (starts at 0)
	if len(data) < sharedOffset {
		return nil, errors.New("hint stream data too short for shared offset")
	}

	br := &bitReader{data: data}

	// Header
	minObjNum, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	firstPageLoc, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	_ = firstPageLoc
	bitsObjCount, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	minPageLen, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	_ = minPageLen
	bitsPageLen, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsContentOff, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsContentLen, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsSharedIdx, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsGreatestShared, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsNumShared, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsFraction, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}

	// Read per-page entries
	// Note: Item 1 is the first page. Item 2 is the second page, etc.
	// The header values correspond to Item 1 (or are baselines).
	// Actually, the spec says:
	// "Item 1: ... The first entry in the page offset hint table corresponds to the first page of the document."
	// But the first page is usually page 0 in 0-indexed systems, or page 1 in 1-indexed.
	// The Linearization dict says "O" is the object number of the first page.

	hints := make([]raw.PageOffsetHint, npages)

	// We need to handle the fact that values are delta-encoded or relative.
	// For simplicity, we'll just read the bits for now.

	for i := 0; i < npages; i++ {
		// 1. Number of objects in page
		nObjs, err := br.ReadBits(bitsObjCount)
		if err != nil {
			return nil, err
		}

		// 2. Page length
		pLen, err := br.ReadBits(bitsPageLen)
		if err != nil {
			return nil, err
		}

		// 3. Content stream offset
		cOff, err := br.ReadBits(bitsContentOff)
		if err != nil {
			return nil, err
		}

		// 4. Content stream length
		cLen, err := br.ReadBits(bitsContentLen)
		if err != nil {
			return nil, err
		}

		// 5. Shared object index
		sIdx, err := br.ReadBits(bitsSharedIdx)
		if err != nil {
			return nil, err
		}

		// 6. Greatest shared object index
		gsIdx, err := br.ReadBits(bitsGreatestShared)
		if err != nil {
			return nil, err
		}

		// 7. Number of shared objects
		nsObjs, err := br.ReadBits(bitsNumShared)
		if err != nil {
			return nil, err
		}

		// 8. Fraction
		frac, err := br.ReadBits(bitsFraction)
		if err != nil {
			return nil, err
		}

		hints[i] = raw.PageOffsetHint{
			MinObjNum:      minObjNum,   // Placeholder
			PageLength:     int64(pLen), // Delta?
			ContentStream:  int64(cOff),
			ContentLength:  int64(cLen),
			SharedObjIndex: sIdx,
		}
		_ = nObjs
		_ = gsIdx
		_ = nsObjs
		_ = frac
	}

	// TODO: Adjust values based on min/base values and deltas.
	// This requires careful reading of the spec (Annex F).
	// For now, we successfully parsed the structure.

	return &raw.HintTable{
		PageOffsets: hints,
	}, nil
}

type bitReader struct {
	data []byte
	pos  int // byte position
	bit  int // bit position (0-7)
}

func (r *bitReader) ReadBits(n int) (int, error) {
	if n > 32 {
		return 0, errors.New("max 32 bits")
	}
	val := 0
	for i := 0; i < n; i++ {
		if r.pos >= len(r.data) {
			return 0, io.EOF
		}
		bit := (r.data[r.pos] >> (7 - r.bit)) & 1
		val = (val << 1) | int(bit)
		r.bit++
		if r.bit == 8 {
			r.bit = 0
			r.pos++
		}
	}
	return val, nil
}

func toInt64(obj raw.Object) int64 {
	switch n := obj.(type) {
	case raw.NumberObj:
		return n.Int()
	default:
		return 0
	}
}
