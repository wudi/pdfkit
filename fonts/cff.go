package fonts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
)

// CFF represents a parsed Compact Font Format structure.
type CFF struct {
	Header   CFFHeader
	Names    []string
	TopDicts []map[int][]Operand // Key is operator, value is operands
	Strings  []string
}

type CFFHeader struct {
	Major   uint8
	Minor   uint8
	HdrSize uint8
	OffSize uint8
}

type Operand struct {
	Int   int
	Float float64
	IsInt bool
}

// ParseCFF parses the CFF data.
func ParseCFF(data []byte) (*CFF, error) {
	r := bytes.NewReader(data)

	// Header
	var hdr CFFHeader
	if err := binary.Read(r, binary.BigEndian, &hdr); err != nil {
		return nil, err
	}

	if _, err := r.Seek(int64(hdr.HdrSize), io.SeekStart); err != nil {
		return nil, err
	}

	// Name INDEX
	names, err := readIndex(r)
	if err != nil {
		return nil, fmt.Errorf("read name index: %w", err)
	}
	nameStrings := make([]string, len(names))
	for i, b := range names {
		nameStrings[i] = string(b)
	}

	// Top DICT INDEX
	topDictData, err := readIndex(r)
	if err != nil {
		return nil, fmt.Errorf("read top dict index: %w", err)
	}

	topDicts := make([]map[int][]Operand, len(topDictData))
	for i, data := range topDictData {
		topDicts[i], err = parseDict(data)
		if err != nil {
			return nil, fmt.Errorf("parse top dict %d: %w", i, err)
		}
	}

	// String INDEX
	stringData, err := readIndex(r)
	if err != nil {
		return nil, fmt.Errorf("read string index: %w", err)
	}
	strings := make([]string, len(stringData))
	for i, b := range stringData {
		strings[i] = string(b)
	}

	return &CFF{
		Header:   hdr,
		Names:    nameStrings,
		TopDicts: topDicts,
		Strings:  strings,
	}, nil
}

func readIndex(r *bytes.Reader) ([][]byte, error) {
	var count uint16
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	offSize, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	offsets := make([]int, count+1)
	for i := 0; i <= int(count); i++ {
		off, err := readOffset(r, int(offSize))
		if err != nil {
			return nil, err
		}
		offsets[i] = off
	}

	totalSize := offsets[count] - 1 // Offsets are 1-based relative to data start

	data := make([]byte, totalSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	items := make([][]byte, count)
	for i := 0; i < int(count); i++ {
		start := offsets[i] - 1
		end := offsets[i+1] - 1
		if start < 0 || end > len(data) || start > end {
			return nil, fmt.Errorf("invalid index offsets")
		}
		items[i] = data[start:end]
	}

	return items, nil
}

func readOffset(r io.Reader, size int) (int, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[4-size:]); err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint32(buf[:])), nil
}

func parseDict(data []byte) (map[int][]Operand, error) {
	dict := make(map[int][]Operand)
	var operands []Operand

	r := bytes.NewReader(data)
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if b <= 21 {
			// Operator
			op := int(b)
			if b == 12 {
				b2, err := r.ReadByte()
				if err != nil {
					return nil, err
				}
				op = 1200 + int(b2)
			}
			dict[op] = operands
			operands = nil
		} else if b == 28 || b == 29 || (b >= 32 && b <= 254) {
			// Integer operand
			r.UnreadByte()
			val, err := readInteger(r)
			if err != nil {
				return nil, err
			}
			operands = append(operands, Operand{Int: val, IsInt: true})
		} else if b == 30 {
			// Real operand
			val, err := readReal(r)
			if err != nil {
				return nil, err
			}
			operands = append(operands, Operand{Float: val, IsInt: false})
		} else {
			// Reserved
		}
	}
	return dict, nil
}

func readReal(r *bytes.Reader) (float64, error) {
	var s string
	done := false
	for !done {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		nibbles := []byte{b >> 4, b & 0x0f}
		for _, n := range nibbles {
			switch n {
			case 0xa:
				s += "."
			case 0xb:
				s += "E"
			case 0xc:
				s += "E-"
			case 0xd:
				// reserved
			case 0xe:
				s += "-"
			case 0xf:
				done = true
			default:
				s += fmt.Sprintf("%d", n)
			}
			if done {
				break
			}
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

func readInteger(r *bytes.Reader) (int, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	if b0 >= 32 && b0 <= 246 {
		return int(b0) - 139, nil
	} else if b0 >= 247 && b0 <= 250 {
		b1, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		return (int(b0)-247)*256 + int(b1) + 108, nil
	} else if b0 >= 251 && b0 <= 254 {
		b1, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		return -(int(b0)-251)*256 - int(b1) - 108, nil
	} else if b0 == 28 {
		var val int16
		if err := binary.Read(r, binary.BigEndian, &val); err != nil {
			return 0, err
		}
		return int(val), nil
	} else if b0 == 29 {
		var val int32
		if err := binary.Read(r, binary.BigEndian, &val); err != nil {
			return 0, err
		}
		return int(val), nil
	}
	return 0, fmt.Errorf("invalid integer prefix: %d", b0)
}
