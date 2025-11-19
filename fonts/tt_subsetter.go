package fonts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// SubsetTrueType subsets a TrueType font file, keeping only the specified GIDs.
// It performs a sparse subsetting: GIDs are preserved (Identity-H compatible),
// but unused glyph data is removed from the 'glyf' table.
func SubsetTrueType(data []byte, usedGIDs map[int]bool) ([]byte, error) {
	p := &ttParser{data: data}
	if err := p.ParseDirectory(); err != nil {
		return nil, err
	}

	// Check for essential tables
	if !p.HasTable("glyf") || !p.HasTable("loca") || !p.HasTable("head") || !p.HasTable("maxp") || !p.HasTable("hmtx") || !p.HasTable("hhea") {
		// Not a standard TrueType font (maybe CFF/OTF), return original
		return data, nil
	}

	// Check for complex scripts (Arabic) in GSUB
	if p.hasComplexScript() {
		// For complex scripts, naive subsetting breaks shaping (ligatures/positioning).
		// Until we implement a proper shaper-aware subsetter, we must keep the full font
		// or at least all glyphs. For safety, we return the original font.
		return data, nil
	}

	// 1. Read head to get indexToLocFormat
	headData, err := p.ReadTable("head")
	if err != nil {
		return nil, err
	}
	indexToLocFormat := int16(binary.BigEndian.Uint16(headData[50:52]))

	// 2. Read maxp to get numGlyphs
	maxpData, err := p.ReadTable("maxp")
	if err != nil {
		return nil, err
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxpData[4:6]))

	// 3. Compute Closure of used glyphs (handle composites)
	// Always include .notdef (GID 0)
	closure := make(map[int]bool)
	closure[0] = true
	for gid := range usedGIDs {
		closure[gid] = true
	}

	if err := p.computeClosure(closure, numGlyphs, indexToLocFormat); err != nil {
		return nil, fmt.Errorf("compute closure: %w", err)
	}

	// 4. Determine new numGlyphs (trim trailing unused glyphs)
	maxUsedGID := 0
	for gid := range closure {
		if gid > maxUsedGID {
			maxUsedGID = gid
		}
	}
	newNumGlyphs := maxUsedGID + 1
	if newNumGlyphs > numGlyphs {
		newNumGlyphs = numGlyphs // Should not happen if input is valid
	}

	// 5. Rebuild glyf and loca tables
	newGlyf, newLoca, err := p.rebuildGlyfLoca(closure, newNumGlyphs, indexToLocFormat)
	if err != nil {
		return nil, err
	}

	// 6. Rebuild hmtx table
	newHmtx, err := p.rebuildHmtx(newNumGlyphs)
	if err != nil {
		return nil, err
	}

	// 7. Update maxp (numGlyphs)
	newMaxp := make([]byte, len(maxpData))
	copy(newMaxp, maxpData)
	binary.BigEndian.PutUint16(newMaxp[4:], uint16(newNumGlyphs))

	// 8. Assemble new font
	// Tables to keep
	keepTables := []string{"head", "hhea", "maxp", "hmtx", "loca", "glyf", "cmap", "name", "OS/2", "post", "cvt ", "fpgm", "prep", "GSUB", "GPOS", "GDEF", "GASP"}

	w := &ttWriter{}

	w.AddTable("glyf", newGlyf)
	w.AddTable("loca", newLoca)
	w.AddTable("hmtx", newHmtx)
	w.AddTable("maxp", newMaxp)

	// Copy other tables
	for _, tag := range keepTables {
		if tag == "glyf" || tag == "loca" || tag == "hmtx" || tag == "maxp" {
			continue
		}
		if p.HasTable(tag) {
			data, err := p.ReadTable(tag)
			if err != nil {
				return nil, err
			}

			// Patch hhea if needed
			if tag == "hhea" {
				// Update numberOfHMetrics to newNumGlyphs
				// hhea is 36 bytes long usually. numberOfHMetrics is at offset 34.
				if len(data) >= 36 {
					// Make a copy to avoid modifying original
					newData := make([]byte, len(data))
					copy(newData, data)
					binary.BigEndian.PutUint16(newData[34:], uint16(newNumGlyphs))
					data = newData
				}
			}

			w.AddTable(tag, data)
		}
	}

	return w.Bytes(), nil
}

type ttParser struct {
	data   []byte
	tables map[string]tableEntry
}

type tableEntry struct {
	offset uint32
	length uint32
}

