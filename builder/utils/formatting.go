package utils

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var res strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			res.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			res.WriteRune('-')
		}
	}
	// Replace double dashes
	str := res.String()
	for strings.Contains(str, "--") {
		str = strings.ReplaceAll(str, "--", "-")
	}
	return strings.Trim(str, "-")
}

func SortPosts(posts []models.PostMetadata) {
	sort.Slice(posts, func(i, j int) bool {
		wi, wj := posts[i].Weight, posts[j].Weight

		// Sort by Weight Descending (Higher weight first)
		if wi != wj {
			return wi > wj
		}

		// Use Unix timestamps for faster integer comparison
		ti, tj := posts[i].DateObj.Unix(), posts[j].DateObj.Unix()
		if ti == tj {
			// Title Descending if dates match (arbitrary, stable)
			return posts[i].Title > posts[j].Title
		}
		// Date Descending (Newer first)
		return ti > tj
	})
}

func GetString(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func GetSlice(m map[string]interface{}, k string) []string {
	var res []string
	if v, ok := m[k]; ok {
		if l, ok := v.([]interface{}); ok {
			for _, i := range l {
				res = append(res, fmt.Sprintf("%v", i))
			}
		}
	}
	return res
}

func GetBool(m map[string]interface{}, k string) bool {
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
