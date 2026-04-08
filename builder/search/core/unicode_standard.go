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
func NormalizeNFC(s string) string {
	return norm.NFC.String(s)
}

// ToLower lowercases a string with Unicode support.
func ToLower(s string) string {
	return lowerCaser.String(s)
}

// ToTitle title-cases a string with Unicode support.
func ToTitle(s string) string {
	return titleCaser.String(s)
}

// HTMLEscape escapes HTML entities in a string.
func HTMLEscape(s string) string {
	return html.EscapeString(s)
}

// NowUnix returns the current Unix timestamp.
func NowUnix() int64 {
	return time.Now().Unix()
}
