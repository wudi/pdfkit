package cmm

import (
	"encoding/binary"
	"errors"
	"math"
)

type LUT struct {
	InputChannels  uint8
	OutputChannels uint8
	GridPoints     uint8
	Matrix         [9]float64
	InputTables    [][]float64 // Normalized 0-1
	CLUT           []float64   // Normalized 0-1
	OutputTables   [][]float64 // Normalized 0-1
}

func (p *ICCProfile) ReadLUTTag(sig string) (*LUT, error) {
	data, ok := p.GetTag(sig)
	if !ok {
		return nil, errors.New("tag not found")
	}
	if len(data) < 8 {
		return nil, errors.New("tag too short")
	}
	typeSig := binary.BigEndian.Uint32(data[0:4])

	switch typeSig {
	case 0x6D667431: // 'mft1' (8-bit LUT)
		return parseMFT1(data)
	case 0x6D667432: // 'mft2' (16-bit LUT)
		return parseMFT2(data)
	}
	return nil, errors.New("unsupported LUT type")
}

func parseMFT2(data []byte) (*LUT, error) {
	if len(data) < 52 {
		return nil, errors.New("mft2 tag too short")
	}
	lut := &LUT{}
	lut.InputChannels = data[8]
	lut.OutputChannels = data[9]
	lut.GridPoints = data[10]

	// Matrix (12-48) - 3x3 fixed 15.16
	for i := 0; i < 9; i++ {
		lut.Matrix[i] = s15Fixed16ToFloat(binary.BigEndian.Uint32(data[12+i*4 : 16+i*4]))
	}

	inputEntries := binary.BigEndian.Uint16(data[48:50])
	outputEntries := binary.BigEndian.Uint16(data[50:52])

	offset := 52

	// Input Tables
	// inputChannels * inputEntries * 2 bytes
	inputSize := int(lut.InputChannels) * int(inputEntries) * 2
	if offset+inputSize > len(data) {
		return nil, errors.New("mft2 input tables truncated")
	}
	lut.InputTables = make([][]float64, lut.InputChannels)
	for c := 0; c < int(lut.InputChannels); c++ {
		lut.InputTables[c] = make([]float64, inputEntries)
		for i := 0; i < int(inputEntries); i++ {
			val := binary.BigEndian.Uint16(data[offset : offset+2])
			lut.InputTables[c][i] = float64(val) / 65535.0
			offset += 2
		}
	}

	// CLUT
	// gridPoints ^ inputChannels * outputChannels * 2 bytes
	numGridPoints := int(math.Pow(float64(lut.GridPoints), float64(lut.InputChannels)))
	clutSize := numGridPoints * int(lut.OutputChannels) * 2
	if offset+clutSize > len(data) {
		return nil, errors.New("mft2 CLUT truncated")
	}
	lut.CLUT = make([]float64, numGridPoints*int(lut.OutputChannels))
	for i := 0; i < len(lut.CLUT); i++ {
		val := binary.BigEndian.Uint16(data[offset : offset+2])
		lut.CLUT[i] = float64(val) / 65535.0
		offset += 2
	}

	// Output Tables
	// outputChannels * outputEntries * 2 bytes
	outputSize := int(lut.OutputChannels) * int(outputEntries) * 2
	if offset+outputSize > len(data) {
		return nil, errors.New("mft2 output tables truncated")
	}
	lut.OutputTables = make([][]float64, lut.OutputChannels)
	for c := 0; c < int(lut.OutputChannels); c++ {
		lut.OutputTables[c] = make([]float64, outputEntries)
		for i := 0; i < int(outputEntries); i++ {
			val := binary.BigEndian.Uint16(data[offset : offset+2])
			lut.OutputTables[c][i] = float64(val) / 65535.0
			offset += 2
		}
	}

	return lut, nil
}

