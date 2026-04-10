//go:build !js || !wasm

package core

import (
	"html"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

var (
	titleCaser = cases.Title(language.English)
	lowerCaser = cases.Lower(language.Und)
)

// NormalizeNFC normalizes a string to NFC form.
func NormalizeNFC(text string) string {
	return norm.NFC.String(text)
}

// ToLower lowercases a string with Unicode support.
func ToLower(text string) string {
	return lowerCaser.String(text)
}

// ToTitle title-cases a string with Unicode support.
func ToTitle(text string) string {
	return titleCaser.String(text)
}

// HTMLEscape escapes HTML entities in a string.
func HTMLEscape(text string) string {
	return html.EscapeString(text)
}

// NowUnix returns the current Unix timestamp.
func NowUnix() int64 {
	return time.Now().Unix()
}
