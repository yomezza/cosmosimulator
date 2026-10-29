package textutil

import (
	"unicode"
	"unicode/utf8"
)

func Valid(s string, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return false
		}
	}
	return true
}
