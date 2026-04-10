package models

import (
	"io/fs"
	"time"
)

// LightPostMetadata is a minimal post metadata structure for site-wide discovery
// and scanning. It contains the basic fields needed to identify a post and
// determine if it needs a full rebuild.
type LightPostMetadata struct {
	Path        string
	Title       string
	DateObj     time.Time
	Tags        []string
	IsPinned    bool
	Weight      int
	ReadingTime int
	IsDraft     bool
	Description string
	Link        string
	HTMLPath    string
}

// MetadataScannerResult captures the results of a metadata scan.
type MetadataScannerResult struct {
	Metadata      []LightPostMetadata
	TagMap        map[string][]LightPostMetadata
	Files         []ScannedFile
	ContentAssets []ScannedAsset
	Has404        bool
}

// ScannedFile carries minimal file info to avoid a second filesystem walk in post processing.
type ScannedFile struct {
	Path            string
	RelPath         string
	Title           string
	Description     string
	Date            string
	IsDraft         bool
	IsPinned        bool
	Weight          int
	Tags            []string
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
