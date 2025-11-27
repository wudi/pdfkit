package cmm

import (
	"testing"
)

func TestInterpCLUT3D(t *testing.T) {
	// Create a simple 2x2x2 grid (8 points)
	gridPoints := 2

	// Table data (8 points * 1 output channel)
	// Index = ix * G^2 + iy * G + iz
	// We want output = x*10 + y*20 + z*40
	// x,y,z are 0 or 1 (indices)

	table := make([]float64, 8)
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			for z := 0; z < 2; z++ {
				val := float64(x*10 + y*20 + z*40)
				idx := x*4 + y*2 + z
				table[idx] = val
			}
		}
	}

	// Test cases
	tests := []struct {
		in  []float64
		out float64
	}{
		{[]float64{0, 0, 0}, 0},
		{[]float64{1, 0, 0}, 10},
		{[]float64{0, 1, 0}, 20},
		{[]float64{0, 0, 1}, 40},
		{[]float64{1, 1, 1}, 70},
		{[]float64{0.5, 0, 0}, 5},
		{[]float64{0, 0.5, 0}, 10},
		{[]float64{0, 0, 0.5}, 20},
		{[]float64{0.5, 0.5, 0}, 15},   // 0.5*10 + 0.5*20 = 15
		{[]float64{0.5, 0.5, 0.5}, 35}, // 5 + 10 + 20 = 35
	}

	for _, tc := range tests {
		res := interpCLUT3D(tc.in, table, 1, gridPoints) // 1 output channel
		if len(res) != 1 {
			t.Errorf("Expected 1 output, got %d", len(res))
			continue
		}
		// Allow small error for float math
		diff := res[0] - tc.out
		if diff < -0.001 || diff > 0.001 {
			t.Errorf("Input %v: expected %v, got %v", tc.in, tc.out, res[0])
		}
	}
}
