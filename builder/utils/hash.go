package utils

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/zeebo/blake3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func GetFrontmatterHash(metaData map[string]interface{}) (string, error) {
	h := blake3.New()

	writeStringBlake3(h, GetString(metaData, "title"))
	_, _ = h.Write([]byte{0})
	writeStringBlake3(h, GetString(metaData, "description"))
	_, _ = h.Write([]byte{0})
	writeStringBlake3(h, GetString(metaData, "date"))
	_, _ = h.Write([]byte{0})

	// Sort in-place (caller shouldn't rely on original order)
	tags := GetSlice(metaData, "tags")
	if len(tags) > 0 {
		sort.Strings(tags)
		for _, tag := range tags {
			writeStringBlake3(h, tag)
			_, _ = h.Write([]byte{0})
		}
	}

	// Pinned flag
	if isPinned, _ := metaData["pinned"].(bool); isPinned {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeStringBlake3 writes a string to the BLAKE3 hash
func writeStringBlake3(h *blake3.Hasher, s string) {
	_, _ = h.Write([]byte(s))
}

// yamlDelim is the YAML frontmatter delimiter
var yamlDelim = []byte("---")

// GetBodyHash extracts the body content (after frontmatter) and returns its BLAKE3 hash
// This is CRITICAL for cache validity - body changes without frontmatter changes
// would otherwise be silently ignored
func GetBodyHash(source []byte) string {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) >= 3 {
		body := parts[2]
		body = bytes.TrimSpace(body)
		hash := blake3.Sum256(body)
		return hex.EncodeToString(hash[:])
	}
	hash := blake3.Sum256(source)
	return hex.EncodeToString(hash[:])
}

type postGraphInfo struct {
	Title string   `json:"title"`
	Link  string   `json:"link"`
	Tags  []string `json:"tags"`
}

func GetGraphHash(posts []models.PostMetadata) (string, error) {
	graphInfo := make([]postGraphInfo, 0, len(posts))
	for _, p := range posts {
		graphInfo = append(graphInfo, postGraphInfo{
			Title: p.Title,
			Link:  p.Link,
			Tags:  p.Tags,
		})
	}

	data, err := json.Marshal(graphInfo)
	if err != nil {
		return "", err
	}

	hash := blake3.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
