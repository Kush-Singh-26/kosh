//go:build !js || !wasm

package core

import (
	"html"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

var (
	titleCaser = cases.Title(language.English)
	lowerCaser = cases.Lower(language.Und)
)

func NormalizeNFC(s string) string {
	return norm.NFC.String(s)
}

func ToLower(s string) string {
	return lowerCaser.String(s)
}

func ToTitle(s string) string {
	return titleCaser.String(s)
}

func HTMLEscape(s string) string {
	return html.EscapeString(s)
}
