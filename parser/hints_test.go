package parser

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
)

func TestParseHintStreamHeader(t *testing.T) {
	// Construct a fake hint stream
	// Header is 11 fields.
	// 32, 32, 16, 32, 16, 16, 16, 16, 16, 16, 16
	// Total bits: 64 + 16 + 32 + 16*7 = 112 + 112 = 224 bits = 28 bytes.

	data := make([]byte, 100)
	// Set S (shared offset) to 50
	dict := raw.Dict()
	dict.Set(raw.NameObj{Val: "S"}, raw.NumberObj{I: 50, IsInt: true})

	// We leave data as zeros, so all values read as 0.
	// This is enough to test that it doesn't crash and reads the header.

	ht, err := ParseHintStream(data, dict, 1)
	if err != nil {
		t.Fatalf("ParseHintStream failed: %v", err)
	}

	if len(ht.PageOffsets) == 0 {
		t.Errorf("expected at least one page offset hint")
	}
}

func TestParseHintStream_PageOffsetHint(t *testing.T) {
	// Construct a fake hint stream with one page
	// Header (224 bits) + Page Offset Hint Table (variable)
	// We need to construct valid bitstream for the header to pass validation.

	// Header values:
	// minObjsInPage: 1 (32)
	// firstPageLoc: 0 (32)
	// bitsDeltaObjs: 4 (16)
	// minPageLen: 100 (32)
	// bitsDeltaPageLen: 0 (16)
	// minContentOffset: 0 (32)
	// bitsDeltaContentOffset: 0 (16)
	// minContentLen: 50 (32)
	// bitsDeltaContentLen: 0 (16)
	// bitsSharedObjIdx: 0 (16)
	// bitsGreatestSharedObjIdx: 0 (16)
	// bitsNumSharedObjs: 0 (16)
	// bitsFraction: 0 (16)

	// Page Entry (1 page):
	// deltaObjs: 1 (4 bits) -> nObjs = 1 + 1 = 2
	// deltaPageLen: 0 (0 bits)
	// deltaContentOffset: 0 (0 bits)
	// deltaContentLen: 0 (0 bits)
	// sharedObjIdx: 0 (0 bits)
	// greatestSharedObjIdx: 0 (0 bits)
	// numSharedObjs: 0 (0 bits)
	// fraction: 0 (0 bits)

	bw := &bitWriter{}
	bw.WriteBits(1, 32)   // minObjsInPage
	bw.WriteBits(0, 32)   // firstPageLoc
	bw.WriteBits(4, 16)   // bitsDeltaObjs
	bw.WriteBits(100, 32) // minPageLen
	bw.WriteBits(0, 16)   // bitsDeltaPageLen
	bw.WriteBits(0, 32)   // minContentOffset
	bw.WriteBits(0, 16)   // bitsDeltaContentOffset
	bw.WriteBits(50, 32)  // minContentLen
	bw.WriteBits(0, 16)   // bitsDeltaContentLen
	bw.WriteBits(0, 16)   // bitsSharedObjIdx
	bw.WriteBits(0, 16)   // bitsGreatestSharedObjIdx
	bw.WriteBits(0, 16)   // bitsNumSharedObjs
	bw.WriteBits(0, 16)   // bitsFraction

	// Page 1
	bw.WriteBits(1, 4) // deltaObjs = 1

	data := bw.Bytes()

	dict := raw.Dict()
	dict.Set(raw.NameObj{Val: "S"}, raw.NumberObj{I: 1000, IsInt: true}) // S > len(data) is fine for this test as we check length before S

	// We need to mock S check or ensure data is long enough?
	// The parser checks: if len(data) < sharedOffset { return error }
	// So we must set S to something small or make data large.
	// But S is offset of Shared Object Hint Table.
	// If we set S to len(data), it should pass the check.
	dict.Set(raw.NameObj{Val: "S"}, raw.NumberObj{I: int64(len(data)), IsInt: true})

	ht, err := ParseHintStream(data, dict, 1)
	if err != nil {
		t.Fatalf("ParseHintStream failed: %v", err)
	}

	if len(ht.PageOffsets) != 1 {
		t.Fatalf("expected 1 page offset hint, got %d", len(ht.PageOffsets))
	}

	hint := ht.PageOffsets[0]
	if hint.NumObjects != 2 {
		t.Errorf("expected NumObjects = 2 (min 1 + delta 1), got %d", hint.NumObjects)
	}
}

type bitWriter struct {
	data []byte
	bit  int // 0-7
}

func (w *bitWriter) WriteBits(val int, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (val >> i) & 1
		if len(w.data) == 0 || w.bit == 8 {
			w.data = append(w.data, 0)
			w.bit = 0
		}
		if bit == 1 {
			w.data[len(w.data)-1] |= 1 << (7 - w.bit)
		}
		w.bit++
	}
}

func (w *bitWriter) Bytes() []byte {
	return w.data
}