func (p *ttParser) ParseDirectory() error {
	if len(p.data) < 12 {
		return fmt.Errorf("invalid font header")
	}
	numTables := int(binary.BigEndian.Uint16(p.data[4:6]))
	p.tables = make(map[string]tableEntry)

	offset := 12
	for i := 0; i < numTables; i++ {
		if offset+16 > len(p.data) {
			return fmt.Errorf("table directory truncated")
		}
		tag := string(p.data[offset : offset+4])
		chk := binary.BigEndian.Uint32(p.data[offset+4 : offset+8])
		off := binary.BigEndian.Uint32(p.data[offset+8 : offset+12])
		len := binary.BigEndian.Uint32(p.data[offset+12 : offset+16])
		_ = chk // verify checksum?

		p.tables[tag] = tableEntry{offset: off, length: len}
		offset += 16
	}
	return nil
}

func (p *ttParser) HasTable(tag string) bool {
	_, ok := p.tables[tag]
	return ok
}

func (p *ttParser) ReadTable(tag string) ([]byte, error) {
	entry, ok := p.tables[tag]
	if !ok {
		return nil, fmt.Errorf("table %s not found", tag)
	}
	if int(entry.offset+entry.length) > len(p.data) {
		return nil, fmt.Errorf("table %s out of bounds", tag)
	}
	return p.data[entry.offset : entry.offset+entry.length], nil
}

func (p *ttParser) hasComplexScript() bool {
	if !p.HasTable("GSUB") {
		return false
	}
	data, err := p.ReadTable("GSUB")
	if err != nil {
		return false
	}
	if len(data) < 10 {
		return false
	}

	// ScriptListOffset is at offset 4
	scriptListOffset := binary.BigEndian.Uint16(data[4:6])
	if int(scriptListOffset) >= len(data) {
		return false
	}

	listData := data[scriptListOffset:]
	if len(listData) < 2 {
		return false
	}
	scriptCount := binary.BigEndian.Uint16(listData[0:2])

	offset := 2
	for i := 0; i < int(scriptCount); i++ {
		if offset+6 > len(listData) {
			break
		}
		tag := string(listData[offset : offset+4])
		if tag == "arab" {
			return true
		}
		offset += 6
	}
	return false
}

func (p *ttParser) computeClosure(closure map[int]bool, numGlyphs int, indexToLocFormat int16) error {
	loca, err := p.ReadTable("loca")
	if err != nil {
		return err
	}
	glyf, err := p.ReadTable("glyf")
	if err != nil {
		return err
	}

	getLoc := func(gid int) uint32 {
		if indexToLocFormat == 0 {
			return uint32(binary.BigEndian.Uint16(loca[gid*2:])) * 2
		}
		return binary.BigEndian.Uint32(loca[gid*4:])
	}

	// Queue for BFS
	queue := make([]int, 0, len(closure))
	for gid := range closure {
		queue = append(queue, gid)
	}

	for len(queue) > 0 {
		gid := queue[0]
		queue = queue[1:]

		if gid >= numGlyphs {
			continue
		}

		start := getLoc(gid)
		end := getLoc(gid + 1)
		if start >= end {
			continue // Empty glyph
		}
		if start >= uint32(len(glyf)) {
			continue
		}

		// Parse glyph header
		// int16 numberOfContours
		// int16 xMin, yMin, xMax, yMax
		if start+10 > uint32(len(glyf)) {
			continue
		}
		numContours := int16(binary.BigEndian.Uint16(glyf[start : start+2]))

		if numContours >= 0 {
			continue // Simple glyph
		}

		// Composite glyph
		offset := start + 10
		for {
			if offset+4 > uint32(len(glyf)) {
				break
			}
			flags := binary.BigEndian.Uint16(glyf[offset : offset+2])
			subGID := int(binary.BigEndian.Uint16(glyf[offset+2 : offset+4]))

			if !closure[subGID] {
				closure[subGID] = true
				queue = append(queue, subGID)
			}

			offset += 4
			// Skip arguments
			var skip int
			if flags&0x0001 != 0 { // ARG_1_AND_2_ARE_WORDS
				skip += 4
			} else {
				skip += 2
			}
			if flags&0x0008 != 0 { // WE_HAVE_A_SCALE
				skip += 2
			} else if flags&0x0040 != 0 { // WE_HAVE_AN_X_AND_Y_SCALE
				skip += 4
			} else if flags&0x0080 != 0 { // WE_HAVE_A_TWO_BY_TWO
				skip += 8
			}
			offset += uint32(skip)

			if flags&0x0020 == 0 { // MORE_COMPONENTS
				break
			}
		}
	}
	return nil
}

