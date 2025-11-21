package fonts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"pdflib/ir/semantic"
)

// OpenTypeTable represents an entry in the OpenType table directory.
type OpenTypeTable struct {
	Tag      string
	CheckSum uint32
	Offset   uint32
	Length   uint32
}

// LoadOpenType parses an OpenType font (CFF or TrueType outlines).
// If it contains CFF outlines, it extracts the CFF table and embeds it as FontFile3.
func LoadOpenType(name string, data []byte) (*semantic.Font, error) {
	tables, err := ParseOpenTypeTableDirectory(data)
	if err != nil {
		return nil, fmt.Errorf("parse table directory: %w", err)
	}

	if cffTable, ok := tables["CFF "]; ok {
		// CFF OpenType
		cffData, err := ExtractTable(data, cffTable)
		if err != nil {
			return nil, fmt.Errorf("extract CFF table: %w", err)
		}

		// Parse CFF to verify and get internal name if needed
		cff, err := ParseCFF(cffData)
		if err != nil {
			return nil, fmt.Errorf("parse CFF: %w", err)
		}

		fontName := name
		if len(cff.Names) > 0 && fontName == "" {
			fontName = cff.Names[0]
		}

		// Use sfnt for metrics
		f, err := sfnt.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse sfnt: %w", err)
		}

		unitsPerEm := f.UnitsPerEm()
		ppem := fixed.Int26_6(unitsPerEm << 6)
		buf := &sfnt.Buffer{}

		metrics, _ := f.Metrics(buf, ppem, font.HintingNone)
		bounds, _ := f.Bounds(buf, ppem, font.HintingNone)

		descriptor := &semantic.FontDescriptor{
			FontName:    fontName,
			Flags:       4, // Symbolic
			ItalicAngle: italicAngle(f),
			Ascent:      scaleFixed(metrics.Ascent, unitsPerEm),
			Descent:     scaleFixed(metrics.Descent, unitsPerEm),
			CapHeight:   scaleFixed(metrics.Ascent, unitsPerEm), // Approx
			StemV:       80,
			FontBBox: [4]float64{
				scaleFixed(bounds.Min.X, unitsPerEm),
				scaleFixed(bounds.Min.Y, unitsPerEm),
				scaleFixed(bounds.Max.X, unitsPerEm),
				scaleFixed(bounds.Max.Y, unitsPerEm),
			},
			FontFile:     cffData,
			FontFileType: "FontFile3", // Type1C
		}

		// For CFF, we usually use Type1C (Compact Font Format)
		// We can wrap it in Type0 if we want CID support, but for simple CFF we can use Type1 with FontFile3.
		// However, standard Type1 fonts use FontFile (PFB).
		// OpenType with CFF is usually embedded as a CIDFontType0 inside a Type0 font,
		// OR as a Type1 font if it's not CID-keyed.
		// Let's assume simple Type1 behavior for now unless we detect CID.

		// Check if CFF is CID
		isCID := false
		if len(cff.TopDicts) > 0 {
			if _, ok := cff.TopDicts[0][1230]; ok { // ROS operator (Registry-Ordering-Supplement) indicates CID
				isCID = true
			}
		}

		widths := glyphWidths(f, buf, unitsPerEm, ppem)

		if isCID {
			cidInfo := semantic.CIDSystemInfo{Registry: "Adobe", Ordering: "Identity", Supplement: 0}
			descendant := &semantic.CIDFont{
				Subtype:       "CIDFontType0", // CFF based
				BaseFont:      fontName,
				CIDSystemInfo: cidInfo,
				DW:            1000,
				W:             widths,
				Descriptor:    descriptor,
			}
			return &semantic.Font{
				Subtype:        "Type0",
				BaseFont:       fontName,
				Encoding:       "Identity-H",
				CIDSystemInfo:  &cidInfo,
				DescendantFont: descendant,
				ToUnicode:      buildToUnicodeMap(f, buf),
			}, nil
		}

		return &semantic.Font{
			Subtype:    "Type1",
			BaseFont:   fontName,
			Encoding:   "WinAnsiEncoding",
			Widths:     widths,
			Descriptor: descriptor,
		}, nil
	}

	// Fallback to TrueType loader
	return LoadTrueType(name, data)
}

// ParseOpenTypeTableDirectory parses the header and table directory of an OpenType/TrueType font.
func ParseOpenTypeTableDirectory(data []byte) (map[string]OpenTypeTable, error) {
	r := bytes.NewReader(data)

	// Offset Table
	var scalerType uint32
	if err := binary.Read(r, binary.BigEndian, &scalerType); err != nil {
		return nil, err
	}

	// 0x00010000 for TrueType, 0x4F54544F ('OTTO') for CFF OpenType
	// We accept both here as we just want the tables.

	var numTables uint16
	if err := binary.Read(r, binary.BigEndian, &numTables); err != nil {
		return nil, err
	}

	// Skip searchRange, entrySelector, rangeShift
	if _, err := r.Seek(6, io.SeekCurrent); err != nil {
		return nil, err
	}

	tables := make(map[string]OpenTypeTable)
	for i := 0; i < int(numTables); i++ {
		var tag [4]byte
		if _, err := io.ReadFull(r, tag[:]); err != nil {
			return nil, err
		}
		var checkSum, offset, length uint32
		if err := binary.Read(r, binary.BigEndian, &checkSum); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.BigEndian, &offset); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return nil, err
		}

		tables[string(tag[:])] = OpenTypeTable{
			Tag:      string(tag[:]),
			CheckSum: checkSum,
			Offset:   offset,
			Length:   length,
		}
	}

	return tables, nil
}

// ExtractTable returns the raw data of a specific table.
func ExtractTable(data []byte, table OpenTypeTable) ([]byte, error) {
	if int(table.Offset+table.Length) > len(data) {
		return nil, fmt.Errorf("table %s out of bounds", table.Tag)
	}
	return data[table.Offset : table.Offset+table.Length], nil
}
