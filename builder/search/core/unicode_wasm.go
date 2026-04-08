//go:build js && wasm

package core

import (
	"html"
	"strings"
	"time"
	"unicode"
)

// NormalizeNFC returns the input unchanged in WASM builds.
func NormalizeNFC(s string) string {
	return s
}

// ToLower lowercases a string for WASM builds.
func ToLower(s string) string {
	return strings.ToLower(s)
}

// ToTitle title-cases a string for WASM builds.
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

// HTMLEscape escapes HTML entities in a string.
func HTMLEscape(s string) string {
	return html.EscapeString(s)
}

// NowUnix returns the current Unix timestamp.
func NowUnix() int64 {
	return time.Now().Unix()
}
