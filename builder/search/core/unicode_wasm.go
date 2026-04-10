//go:build js && wasm

package core

import (
	"html"
	"strings"
	"time"
	"unicode"
)

// NormalizeNFC returns the input unchanged in WASM builds.
func NormalizeNFC(text string) string {
	return text
}

// ToLower lowercases a string for WASM builds.
func ToLower(text string) string {
	return strings.ToLower(text)
}

// ToTitle title-cases a string for WASM builds.
func ToTitle(text string) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) == 1 {
		return string(unicode.ToUpper(runes[0]))
	}
	return string(unicode.ToUpper(runes[0])) + strings.ToLower(string(runes[1:]))
}

// HTMLEscape escapes HTML entities in a string.
func HTMLEscape(text string) string {
	return html.EscapeString(text)
}

// NowUnix returns the current Unix timestamp.
func NowUnix() int64 {
	return time.Now().Unix()
}