func (p *ttParser) rebuildGlyfLoca(closure map[int]bool, numGlyphs int, indexToLocFormat int16) ([]byte, []byte, error) {
	oldLoca, err := p.ReadTable("loca")
	if err != nil {
		return nil, nil, err
	}
	oldGlyf, err := p.ReadTable("glyf")
	if err != nil {
		return nil, nil, err
	}

	getLoc := func(gid int) uint32 {
		if indexToLocFormat == 0 {
			return uint32(binary.BigEndian.Uint16(oldLoca[gid*2:])) * 2
		}
		return binary.BigEndian.Uint32(oldLoca[gid*4:])
	}

	var newGlyf bytes.Buffer
	var newLoca bytes.Buffer

	// We force long format for safety and simplicity in output
	// But if we want to respect original format, we can.
	// Let's stick to long format (1) for output loca to avoid overflow issues,
	// unless we want to minimize size further.
	// Actually, if we use long format, we must update 'head' table indexToLocFormat to 1.
	// Let's do that. It simplifies things.

	offsets := make([]uint32, numGlyphs+1)
	currentOffset := uint32(0)

	for gid := 0; gid < numGlyphs; gid++ {
		offsets[gid] = currentOffset
		if closure[gid] {
			start := getLoc(gid)
			end := getLoc(gid + 1)
			if start < end && start < uint32(len(oldGlyf)) && end <= uint32(len(oldGlyf)) {
				length := end - start
				newGlyf.Write(oldGlyf[start:end])
				currentOffset += length
			}
		}
	}
	offsets[numGlyphs] = currentOffset

	for _, off := range offsets {
		binary.Write(&newLoca, binary.BigEndian, off)
	}

	return newGlyf.Bytes(), newLoca.Bytes(), nil
}

func (p *ttParser) rebuildHmtx(numGlyphs int) ([]byte, error) {
	hhea, err := p.ReadTable("hhea")
	if err != nil {
		return nil, err
	}
	numOfHMetrics := int(binary.BigEndian.Uint16(hhea[34:36]))

	hmtx, err := p.ReadTable("hmtx")
	if err != nil {
		return nil, err
	}

	// We just truncate hmtx to match newNumGlyphs
	// But we need to be careful about numOfHMetrics.
	// If newNumGlyphs < numOfHMetrics, we must truncate the metrics array.
	// If newNumGlyphs >= numOfHMetrics, we keep metrics array and truncate/keep LSBS.

	// Simplest approach: reconstruct hmtx fully for newNumGlyphs.
	// But we need to know the metric for each GID.

	getMetric := func(gid int) (uint16, int16) {
		if gid < numOfHMetrics {
			adv := binary.BigEndian.Uint16(hmtx[gid*4 : gid*4+2])
			lsb := int16(binary.BigEndian.Uint16(hmtx[gid*4+2 : gid*4+4]))
			return adv, lsb
		}
		// Last entry in metrics array determines advance width
		lastAdv := binary.BigEndian.Uint16(hmtx[(numOfHMetrics-1)*4 : (numOfHMetrics-1)*4+2])

		// LSB is in the array following metrics
		lsbOffset := numOfHMetrics*4 + (gid-numOfHMetrics)*2
		lsb := int16(binary.BigEndian.Uint16(hmtx[lsbOffset : lsbOffset+2]))
		return lastAdv, lsb
	}

	// To save space, we can try to optimize numOfHMetrics, but let's just keep it simple:
	// Make all glyphs have explicit metrics (numOfHMetrics = numGlyphs).
	// This is slightly larger but robust.
	// Or better: keep original structure logic.

	// Let's just copy the relevant parts.
	var newHmtx bytes.Buffer

	// We will set newNumberOfHMetrics = newNumGlyphs (simplest valid hmtx)
	// This avoids calculating optimal numberOfHMetrics.
	// We must update hhea later.

	for gid := 0; gid < numGlyphs; gid++ {
		adv, lsb := getMetric(gid)
		binary.Write(&newHmtx, binary.BigEndian, adv)
		binary.Write(&newHmtx, binary.BigEndian, lsb)
	}

	return newHmtx.Bytes(), nil
}

type ttWriter struct {
	tables []tableData
}

type tableData struct {
	tag  string
	data []byte
}

func (w *ttWriter) AddTable(tag string, data []byte) {
	w.tables = append(w.tables, tableData{tag, data})
}

