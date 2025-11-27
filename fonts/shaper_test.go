package fonts_test

import (
	"testing"

	"github.com/go-text/typesetting/language"
	"github.com/wudi/pdfkit/fonts"
)

func TestDetectScript(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect language.Script
	}{
		{"Latin", "Hello World", language.Latin},
		{"Arabic", "مرحبا بالعالم", language.Arabic},
		{"Hebrew", "שלום עולם", language.Hebrew},
		{"Cyrillic", "Привет мир", language.Cyrillic},
		{"Greek", "Γειά σου Κόσμε", language.Greek},
		{"Mixed Latin/Arabic (Arabic dominant)", "Hello مرحباا", language.Arabic}, // 5 Latin, 6 Arabic.
		// Actually "مرحبا" is 5 chars: Meem, Reh, Hah, Beh, Alef.
		// "Hello" is 5.
		// If tie, it picks first encountered or last?
		// Implementation:
		// if counts[script] > maxCount { maxCount = ...; bestScript = ... }
		// So strictly greater. If tie, it keeps previous best.
		// "Hello " (6 chars) vs "مرحبا" (5 chars). Latin wins.
		{"Mixed Latin/Arabic (Latin dominant)", "Hello World مرحبا", language.Latin},
		{"Mixed Latin/Arabic (Arabic dominant)", "مرحبا بالعالم Hello", language.Arabic},
		{"CJK (Han)", "你好世界", language.Han},
		{"Hiragana", "こんにちは", language.Hiragana},
		{"Katakana", "コンニチハ", language.Katakana},
		{"Hangul", "안녕하세요", language.Hangul},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fonts.DetectScript([]rune(tc.input))
			if got != tc.expect {
				t.Errorf("Expected %v, got %v", tc.expect, got)
			}
		})
	}
}
