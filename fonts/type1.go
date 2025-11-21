package fonts

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	"pdflib/ir/semantic"
)

// ParseType1 parses PFB (font data) and returns a semantic.Font.
// It extracts metrics from the ASCII segment of the PFB.
func ParseType1(name string, pfbData []byte) (*semantic.Font, error) {
	l1, _, _, err := parsePFB(pfbData)
	if err != nil {
		return nil, fmt.Errorf("parse pfb: %w", err)
	}

	// Extract metrics from the ASCII segment (first l1 bytes)
	// The ASCII segment is a PostScript program. We can scan it for keys.
	asciiSegment := pfbData[6 : 6+l1] // Skip header (6 bytes)
	metrics, err := parseType1Metrics(asciiSegment)
	if err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}

	if metrics.FontName == "" {
		metrics.FontName = name
	}

	descriptor := &semantic.FontDescriptor{
		FontName:     metrics.FontName,
		Flags:        32, // Nonsymbolic
		ItalicAngle:  metrics.ItalicAngle,
		Ascent:       metrics.Ascent,
		Descent:      metrics.Descent,
		CapHeight:    metrics.CapHeight,
		StemV:        metrics.StemV,
		FontBBox:     metrics.FontBBox,
		FontFile:     pfbData,
		FontFileType: "FontFile", // Type 1
	}

	// We store the lengths in the descriptor or font?
	// semantic.Font doesn't have Length1/2/3.
	// We can store them in a custom way or assume the writer re-calculates them.
	// For now, we just return the font. The writer will need to handle PFB splitting if it wants to be precise,
	// or we can add fields to semantic.Font later.
	// Actually, let's check if we can pass them.
	// For now, we ignore passing l1, l2, l3 explicitly, assuming the writer can re-parse PFB or we add support later.

	return &semantic.Font{
		Subtype:    "Type1",
		BaseFont:   metrics.FontName,
		Encoding:   "WinAnsiEncoding",
		Widths:     metrics.Widths, // Note: Type 1 usually doesn't have widths in the main dict, they are in AFM/PFM.
		Descriptor: descriptor,
	}, nil
}

func parsePFB(data []byte) (int, int, int, error) {
	r := bytes.NewReader(data)
	l1, l2, l3 := 0, 0, 0

	// Segment 1: ASCII
	if err := checkHeader(r, 1); err != nil {
		return 0, 0, 0, err
	}
	len1, err := readLength(r)
	if err != nil {
		return 0, 0, 0, err
	}
	l1 = int(len1)
	if _, err := r.Seek(int64(l1), io.SeekCurrent); err != nil {
		return 0, 0, 0, err
	}

	// Segment 2: Binary
	if err := checkHeader(r, 2); err != nil {
		return 0, 0, 0, err
	}
	len2, err := readLength(r)
	if err != nil {
		return 0, 0, 0, err
	}
	l2 = int(len2)
	if _, err := r.Seek(int64(l2), io.SeekCurrent); err != nil {
		return 0, 0, 0, err
	}

	// Segment 3: EOF or more binary?
	if err := checkHeader(r, 1); err != nil {
		// Might be EOF directly?
		return l1, l2, 0, nil
	}
	len3, err := readLength(r)
	if err != nil {
		return 0, 0, 0, err
	}
	l3 = int(len3)

	return l1, l2, l3, nil
}

func checkHeader(r *bytes.Reader, expectedType byte) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	if b != 0x80 {
		return fmt.Errorf("invalid pfb header byte: %x", b)
	}
	t, err := r.ReadByte()
	if err != nil {
		return err
	}
	if t != expectedType {
		return fmt.Errorf("expected pfb segment type %d, got %d", expectedType, t)
	}
	return nil
}

func readLength(r *bytes.Reader) (uint32, error) {
	var l uint32
	if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
		return 0, err
	}
	return l, nil
}

type type1Metrics struct {
	FontName    string
	ItalicAngle float64
	Ascent      float64
	Descent     float64
	CapHeight   float64
	StemV       int
	FontBBox    [4]float64
	Widths      map[int]int
}

func parseType1Metrics(data []byte) (*type1Metrics, error) {
	m := &type1Metrics{
		Widths: make(map[int]int),
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/FontName") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				m.FontName = strings.TrimPrefix(parts[1], "/")
			}
		} else if strings.HasPrefix(line, "/ItalicAngle") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				m.ItalicAngle, _ = strconv.ParseFloat(parts[1], 64)
			}
		} else if strings.HasPrefix(line, "/FontBBox") {
			// /FontBBox { -100 -200 1000 900 } readonly def
			start := strings.Index(line, "{")
			end := strings.Index(line, "}")
			if start != -1 && end != -1 && end > start {
				content := line[start+1 : end]
				nums := strings.Fields(content)
				if len(nums) >= 4 {
					m.FontBBox[0], _ = strconv.ParseFloat(nums[0], 64)
					m.FontBBox[1], _ = strconv.ParseFloat(nums[1], 64)
					m.FontBBox[2], _ = strconv.ParseFloat(nums[2], 64)
					m.FontBBox[3], _ = strconv.ParseFloat(nums[3], 64)
				}
			}
		}
		// Ascent/Descent/CapHeight are not always explicit in Type 1 dict.
		// They are usually in AFM.
		// We can approximate them from BBox if missing.
	}

	if m.Ascent == 0 {
		m.Ascent = m.FontBBox[3]
	}
	if m.Descent == 0 {
		m.Descent = m.FontBBox[1]
	}
	if m.CapHeight == 0 {
		m.CapHeight = m.Ascent // Approximation
	}

	return m, nil
}