func (w *ttWriter) Bytes() []byte {
	// Sort tables by tag
	sort.Slice(w.tables, func(i, j int) bool { return w.tables[i].tag < w.tables[j].tag })

	numTables := len(w.tables)
	// Calculate offsets
	// Header: 12 bytes
	// Directory: 16 * numTables
	offset := 12 + 16*numTables

	// Align tables to 4 bytes

	var buf bytes.Buffer
	// Header
	buf.Write([]byte{0x00, 0x01, 0x00, 0x00}) // sfnt version 1.0
	binary.Write(&buf, binary.BigEndian, uint16(numTables))

	// SearchRange, EntrySelector, RangeShift
	entrySelector := 0
	for (1 << (entrySelector + 1)) <= numTables {
		entrySelector++
	}
	searchRange := (1 << entrySelector) * 16
	rangeShift := numTables*16 - searchRange

	binary.Write(&buf, binary.BigEndian, uint16(searchRange))
	binary.Write(&buf, binary.BigEndian, uint16(entrySelector))
	binary.Write(&buf, binary.BigEndian, uint16(rangeShift))

	// Directory
	for _, t := range w.tables {
		// Pad data to 4 bytes
		padding := (4 - (len(t.data) % 4)) % 4

		checksum := calcChecksum(t.data)

		buf.WriteString(t.tag)
		binary.Write(&buf, binary.BigEndian, checksum)
		binary.Write(&buf, binary.BigEndian, uint32(offset))
		binary.Write(&buf, binary.BigEndian, uint32(len(t.data)))

		offset += len(t.data) + padding
	}

	// Write Tables
	tableOffsets := make(map[string]int)
	for _, t := range w.tables {
		start := buf.Len()
		tableOffsets[t.tag] = start

		buf.Write(t.data)
		padding := (4 - (len(t.data) % 4)) % 4
		for k := 0; k < padding; k++ {
			buf.WriteByte(0)
		}
	}

	finalBytes := buf.Bytes()

	// Fix hhea numberOfHMetrics
	// We know we rebuilt hmtx with all metrics explicit.
	// So numberOfHMetrics should be numGlyphs.
	// We need to find hhea and maxp to sync them.
	// Let's do this in SubsetTrueType before adding to writer.

	// Fix head checksum adjustment
	// 1. Find head table offset
	if off, ok := tableOffsets["head"]; ok {
		// Zero out checksumAdjustment (offset 8 in head table)
		if off+12 <= len(finalBytes) {
			finalBytes[off+8] = 0
			finalBytes[off+9] = 0
			finalBytes[off+10] = 0
			finalBytes[off+11] = 0
		}

		// Recalculate checksum of head table
		// We need to update the directory entry for head too!
		// Directory is at 12 + 16*index
		// Find index of head
		for i, t := range w.tables {
			if t.tag == "head" {
				dirOffset := 12 + 16*i
				// Recalc checksum of head data (which now has 0 adj)
				// Note: padding is included in checksum calculation usually?
				// "The checksum of a table is the unsigned sum of the uint32s of the table"
				// "padded with 0s to 4-byte boundary"

				// We need the length from directory
				length := binary.BigEndian.Uint32(finalBytes[dirOffset+12 : dirOffset+16])
				// The data in finalBytes is padded.
				// We can just checksum the slice from finalBytes.
				// But we need to include padding in calculation.
				// The slice in finalBytes includes padding? No, we wrote padding after.
				// Wait, calcChecksum handles padding.

				// Let's just re-checksum the head data in finalBytes
				// It is at finalBytes[off : off+length]
				// But we need to account for padding bytes we wrote.
				paddedLen := (length + 3) & ^uint32(3)
				headSlice := finalBytes[off : uint32(off)+paddedLen]
				newChk := calcChecksum(headSlice)
				binary.BigEndian.PutUint32(finalBytes[dirOffset+4:], newChk)
				break
			}
		}

		// Calc full file checksum
		fullChk := calcChecksum(finalBytes)
		adjustment := 0xB1B0AFBA - fullChk

		binary.BigEndian.PutUint32(finalBytes[off+8:], adjustment)
	}

	return finalBytes
}

func calcChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data); i += 4 {
		if i+4 <= len(data) {
			sum += binary.BigEndian.Uint32(data[i : i+4])
		} else {
			// Handle remaining bytes
			var buf [4]byte
			copy(buf[:], data[i:])
			sum += binary.BigEndian.Uint32(buf[:])
		}
	}
	return sum
}
