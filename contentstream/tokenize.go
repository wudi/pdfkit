package contentstream

import "strings"

// tokenize splits a content stream into operands/operators (naive whitespace split).
func tokenize(src string) []string {
	fields := strings.Fields(src)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f)
	}
	return out
}
