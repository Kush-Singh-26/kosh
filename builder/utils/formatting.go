package utils

import (
	"fmt"
	"sort"

	"my-ssg/builder/models"
)

func SortPosts(posts []models.PostMetadata) {
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].DateObj.Equal(posts[j].DateObj) {
			return posts[i].Title > posts[j].Title
		}
		return posts[i].DateObj.After(posts[j].DateObj)
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
