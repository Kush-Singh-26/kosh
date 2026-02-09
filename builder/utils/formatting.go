package utils

import (
	"fmt"
	"sort"

	"my-ssg/builder/models"
)

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
