//go:build js && wasm

package search

import (
	"strings"
	"unicode"
)

func NormalizeNFC(s string) string {
	return s
}

func ToLower(s string) string {
	return strings.ToLower(s)
}

func ToTitle(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) == 1 {
		return string(unicode.ToUpper(r[0]))
	}
	return string(unicode.ToUpper(r[0])) + strings.ToLower(string(r[1:]))
}