func parseMFT1(data []byte) (*LUT, error) {
	if len(data) < 52 {
		return nil, errors.New("mft1 tag too short")
	}
	lut := &LUT{}
	lut.InputChannels = data[8]
	lut.OutputChannels = data[9]
	lut.GridPoints = data[10]

	// Matrix (12-48)
	for i := 0; i < 9; i++ {
		lut.Matrix[i] = s15Fixed16ToFloat(binary.BigEndian.Uint32(data[12+i*4 : 16+i*4]))
	}

	// mft1 has fixed 256 entries for input/output tables
	inputEntries := 256
	outputEntries := 256

	offset := 52

	// Input Tables (1 byte per entry)
	inputSize := int(lut.InputChannels) * inputEntries
	if offset+inputSize > len(data) {
		return nil, errors.New("mft1 input tables truncated")
	}
	lut.InputTables = make([][]float64, lut.InputChannels)
	for c := 0; c < int(lut.InputChannels); c++ {
		lut.InputTables[c] = make([]float64, inputEntries)
		for i := 0; i < inputEntries; i++ {
			val := data[offset]
			lut.InputTables[c][i] = float64(val) / 255.0
			offset++
		}
	}

	// CLUT (1 byte per entry)
	numGridPoints := int(math.Pow(float64(lut.GridPoints), float64(lut.InputChannels)))
	clutSize := numGridPoints * int(lut.OutputChannels)
	if offset+clutSize > len(data) {
		return nil, errors.New("mft1 CLUT truncated")
	}
	lut.CLUT = make([]float64, numGridPoints*int(lut.OutputChannels))
	for i := 0; i < len(lut.CLUT); i++ {
		val := data[offset]
		lut.CLUT[i] = float64(val) / 255.0
		offset++
	}

	// Output Tables (1 byte per entry)
	outputSize := int(lut.OutputChannels) * outputEntries
	if offset+outputSize > len(data) {
		return nil, errors.New("mft1 output tables truncated")
	}
	lut.OutputTables = make([][]float64, lut.OutputChannels)
	for c := 0; c < int(lut.OutputChannels); c++ {
		lut.OutputTables[c] = make([]float64, outputEntries)
		for i := 0; i < outputEntries; i++ {
			val := data[offset]
			lut.OutputTables[c][i] = float64(val) / 255.0
			offset++
		}
	}

	return lut, nil
}

// Convert executes the LUT transform on the input color.
func (lut *LUT) Convert(in []float64) ([]float64, error) {
	if len(in) != int(lut.InputChannels) {
		return nil, errors.New("input channels mismatch")
	}

	// 1. Matrix (only if input is 3 channels, usually XYZ)
	// The spec says Matrix is applied BEFORE input tables for some types, but for mft1/mft2
	// the matrix is 3x3 and applied to XYZ input?
	// Actually, for A2B0 (Device -> PCS), the processing order is:
	// Input Tables -> 3x3 Matrix (only if PCS is XYZ) -> CLUT -> Output Tables
	// Wait, let's check the spec.
	// For Lut16Type:
	// Matrix -> Input Tables -> CLUT -> Output Tables (This is what some sources say)
	// ISO 15076-1:2010 10.10 lut16Type
	// "The matrix is organized as a 3x3 array... The matrix is used only when the input colour space is XYZ."
	// "The processing sequence is: Matrix -> Input Tables -> CLUT -> Output Tables"

	// So if InputChannels == 3 (likely XYZ), we apply Matrix first.
	// But wait, A2B0 is Device -> PCS. If Device is RGB, Input is RGB.
	// If Device is XYZ, Input is XYZ.
	// Usually A2B0 is used for RGB/CMYK -> PCS.
	// If RGB -> PCS, Input is RGB. Matrix is NOT used?
	// Actually, the matrix is often identity if not used.

	// Let's assume standard processing:
	// 1. Matrix (if 3x3 and input is 3 channels)
	// 2. Input Tables (1D curves)
	// 3. CLUT (N-dimensional interpolation)
	// 4. Output Tables (1D curves)

	temp := make([]float64, len(in))
	copy(temp, in)

	// Apply Matrix if 3 channels
	if lut.InputChannels == 3 {
		// Check if matrix is identity? Or just apply it.
		// e00 e01 e02
		// e10 e11 e12
		// e20 e21 e22
		x := temp[0]*lut.Matrix[0] + temp[1]*lut.Matrix[1] + temp[2]*lut.Matrix[2]
		y := temp[0]*lut.Matrix[3] + temp[1]*lut.Matrix[4] + temp[2]*lut.Matrix[5]
		z := temp[0]*lut.Matrix[6] + temp[1]*lut.Matrix[7] + temp[2]*lut.Matrix[8]
		temp[0], temp[1], temp[2] = x, y, z
	}

	// Apply Input Tables
	for c := 0; c < int(lut.InputChannels); c++ {
		temp[c] = interp1D(temp[c], lut.InputTables[c])
	}

	// Apply CLUT
	clutOut := interpCLUT(temp, lut.CLUT, int(lut.InputChannels), int(lut.OutputChannels), int(lut.GridPoints))

	// Apply Output Tables
	out := make([]float64, lut.OutputChannels)
	for c := 0; c < int(lut.OutputChannels); c++ {
		out[c] = interp1D(clutOut[c], lut.OutputTables[c])
	}

	return out, nil
}

