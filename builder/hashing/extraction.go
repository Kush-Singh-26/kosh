package hashing

import "fmt"

func ExtractStringFromMap(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

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

func ExtractBoolFromMap(m map[string]any, k string) bool {
	if v, ok := m[k]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
