package fonts

import "github.com/go-text/typesetting/language"

// Export DetectScript for testing
func DetectScript(runes []rune) language.Script {
	return detectScript(runes)
}
