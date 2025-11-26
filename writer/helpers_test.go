package writer

import (
	"bytes"
	"testing"
)

func TestRunLengthEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "Empty",
			input:    []byte{},
			expected: []byte{128},
		},
		{
			name:     "Short Literal",
			input:    []byte("ABC"),
			expected: []byte{2, 'A', 'B', 'C', 128},
		},
		{
			name:     "Run of 2",
			input:    []byte("AAB"),
			expected: []byte{255, 'A', 0, 'B', 128},
		},
		{
			name:     "Run of 3",
			input:    []byte("AAAB"),
			expected: []byte{254, 'A', 0, 'B', 128},
		},
		{
			name:  "Mixed",
			input: []byte("AABBBCCCCDD"),
			expected: []byte{
				255, 'A', // AA (run 2) -> 257-2 = 255
				254, 'B', // BBB (run 3) -> 257-3 = 254
				253, 'C', // CCCC (run 4) -> 257-4 = 253
				255, 'D', // DD (run 2) -> 257-2 = 255
				128, // EOD
			},
		},
		{
			name:     "Long Literal",
			input:    bytes.Repeat([]byte{'A', 'B'}, 70), // 140 bytes, no runs > 1
			expected: nil,                                // Calculated below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runLengthEncode(tt.input)

			if tt.name == "Long Literal" {
				// Verify it split correctly (max literal length is 128)
				// 140 bytes should be 128 literal + 12 literal
				if len(got) < 140 {
					t.Errorf("Encoded length too short")
				}
				// First chunk: 127 (len 128)
				if got[0] != 127 {
					t.Errorf("Expected first chunk len 127, got %d", got[0])
				}
				// Second chunk: 11 (len 12)
				// Index = 1 + 128 = 129
				if got[129] != 11 {
					t.Errorf("Expected second chunk len 11, got %d", got[129])
				}
				return
			}

			if !bytes.Equal(got, tt.expected) {
				t.Errorf("runLengthEncode() = %v, want %v", got, tt.expected)
			}
		})
	}
}
