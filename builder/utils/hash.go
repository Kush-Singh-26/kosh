package utils

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/zeebo/xxh3"
	"gopkg.in/yaml.v3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func GetFrontmatterHash(metaData map[string]interface{}) (string, error) {
	h := xxh3.New()

	writeStringXXH3(h, GetString(metaData, "title"))
	_, _ = h.Write([]byte{0})
	writeStringXXH3(h, GetString(metaData, "description"))
	_, _ = h.Write([]byte{0})

	// Handle date explicitly to avoid timezone-related non-determinism
	if dateVal, ok := metaData["date"].(time.Time); ok {
		writeStringXXH3(h, dateVal.Format("2006-01-02"))
	} else {
		writeStringXXH3(h, GetString(metaData, "date"))
	}
	_, _ = h.Write([]byte{0})

	// Sort in-place (caller shouldn't rely on original order)
	tags := GetSlice(metaData, "tags")
	if len(tags) > 0 {
		sort.Strings(tags)
		for _, tag := range tags {
			writeStringXXH3(h, tag)
			_, _ = h.Write([]byte{0})
		}
	}

	// Pinned flag
	if isPinned, _ := metaData["pinned"].(bool); isPinned {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:]), nil
}

// writeStringXXH3 writes a string to the XXH3 hash
func writeStringXXH3(h *xxh3.Hasher, s string) {
	_, _ = h.Write([]byte(s))
}

// yamlDelim is the YAML frontmatter delimiter
var yamlDelim = []byte("---")

// GetBodyHash extracts the body content (after frontmatter) and returns its XXH3 hash
// This is CRITICAL for cache validity - body changes without frontmatter changes
// would otherwise be silently ignored
func GetBodyHash(source []byte) string {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) >= 3 {
		body := parts[2]
		body = bytes.TrimSpace(body)
		hash := xxh3.Hash128(body)
		b := hash.Bytes()
		return hex.EncodeToString(b[:])
	}
	hash := xxh3.Hash128(source)
	b := hash.Bytes()
	return hex.EncodeToString(b[:])
}

// GetFrontmatterHashFromSource extracts frontmatter from raw source and computes its hash
// This enables cache invalidation without full markdown parsing
func GetFrontmatterHashFromSource(source []byte) (string, error) {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) < 3 {
		return "", nil
	}

	frontmatter := bytes.TrimSpace(parts[1])
	metaData, err := parseFrontmatter(frontmatter)
	if err != nil || metaData == nil {
		return "", nil
	}
	return GetFrontmatterHash(metaData)
}

func parseFrontmatter(data []byte) (map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}
	metaData := make(map[string]interface{})
	if err := yaml.Unmarshal(data, &metaData); err != nil {
		return nil, err
	}
	return metaData, nil
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

	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:]), nil
}
