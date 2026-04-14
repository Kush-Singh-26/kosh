package models

import (
	"io/fs"
	"time"
)

// LightResourceMetadata is a minimal resource metadata structure for site-wide discovery
// and scanning. It contains the basic fields needed to identify a resource and
// determine if it needs a full rebuild.
type LightResourceMetadata struct {
	Path        string
	Title       string
	DateObj     time.Time
	Taxonomies  map[string][]string
	IsPinned    bool
	Weight      int
	ReadingTime int
	IsDraft     bool
	Description string
	Link        string
	HTMLPath    string
	Layout      string
}

// MetadataScannerResult captures the results of a metadata scan.
type MetadataScannerResult struct {
	Metadata      []LightResourceMetadata
	TaxonomyMap   map[string]map[string][]LightResourceMetadata
	Files         []ScannedResource
	ContentAssets []ScannedAsset
	Has404        bool
}

// ScannedResource carries minimal resource info to avoid a second filesystem walk in processing.
type ScannedResource struct {
	Path            string
	RelPath         string
	Title           string
	Description     string
	Date            string
	DateObj         time.Time
	IsDraft         bool
	IsPinned        bool
	Weight          int
	Layout          string
	Taxonomies      map[string][]string
	Info            fs.FileInfo
	BodyHash        string
	FrontmatterHash string
	ReadingTime     int
	BodyOffset      int
	Link            string
	SourceLoader    func() ([]byte, error) // Lazy file loader to avoid I/O waste
	// PreParsedMeta holds YAML frontmatter values already decoded by the scanner.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	PreParsedMeta map[string]any
}

// ScannedAsset captures an asset path and its filesystem metadata.
type ScannedAsset struct {
	Path string
	Info fs.FileInfo
}
