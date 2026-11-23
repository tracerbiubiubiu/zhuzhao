package riskasset

import (
	"strconv"
	"unicode/utf8"
)

// validResponseText prevents encoding/json's replacement of malformed Unicode
// from silently aliasing a stable identity or altering owner/org source evidence.
// JSON syntax and field types are validated separately by json.Unmarshal.
func validResponseText(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	for i := 0; i < len(raw); {
		if raw[i] != '\\' {
			i++
			continue
		}
		if i+1 >= len(raw) {
			return false
		}
		if raw[i+1] != 'u' {
			i += 2 // Includes escaped backslashes: literal "\\ud800" is valid evidence.
			continue
		}
		if i+6 > len(raw) {
			return false
		}
		value, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return false
		}
		i += 6
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		} else if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
	}
	return true
}
