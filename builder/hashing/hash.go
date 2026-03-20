package hashing

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/zeebo/xxh3"
	"gopkg.in/yaml.v3"
	"sort"
	"strings"
	"time"
)

var ErrEmptyData = errors.New("empty data")

func GetFrontmatterHash(metadata map[string]any) (string, error) {
	h := xxh3.New()

	writeStringXXH3(h, timeutil.ExtractStringFromMap(metadata, "title"))
	_, _ = h.Write([]byte{0})
	writeStringXXH3(h, timeutil.ExtractStringFromMap(metadata, "description"))
	_, _ = h.Write([]byte{0})

	// Handle date explicitly to avoid timezone-related non-determinism
	if dateVal, ok := metadata["date"].(time.Time); ok {
		writeStringXXH3(h, dateVal.Format("2006-01-02"))
	} else {
		writeStringXXH3(h, timeutil.ExtractStringFromMap(metadata, "date"))
	}
	_, _ = h.Write([]byte{0})

	// Sort in-place (caller shouldn't rely on original order)
	tags := timeutil.ExtractSliceFromMap(metadata, "tags")
	if len(tags) > 0 {
		sort.Strings(tags)
		for _, tag := range tags {
			writeStringXXH3(h, tag)
			_, _ = h.Write([]byte{0})
		}
	}

	// Pinned flag
	if isPinned, _ := metadata["pinned"].(bool); isPinned {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:]), nil
}

// GetFrontmatterHashFromValues computes the canonical frontmatter hash from already-parsed values.
// This avoids reparsing YAML when scanner-stage frontmatter is already available.
func GetFrontmatterHashFromValues(title, description, date string, tags []string, pinned bool) string {
	h := xxh3.New()

	writeStringXXH3(h, title)
	_, _ = h.Write([]byte{0})
	writeStringXXH3(h, description)
	_, _ = h.Write([]byte{0})

	// Keep date canonicalization aligned with GetFrontmatterHash:
	// normalize to YYYY-MM-DD when parseable, otherwise use raw string.
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

	if pinned {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:])
}

// writeStringXXH3 writes a string to the XXH3 hash
func writeStringXXH3(h *xxh3.Hasher, s string) {
	_, _ = h.Write([]byte(s))
}

// yamlDelim is the YAML frontmatter delimiter
var yamlDelim = []byte("---")

func HashBytes(data []byte) string {
	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:])
}

// GetBodyHash extracts the body content (after frontmatter) and returns its XXH3 hash
// This is CRITICAL for cache validity - body changes without frontmatter changes
// would otherwise be silently ignored
func GetBodyHash(source []byte) string {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) >= 3 {
		return HashBytes(bytes.TrimSpace(parts[2]))
	}
	return HashBytes(source)
}

// GetFrontmatterHashFromSource extracts frontmatter from raw source and computes its hash
// This enables cache invalidation without full markdown parsing
func GetFrontmatterHashFromSource(source []byte) (string, error) {
	parts := bytes.SplitN(source, yamlDelim, 3)
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
