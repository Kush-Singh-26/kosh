package timeutil

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// Slugify converts a string into a lowercase URL slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var res strings.Builder
	res.Grow(len(s))
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			res.WriteRune(r)
			lastDash = false
		} else if r == ' ' || r == '-' || r == '_' {
			if !lastDash {
				res.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(res.String(), "-")
}

// SortItems sorts content items by weight and date, descending.
func SortItems(items []models.ContentMetadata) {
	sort.Slice(items, func(i, j int) bool {
		wi, wj := items[i].Weight, items[j].Weight

		// Sort by Weight Descending (Higher weight first)
		if wi != wj {
			return wi > wj
		}

		// Use Unix timestamps for faster integer comparison
		ti, tj := items[i].DateObj.Unix(), items[j].DateObj.Unix()
		if ti == tj {
			// Title Descending if dates match (arbitrary, stable)
			return items[i].Title > items[j].Title
		}
		// Date Descending (Newer first)
		return ti > tj
	})
}

// ExtractStringFromMap returns a string value from a metadata map.
// Expected value types: string, bool, int/float64, time.Time, []any, map[string]any.
func ExtractStringFromMap(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// ExtractDateStringFromMap returns a YYYY-MM-DD string from a metadata map.
// Expected value types: string or time.Time (from YAML decoding).
func ExtractDateStringFromMap(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if t, ok := v.(time.Time); ok {
			return t.UTC().Format("2006-01-02")
		}
		if s, ok := v.(string); ok {
			if t, err := time.ParseInLocation("2006-01-02", s, time.UTC); err == nil {
				return t.Format("2006-01-02")
			}
			return s
		}
	}
	return ""
}

// ExtractSliceFromMap returns a string slice from a metadata map.
// Expected value type: []any (from YAML decoding).
func ExtractSliceFromMap(m map[string]any, k string) []string {
	var res []string
	if v, ok := m[k]; ok {
		if l, ok := v.([]any); ok {
			for _, i := range l {
				res = append(res, fmt.Sprintf("%v", i))
			}
		}
	}
	return res
}

// ExtractBoolFromMap returns a boolean value from a metadata map.
// Expected value type: bool (from YAML decoding).
func ExtractBoolFromMap(m map[string]any, k string) bool {
	if v, ok := m[k]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// CountWords returns the number of words in a byte slice using a direct byte loop
func CountWords(s []byte) int {
	count := 0
	inWord := false
	for _, b := range s {
		isSpace := b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
		if !isSpace {
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}
