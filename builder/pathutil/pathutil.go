package pathutil

import "strings"

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
