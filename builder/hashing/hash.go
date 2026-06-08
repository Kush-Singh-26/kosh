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

	"github.com/zeebo/xxh3"
	"gopkg.in/yaml.v3"

	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// ErrEmptyData indicates empty input for frontmatter parsing.
var ErrEmptyData = errors.New("empty data")

const (
	hashBoolTrue         = 1
	hashBoolFalse        = 0
	hashFieldSeparator   = ':'
	hashSectionSeparator = 0
	weightBytesSize      = 8
	yamlFrontmatterParts = 3
)

// GetFrontmatterHash computes the canonical frontmatter hash from the raw metadata map.
func GetFrontmatterHash(metadata map[string]any, taxonomyKeys []string) (string, error) {
	h := xxh3.New()

	standardFields := extractStandardFields(metadata, taxonomyKeys)
	hashStandardFields(HashStandardFieldsOptions{
		Hasher:      h,
		Title:       standardFields.Title,
		Description: standardFields.Description,
		Date:        standardFields.Date,
		Taxonomies:  standardFields.Taxonomies,
		IsPinned:    standardFields.IsPinned,
		IsDraft:     standardFields.IsDraft,
		Weight:      standardFields.Weight,
	})

	hashCustomFields(h, metadata, taxonomyKeys)

	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:]), nil
}

func extractStandardFields(metadata map[string]any, taxonomyKeys []string) FrontmatterHashOptions {
	opts := FrontmatterHashOptions{
		Title:       timeutil.ExtractStringFromMap(metadata, "title"),
		Description: timeutil.ExtractStringFromMap(metadata, "description"),
		Date:        timeutil.ExtractDateStringFromMap(metadata, "date"),
		Taxonomies:  make(map[string][]string),
	}

	for _, k := range taxonomyKeys {
		if terms := timeutil.ExtractSliceFromMap(metadata, k); len(terms) > 0 {
			opts.Taxonomies[k] = terms
		}
	}

	if p, ok := metadata["pinned"].(bool); ok {
		opts.IsPinned = p
	}

	if d, ok := metadata["draft"].(bool); ok {
		opts.IsDraft = d
	}

	if w, ok := metadata["weight"].(int); ok {
		opts.Weight = w
	} else if w, ok := metadata["weight"].(float64); ok {
		opts.Weight = int(w)
	}

	return opts
}

