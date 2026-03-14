package utils

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

// Global Minifier Instance
var Minifier *minify.M

func InitMinifier() {
	Minifier = minify.New()
	// Configure HTML minifier to keep end tags for tables
	// Without this, </td>, </th>, </tr> are stripped which breaks table rendering
	htmlMinifier := &html.Minifier{
		KeepEndTags: true,
	}
	Minifier.Add("text/html", htmlMinifier)
}

func ProcessHTML(htmlStr string, baseURL string, prefix string, compress bool) string {
	// Quick check for <img> tag to avoid allocations
	if !strings.Contains(htmlStr, "<img") && !strings.Contains(htmlStr, "<IMG") {
		return htmlStr
	}
	return string(ProcessHTMLBytes([]byte(htmlStr), baseURL, prefix, compress))
}

func ProcessHTMLBytes(htmlBytes []byte, baseURL string, prefix string, compress bool) []byte {
	// Case-insensitive check without full ToLower allocation
	idx := bytes.Index(htmlBytes, []byte("<img"))
	if idx == -1 {
		idx = bytes.Index(htmlBytes, []byte("<IMG"))
	}
	if idx == -1 {
		return htmlBytes
	}

	var result bytes.Buffer
	result.Grow(len(htmlBytes))

	curr := 0
	for {
		// Find next <img (case insensitive)
		idx := -1
		searchStart := curr
		for {
			relativeIdx := bytes.IndexByte(htmlBytes[searchStart:], '<')
			if relativeIdx == -1 {
				break
			}
			absoluteIdx := searchStart + relativeIdx
			if absoluteIdx+4 > len(htmlBytes) {
				break
			}
			// Check for <img or <IMG
			if (htmlBytes[absoluteIdx+1] == 'i' || htmlBytes[absoluteIdx+1] == 'I') &&
				(htmlBytes[absoluteIdx+2] == 'm' || htmlBytes[absoluteIdx+2] == 'M') &&
				(htmlBytes[absoluteIdx+3] == 'g' || htmlBytes[absoluteIdx+3] == 'G') {
				idx = absoluteIdx
				break
			}
			searchStart = absoluteIdx + 1
		}

		if idx == -1 {
			result.Write(htmlBytes[curr:])
			break
		}

		result.Write(htmlBytes[curr:idx])

		// Find end of tag
		tagEnd := bytes.IndexByte(htmlBytes[idx:], '>')
		if tagEnd == -1 {
			result.Write(htmlBytes[idx:])
			break
		}

		tag := htmlBytes[idx : idx+tagEnd+1]
		curr = idx + tagEnd + 1

		// Find src attribute within the tag
		srcIdx := -1
		quote := byte(0)

		// Scan for src= (Must have something between <img and src)
		// We can use bytes.Index here for "src=" or "SRC="
		sIdx := bytes.Index(tag, []byte("src="))
		if sIdx == -1 {
			sIdx = bytes.Index(tag, []byte("SRC="))
		}

		if sIdx != -1 && sIdx >= 4 { // Minimum <img src=
			if sIdx+4 < len(tag) && (tag[sIdx+4] == '"' || tag[sIdx+4] == '\'') {
				srcIdx = sIdx
				quote = tag[sIdx+4]
			}
		}

		if srcIdx == -1 {
			result.Write(tag)
			continue
		}

		valStart := srcIdx + 5
		valEnd := bytes.IndexByte(tag[valStart:], quote)
		if valEnd == -1 {
			result.Write(tag)
			continue
		}

		// Extract and process src value
		srcBytes := tag[valStart : valStart+valEnd]
		src := string(srcBytes)

		if src != "" && !strings.HasPrefix(src, "http") && !strings.HasPrefix(src, "//") && !strings.HasPrefix(src, "data:") {
			// 1. WebP conversion (only for local images)
			if compress {
				ext := strings.ToLower(filepath.Ext(src))
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
					src = src[:len(src)-len(ext)] + ".webp"
				}
			}

			// 2. Path correction (Relativize or Prepend BaseURL)
			if after, ok := strings.CutPrefix(src, "/"); ok {
				if baseURL == "" {
					src = prefix + after
				} else {
					src = baseURL + src
				}
			}

			// 3. Lowercase local images to match NormalizePath behavior on Windows
			src = strings.ToLower(src)
		}

		// Reassemble the tag with the modified src
		result.Write(tag[:valStart])
		result.WriteString(src)
		result.Write(tag[valStart+valEnd:])
	}

	return result.Bytes()
}
