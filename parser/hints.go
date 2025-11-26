package parser

import (
	"errors"
	"io"

	"github.com/wudi/pdfkit/ir/raw"
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

	// Header (Table F.4)
	minObjsInPage, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	firstPageLoc, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	bitsDeltaObjs, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	minPageLen, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	bitsDeltaPageLen, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	minContentOffset, err := br.ReadBits(32) // Relative to page start
	if err != nil {
		return nil, err
	}
	bitsDeltaContentOffset, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	minContentLen, err := br.ReadBits(32)
	if err != nil {
		return nil, err
	}
	bitsDeltaContentLen, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsSharedObjIdx, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsGreatestSharedObjIdx, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsNumSharedObjs, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	bitsFraction, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}

	hints := make([]raw.PageOffsetHint, npages)

	currentPageStart := int64(firstPageLoc)

	for i := 0; i < npages; i++ {
		// 1. Number of objects in page
		deltaObjs, err := br.ReadBits(bitsDeltaObjs)
		if err != nil {
			return nil, err
		}
		nObjs := minObjsInPage + deltaObjs

		// 2. Page length
		deltaPageLen, err := br.ReadBits(bitsDeltaPageLen)
		if err != nil {
			return nil, err
		}
		pageLen := minPageLen + deltaPageLen

		// 3. Content stream offset (relative to page start)
		deltaContentOffset, err := br.ReadBits(bitsDeltaContentOffset)
		if err != nil {
			return nil, err
		}
		contentOffsetRel := minContentOffset + deltaContentOffset
		contentOffsetAbs := currentPageStart + int64(contentOffsetRel)

		// 4. Content stream length
		deltaContentLen, err := br.ReadBits(bitsDeltaContentLen)
		if err != nil {
			return nil, err
		}
		contentLen := minContentLen + deltaContentLen

		// 5. Shared object index
		sIdx, err := br.ReadBits(bitsSharedObjIdx)
		if err != nil {
			return nil, err
		}

		// 6. Greatest shared object index
		gsIdx, err := br.ReadBits(bitsGreatestSharedObjIdx)
		if err != nil {
			return nil, err
		}

		// 7. Number of shared objects
		nsObjs, err := br.ReadBits(bitsNumSharedObjs)
		if err != nil {
			return nil, err
		}

		// 8. Fraction
		frac, err := br.ReadBits(bitsFraction)
		if err != nil {
			return nil, err
		}

		hints[i] = raw.PageOffsetHint{
			NumObjects:     nObjs,
			PageLength:     int64(pageLen),
			ContentStream:  contentOffsetAbs,
			ContentLength:  int64(contentLen),
			SharedObjIndex: sIdx,
		}

		// Update page start for next page
		currentPageStart += int64(pageLen)

		_ = gsIdx
		_ = nsObjs
		_ = frac
	}

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