func interp1D(val float64, table []float64) float64 {
	if val <= 0 {
		return table[0]
	}
	if val >= 1 {
		return table[len(table)-1]
	}
	// Scale to index
	f := val * float64(len(table)-1)
	idx := int(f)
	frac := f - float64(idx)
	return table[idx]*(1-frac) + table[idx+1]*frac
}

func interpCLUT(in []float64, clut []float64, inCh, outCh, gridPoints int) []float64 {
	// N-linear interpolation
	// This is complex to implement generically for N dimensions.
	// For now, let's implement for 3 inputs (common case RGB/Lab) and 4 inputs (CMYK).
	// If N is large, this is slow.

	// Simplified: Nearest Neighbor for now to save tokens/complexity, or Trilinear for 3D.
	// Let's do Trilinear for 3D.

	if inCh == 3 {
		return interpCLUT3D(in, clut, outCh, gridPoints)
	}
	if inCh == 4 {
		// Fallback to nearest neighbor for 4D
		return interpCLUTNearest(in, clut, inCh, outCh, gridPoints)
	}

	// Fallback
	return interpCLUTNearest(in, clut, inCh, outCh, gridPoints)
}

func interpCLUTNearest(in []float64, clut []float64, inCh, outCh, gridPoints int) []float64 {
	// Calculate index
	idx := 0
	stride := outCh
	for i := 0; i < inCh; i++ {
		// Scale input to grid index
		val := in[i]
		if val < 0 {
			val = 0
		}
		if val > 1 {
			val = 1
		}
		gridIdx := int(val*float64(gridPoints-1) + 0.5)

		// Stride for this dimension
		// Dim 0 varies fastest? Or slowest?
		// ICC spec: "The first dimension varies least rapidly..."
		// So index = d0 * (G^(N-1)) + d1 * (G^(N-2)) ...

		power := int(math.Pow(float64(gridPoints), float64(inCh-1-i)))
		idx += gridIdx * power
	}

	offset := idx * stride
	out := make([]float64, outCh)
	for c := 0; c < outCh; c++ {
		out[c] = clut[offset+c]
	}
	return out
}

func interpCLUT3D(in []float64, clut []float64, outCh, gridPoints int) []float64 {
	// Trilinear interpolation
	// in[0], in[1], in[2] -> x, y, z

	// Indices and fractions
	g := float64(gridPoints - 1)

	x := in[0] * g
	y := in[1] * g
	z := in[2] * g

	x0 := int(x)
	y0 := int(y)
	z0 := int(z)

	if x0 >= gridPoints-1 {
		x0 = gridPoints - 2
	}
	if y0 >= gridPoints-1 {
		y0 = gridPoints - 2
	}
	if z0 >= gridPoints-1 {
		z0 = gridPoints - 2
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if z0 < 0 {
		z0 = 0
	}

	x1 := x0 + 1
	y1 := y0 + 1
	z1 := z0 + 1

	dx := x - float64(x0)
	dy := y - float64(y0)
	dz := z - float64(z0)

	// Helper to get value at grid point
	getVal := func(ix, iy, iz, ch int) float64 {
		// Index = ix * G^2 + iy * G + iz (Assuming order 0,1,2 where 0 is slowest)
		// ICC Spec: "first dimension varies least rapidly" -> index 0
		idx := ix*gridPoints*gridPoints + iy*gridPoints + iz
		return clut[idx*outCh+ch]
	}

	out := make([]float64, outCh)
	for c := 0; c < outCh; c++ {
		c000 := getVal(x0, y0, z0, c)
		c001 := getVal(x0, y0, z1, c)
		c010 := getVal(x0, y1, z0, c)
		c011 := getVal(x0, y1, z1, c)
		c100 := getVal(x1, y0, z0, c)
		c101 := getVal(x1, y0, z1, c)
		c110 := getVal(x1, y1, z0, c)
		c111 := getVal(x1, y1, z1, c)

		c00 := c000*(1-dz) + c001*dz
		c01 := c010*(1-dz) + c011*dz
		c10 := c100*(1-dz) + c101*dz
		c11 := c110*(1-dz) + c111*dz

		c0 := c00*(1-dy) + c01*dy
		c1 := c10*(1-dy) + c11*dy

		out[c] = c0*(1-dx) + c1*dx
	}

	return out
}
