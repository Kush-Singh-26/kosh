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

// ErrEmptyData indicates empty input for frontmatter parsing.
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

	hashStandardFields(HashStandardFieldsOptions{
		Hasher:      h,
		Title:       title,
		Description: description,
		Date:        date,
		Tags:        tags,
		Pinned:      pinned,
		Draft:       draft,
		Weight:      weight,
	})

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

// FrontmatterHashOptions provides parsed frontmatter values for hashing.
type FrontmatterHashOptions struct {
	Title       string
	Description string
	Date        string
	Tags        []string
	Pinned      bool
	Draft       bool
	Weight      int
	Other       map[string]any
}

// GetFrontmatterHashFromValues computes the canonical frontmatter hash from already-parsed values.
// This avoids reparsing YAML when scanner-stage frontmatter is already available.
func GetFrontmatterHashFromValues(opts FrontmatterHashOptions) string {
	h := xxh3.New()

	hashStandardFields(HashStandardFieldsOptions{
		Hasher:      h,
		Title:       opts.Title,
		Description: opts.Description,
		Date:        opts.Date,
		Tags:        opts.Tags,
		Pinned:      opts.Pinned,
		Draft:       opts.Draft,
		Weight:      opts.Weight,
	})

	// Catch-all for custom fields
	var keys []string
	standardKeys := map[string]bool{
		"title": true, "description": true, "date": true, "tags": true,
		"pinned": true, "draft": true, "weight": true,
	}

	for k := range opts.Other {
		if !standardKeys[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		writeStringXXH3(h, k)
		_, _ = h.Write([]byte{':'})
		val := fmt.Sprintf("%v", opts.Other[k])
		writeStringXXH3(h, val)
		_, _ = h.Write([]byte{0})
	}

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:])
}

// HashStandardFieldsOptions configures hashing of standard frontmatter fields.
type HashStandardFieldsOptions struct {
	Hasher      *xxh3.Hasher
	Title       string
	Description string
	Date        string
	Tags        []string
	Pinned      bool
	Draft       bool
	Weight      int
}

func hashStandardFields(opts HashStandardFieldsOptions) {
	writeStringXXH3(opts.Hasher, opts.Title)
	_, _ = opts.Hasher.Write([]byte{0})
	writeStringXXH3(opts.Hasher, opts.Description)
	_, _ = opts.Hasher.Write([]byte{0})

	// Normalize date to YYYY-MM-DD when parseable
	if t, err := time.Parse("2006-01-02", opts.Date); err == nil {
		writeStringXXH3(opts.Hasher, t.Format("2006-01-02"))
	} else {
		writeStringXXH3(opts.Hasher, opts.Date)
	}
	_, _ = opts.Hasher.Write([]byte{0})

	if len(opts.Tags) > 0 {
		normalized := make([]string, len(opts.Tags))
		copy(normalized, opts.Tags)
		for i := range normalized {
			normalized[i] = strings.TrimSpace(normalized[i])
		}
		sort.Strings(normalized)
		for _, tag := range normalized {
			writeStringXXH3(opts.Hasher, tag)
			_, _ = opts.Hasher.Write([]byte{0})
		}
	}
	_, _ = opts.Hasher.Write([]byte{0})

	// Flags and numeric values
	if opts.Pinned {
		_, _ = opts.Hasher.Write([]byte{1})
	} else {
		_, _ = opts.Hasher.Write([]byte{0})
	}
	if opts.Draft {
		_, _ = opts.Hasher.Write([]byte{1})
	} else {
		_, _ = opts.Hasher.Write([]byte{0})
	}

	weightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(weightBytes, uint64(opts.Weight))
	_, _ = opts.Hasher.Write(weightBytes)
}

// writeStringXXH3 writes a string to the XXH3 hash
func writeStringXXH3(h *xxh3.Hasher, s string) {
	_, _ = h.Write([]byte(s))
}

// YAMLDelim is the YAML frontmatter delimiter
var YAMLDelim = []byte("---")

// HashBytes returns a stable hash for the provided bytes.
func HashBytes(data []byte) string {
	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:])
}

// GetBodyHash extracts the body content (after frontmatter) and returns its XXH3 hash.
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

// ParseFrontmatter parses YAML frontmatter into a map.
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
