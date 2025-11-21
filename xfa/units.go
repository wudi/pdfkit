package xfa

import (
	"strconv"
	"strings"
)

// ParseUnit converts an XFA measurement string (e.g., "1in", "72pt", "10mm") to points.
// 1in = 72pt
// 1mm = 2.83465pt
// 1cm = 28.3465pt
func ParseUnit(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	var unit string
	var valStr string

	if strings.HasSuffix(s, "in") {
		unit = "in"
		valStr = s[:len(s)-2]
	} else if strings.HasSuffix(s, "mm") {
		unit = "mm"
		valStr = s[:len(s)-2]
	} else if strings.HasSuffix(s, "cm") {
		unit = "cm"
		valStr = s[:len(s)-2]
	} else if strings.HasSuffix(s, "pt") {
		unit = "pt"
		valStr = s[:len(s)-2]
	} else {
		// Default to points if no unit? Or maybe it's just a number.
		valStr = s
		unit = "pt"
	}

	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0
	}

	switch unit {
	case "in":
		return val * 72.0
	case "mm":
		return val * 72.0 / 25.4
	case "cm":
		return val * 72.0 / 2.54
	case "pt":
		return val
	}
	return val
}