func hashCustomFields(h *xxh3.Hasher, other map[string]any, taxonomyKeys []string) {
	standardKeys := map[string]bool{
		"title": true, "description": true, "date": true,
		"pinned": true, "draft": true, "weight": true,
	}
	for _, k := range taxonomyKeys {
		standardKeys[k] = true
	}

	var keys []string
	for k := range other {
		if !standardKeys[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		writeStringXXH3(h, k)
		_, _ = h.Write([]byte{hashFieldSeparator})
		val := fmt.Sprintf("%v", other[k])
		writeStringXXH3(h, val)
		_, _ = h.Write([]byte{hashSectionSeparator})
	}
}

// FrontmatterHashOptions provides parsed frontmatter values for hashing.
type FrontmatterHashOptions struct {
	Title       string
	Description string
	Date        string
	Taxonomies  map[string][]string
	IsPinned    bool
	IsDraft     bool
	Weight      int
	// Other contains custom frontmatter fields not in the standard whitelist.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	Other map[string]any
	// TaxonomyKeys is the list of configured taxonomy keys (e.g., tags, categories).
	// These are excluded from Other to prevent double-hashing of taxonomy fields.
	TaxonomyKeys []string
}

// GetFrontmatterHashFromValues computes the canonical frontmatter hash from already-parsed values.
func GetFrontmatterHashFromValues(opts FrontmatterHashOptions) string {
	h := xxh3.New()

	hashStandardFields(HashStandardFieldsOptions{
		Hasher:      h,
		Title:       opts.Title,
		Description: opts.Description,
		Date:        opts.Date,
		Taxonomies:  opts.Taxonomies,
		IsPinned:    opts.IsPinned,
		IsDraft:     opts.IsDraft,
		Weight:      opts.Weight,
	})

	hashCustomFields(h, opts.Other, opts.TaxonomyKeys)

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
	Taxonomies  map[string][]string
	IsPinned    bool
	IsDraft     bool
	Weight      int
}

func hashStandardFields(opts HashStandardFieldsOptions) {
	writeStringXXH3(opts.Hasher, opts.Title)
	_, _ = opts.Hasher.Write([]byte{hashSectionSeparator})
	writeStringXXH3(opts.Hasher, opts.Description)
	_, _ = opts.Hasher.Write([]byte{hashSectionSeparator})

	// Normalize date to YYYY-MM-DD when parseable
	if t, err := time.Parse("2006-01-02", opts.Date); err == nil {
		writeStringXXH3(opts.Hasher, t.Format("2006-01-02"))
	} else {
		writeStringXXH3(opts.Hasher, opts.Date)
	}
	_, _ = opts.Hasher.Write([]byte{hashSectionSeparator})

	// Taxonomies: Sort taxonomy keys and their terms for deterministic hashing
	if len(opts.Taxonomies) > 0 {
		taxKeys := make([]string, 0, len(opts.Taxonomies))
		for k := range opts.Taxonomies {
			taxKeys = append(taxKeys, k)
		}
		sort.Strings(taxKeys)

		for _, k := range taxKeys {
			terms := opts.Taxonomies[k]
			if len(terms) == 0 {
				continue
			}
			writeStringXXH3(opts.Hasher, k)
			_, _ = opts.Hasher.Write([]byte{hashFieldSeparator})

			normalized := make([]string, len(terms))
			copy(normalized, terms)
			for i := range normalized {
				normalized[i] = strings.TrimSpace(normalized[i])
			}
			sort.Strings(normalized)
			for _, term := range normalized {
				writeStringXXH3(opts.Hasher, term)
				_, _ = opts.Hasher.Write([]byte{hashSectionSeparator})
			}
			_, _ = opts.Hasher.Write([]byte{hashSectionSeparator}) // End of specific taxonomy
		}
	}
	_, _ = opts.Hasher.Write([]byte{hashSectionSeparator}) // End of all taxonomies

	// Flags and numeric values
	if opts.IsPinned {
		_, _ = opts.Hasher.Write([]byte{hashBoolTrue})
	} else {
		_, _ = opts.Hasher.Write([]byte{hashBoolFalse})
	}
	if opts.IsDraft {
		_, _ = opts.Hasher.Write([]byte{hashBoolTrue})
	} else {
		_, _ = opts.Hasher.Write([]byte{hashBoolFalse})
	}

	weightBytes := make([]byte, weightBytesSize)
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
	parts := bytes.SplitN(source, YAMLDelim, yamlFrontmatterParts)
	if len(parts) >= yamlFrontmatterParts {
		return HashBytes(bytes.TrimSpace(parts[2]))
	}
	return HashBytes(source)
}

// GetFrontmatterHashFromSource extracts frontmatter from raw source and computes its hash.
// If title is missing in metadata, it uses fallbackTitle to ensure consistency with the scanner.
func GetFrontmatterHashFromSource(source []byte, fallbackTitle string, taxonomyKeys []string) (string, error) {
	parts := bytes.SplitN(source, YAMLDelim, yamlFrontmatterParts)
	if len(parts) < yamlFrontmatterParts {
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

	return GetFrontmatterHash(metadata, taxonomyKeys)
}

// ParseFrontmatter parses YAML frontmatter into a map.
// Expected value types mirror yaml.v3 decoding: string, bool, int/float64, time.Time, []any, map[string]any.
func ParseFrontmatter(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, ErrEmptyData
	}
	metadata := make(map[string]any)
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	NormalizeMetadata(metadata)
	return metadata, nil
}

// NormalizeMetadata recursively converts all time.Time values in a map to UTC.
func NormalizeMetadata(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case time.Time:
			m[k] = val.UTC()
		case map[string]any:
			NormalizeMetadata(val)
		case []any:
			for i, item := range val {
				if t, ok := item.(time.Time); ok {
					val[i] = t.UTC()
				} else if mm, ok := item.(map[string]any); ok {
					NormalizeMetadata(mm)
				}
			}
		}
	}
}
