//go:build js && wasm

package core

import (
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

// htmlEscaper mirrors html.EscapeString without linking the html package.
var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"'", "&#39;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&#34;",
)

// HTMLEscape escapes HTML entities in a string.
func HTMLEscape(text string) string {
	if !strings.ContainsAny(text, "&'<>\"") {
		return text
	}
	return htmlEscaper.Replace(text)
}

// NowUnix returns the current Unix timestamp.
func NowUnix() int64 {
	return time.Now().Unix()
}
