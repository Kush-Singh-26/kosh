package hashing

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/zeebo/xxh3"
	"gopkg.in/yaml.v3"
)

var ErrEmptyData = errors.New("empty data")

// GetFrontmatterHash computes the canonical frontmatter hash from the raw metadata map.
// It includes whitelisted standard fields with normalization and a catch-all for custom fields.
func GetFrontmatterHash(metadata map[string]any) (string, error) {
	h := xxh3.New()

	// 1. Standard Whitelisted Fields (Normalized)
	title := timeutil.ExtractStringFromMap(metadata, "title")
	description := timeutil.ExtractStringFromMap(metadata, "description")
	date := timeutil.ExtractDateStringFromMap(metadata, "date")
	tags := timeutil.ExtractSliceFromMap(metadata, "tags")

	pinned := false
	if p, ok := metadata["pinned"].(bool); ok {
		pinned = p
	}

	draft := false
	if d, ok := metadata["draft"].(bool); ok {
		draft = d
	}

	weight := 0
	if w, ok := metadata["weight"].(int); ok {
		weight = w
	} else if w, ok := metadata["weight"].(float64); ok {
		weight = int(w)
	}

	hashStandardFields(h, title, description, date, tags, pinned, draft, weight)

	// 2. Catch-all for Custom Fields
	// Sort keys to ensure deterministic hashing
	var keys []string
	standardKeys := map[string]bool{
		"title": true, "description": true, "date": true, "tags": true,
		"pinned": true, "draft": true, "weight": true,
	}

	for k := range metadata {
		if !standardKeys[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		writeStringXXH3(h, k)
		_, _ = h.Write([]byte{':'})
		// Convert value to string representation for hashing
		val := fmt.Sprintf("%v", metadata[k])
		writeStringXXH3(h, val)
		_, _ = h.Write([]byte{0})
	}

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:]), nil
}

// GetFrontmatterHashFromValues computes the canonical frontmatter hash from already-parsed values.
// This avoids reparsing YAML when scanner-stage frontmatter is already available.
func GetFrontmatterHashFromValues(title, description, date string, tags []string, pinned, draft bool, weight int, other map[string]any) string {
	h := xxh3.New()

	hashStandardFields(h, title, description, date, tags, pinned, draft, weight)

	// Catch-all for custom fields
	var keys []string
	standardKeys := map[string]bool{
		"title": true, "description": true, "date": true, "tags": true,
		"pinned": true, "draft": true, "weight": true,
	}

	for k := range other {
		if !standardKeys[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		writeStringXXH3(h, k)
		_, _ = h.Write([]byte{':'})
		val := fmt.Sprintf("%v", other[k])
		writeStringXXH3(h, val)
		_, _ = h.Write([]byte{0})
	}

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:])
}

func hashStandardFields(h *xxh3.Hasher, title, description, date string, tags []string, pinned, draft bool, weight int) {
	writeStringXXH3(h, title)
	_, _ = h.Write([]byte{0})
	writeStringXXH3(h, description)
	_, _ = h.Write([]byte{0})

	// Normalize date to YYYY-MM-DD when parseable
	if t, err := time.Parse("2006-01-02", date); err == nil {
		writeStringXXH3(h, t.Format("2006-01-02"))
	} else {
		writeStringXXH3(h, date)
	}
	_, _ = h.Write([]byte{0})

	if len(tags) > 0 {
		normalized := make([]string, len(tags))
		copy(normalized, tags)
		for i := range normalized {
			normalized[i] = strings.TrimSpace(normalized[i])
		}
		sort.Strings(normalized)
		for _, tag := range normalized {
			writeStringXXH3(h, tag)
			_, _ = h.Write([]byte{0})
		}
	}
	_, _ = h.Write([]byte{0})

	// Flags and numeric values
	if pinned {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
	if draft {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}

	weightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(weightBytes, uint64(weight))
	_, _ = h.Write(weightBytes)
}

// writeStringXXH3 writes a string to the XXH3 hash
func writeStringXXH3(h *xxh3.Hasher, s string) {
	_, _ = h.Write([]byte(s))
}

// YAMLDelim is the YAML frontmatter delimiter
var YAMLDelim = []byte("---")

func HashBytes(data []byte) string {
	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:])
}

// GetBodyHash extracts the body content (after frontmatter) and returns its XXH3 hash
// This is CRITICAL for cache validity - body changes without frontmatter changes
// would otherwise be silently ignored
func GetBodyHash(source []byte) string {
	parts := bytes.SplitN(source, YAMLDelim, 3)
	if len(parts) >= 3 {
		return HashBytes(bytes.TrimSpace(parts[2]))
	}
	return HashBytes(source)
}

// GetFrontmatterHashFromSource extracts frontmatter from raw source and computes its hash.
// If title is missing in metadata, it uses fallbackTitle to ensure consistency with the scanner.
func GetFrontmatterHashFromSource(source []byte, fallbackTitle string) (string, error) {
	parts := bytes.SplitN(source, YAMLDelim, 3)
	if len(parts) < 3 {
		return "", nil
	}

	frontmatter := bytes.TrimSpace(parts[1])
	metadata, err := ParseFrontmatter(frontmatter)
	if err != nil {
		if errors.Is(err, ErrEmptyData) {
			return "", nil
		}
		return "", err
	}

	// Ensure title is set if missing, to match scanner behavior
	if _, ok := metadata["title"]; !ok || metadata["title"] == "" {
		metadata["title"] = fallbackTitle
	}

	return GetFrontmatterHash(metadata)
}

func ParseFrontmatter(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, ErrEmptyData
	}
	metadata := make(map[string]any)
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}
