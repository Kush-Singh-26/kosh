// defines the data structures used by templates and generators
package models

//go:generate msgp

import (
	"encoding/xml"
	"html/template"
	"io/fs"
	"time"
)

//msgp:ignore TreeNode Breadcrumb NavPage VersionInfo PostMetadata TagData Paginator PageData UrlSet Url Rss Channel Item GraphNode GraphLink GraphData LightPostMetadata MetadataScannerResult ScannedFile ScannedAsset MenuEntry AuthorConfig GeneratorsConfig FeaturesConfig SocialCardsConfig

// LightPostMetadata is a minimal post metadata structure for site-wide discovery
// and scanning. It contains the basic fields needed to identify a post and
// determine if it needs a full rebuild.
type LightPostMetadata struct {
	Path        string
	Version     string
	Title       string
	DateObj     time.Time
	Tags        []string
	Pinned      bool
	Weight      int
	ReadingTime int
	Draft       bool
	Description string
	Link        string
	HTMLPath    string
}

type MetadataScannerResult struct {
	Metadata       []LightPostMetadata
	TagMap         map[string][]LightPostMetadata
	PostsByVersion map[string][]LightPostMetadata
	Files          []ScannedFile
	ContentAssets  []ScannedAsset
	Has404         bool
}

// ScannedFile carries minimal file info to avoid a second filesystem walk in post processing.
type ScannedFile struct {
	Path            string
	RelPath         string
	Version         string
	Title           string
	Description     string
	Date            string
	Draft           bool
	Pinned          bool
	Weight          int
	Tags            []string
	Info            fs.FileInfo
	BodyHash        string
	FrontmatterHash string
	ReadingTime     int
	BodyOffset      int
	Link            string
	Source          []byte         // Pre-read source bytes to avoid double-read
	PreParsedMeta   map[string]any // Pre-parsed frontmatter to avoid double-parse
}

type ScannedAsset struct {
	Path string
	Info fs.FileInfo
}

// TOCEntry represents a table of contents entry
type TOCEntry struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
}

type TreeNode struct {
	Title     string      `json:"title"`
	Link      string      `json:"link"`
	Weight    int         `json:"weight"`
	Children  []*TreeNode `json:"children"`
	Active    bool        `json:"active"`     // For template helper
	IsSection bool        `json:"is_section"` // True if node has children
}

type Breadcrumb struct {
	Title     string
	Link      string
	IsCurrent bool
}

type NavPage struct {
	Title string
	Link  string
}

type VersionInfo struct {
	Name      string
	Path      string // Raw version path (e.g., "v7.0")
	URL       string
	IsLatest  bool
	IsCurrent bool
}

// PostMetadata represents the frontmatter and derived data of a markdown post
// for template rendering. It is the primary data structure passed to HTML templates
// for displaying post lists, navigation, and page content.
type PostMetadata struct {
	Title       string
	Link        string
	Description string
	Tags        []string
	Weight      int
	ReadingTime int
	Pinned      bool
	Draft       bool
	DateObj     time.Time
	Version     string // "v2.0", "v1.0", "" for latest
}

type TagData struct {
	Name  string
	Link  string
	Count int
}

type Paginator struct {
	CurrentPage int
	TotalPages  int
	PrevURL     string
	NextURL     string
	FirstURL    string
	LastURL     string
	HasPrev     bool
	HasNext     bool
}

// PageData is the context passed to HTML templates.
type PageData struct {
	Title        string
	TabTitle     string
	Description  string
	BaseURL      string
	Content      template.HTML
	Meta         map[string]any
	IsIndex      bool
	IsTagsIndex  bool
	Posts        []PostMetadata
	PinnedPosts  []PostMetadata
	AllTags      []TagData
	BuildVersion int64
	Permalink    string
	Image        string
	TOC          []TOCEntry
	SiteTree     []*TreeNode
	SidebarHTML  template.HTML
	Paginator    Paginator
	Assets       map[string]string
	Weight       int
	ReadingTime  int
	HasImages    bool

	// Navigation
	Breadcrumbs []Breadcrumb
	PrevPage    *NavPage
	NextPage    *NavPage

	// Depth-aware pathing
	RelativePrefix string // e.g., "../" for depth 1

	// Versioning
	CurrentVersion string
	Versions       []VersionInfo
	IsOutdated     bool

	// Config-driven fields
	Config TemplateConfig // To access Config fields in templates (Menu, Author, etc.)

	// SEO
	JSONLD template.HTML
}

// MenuEntry defines a single menu item in site navigation.
type MenuEntry struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url,omitempty"`
	Target string `yaml:"target,omitempty"`
	ID     string `yaml:"id,omitempty"`
	Class  string `yaml:"class,omitempty"`
}

// AuthorConfig defines site author information.
type AuthorConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// GeneratorsConfig enables/disables site-wide generators.
type GeneratorsConfig struct {
	Sitemap bool `yaml:"sitemap"`
	RSS     bool `yaml:"rss"`
	Graph   bool `yaml:"graph"`
	PWA     bool `yaml:"pwa"`
	Search  bool `yaml:"search"`
}

// FeaturesConfig enables/disables site features.
type FeaturesConfig struct {
	RawMarkdown bool             `yaml:"rawMarkdown"`
	Generators  GeneratorsConfig `yaml:"generators"`
}

// SocialCardsConfig defines visual parameters for social card generation.
type SocialCardsConfig struct {
	Background string   `yaml:"background"`
	Gradient   []string `yaml:"gradient"`
	Angle      int      `yaml:"angle"`
	TextColor  string   `yaml:"textColor"`
}

// TemplateConfig defines the strictly-typed subset of project configuration
// accessible within HTML templates. This prevents tight coupling between
// models and the main config package while restoring type safety.
type TemplateConfig interface {
	GetMenu() []MenuEntry
	GetAuthor() AuthorConfig
	GetSocial() SocialCardsConfig
	GetFeatures() FeaturesConfig
	GetSiteTitle() string
	GetBaseURL() string
}

// --- Sitemap Structures ---

// URLSet represents a sitemap urlset.
type URLSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []URL    `xml:"url"`
}

// URL represents a sitemap url entry.
type URL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// --- RSS Structures ---

type Rss struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Items       []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Guid        string `xml:"guid"`
}

// --- Graph Data Structures ---

type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group int    `json:"group"` // 1 for Posts, 2 for Tags
	Value int    `json:"val"`   // Size of the node
	URL   string `json:"url,omitempty"`
}

type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// PostRecord represents a search-optimized record for BM25 indexing and
// search functionality. It contains normalized fields for efficient text
// matching and version-aware search.
type PostRecord struct {
	// ID is a uint64 representation of the post's link, used for compact
	// in-memory indexing. Note that search.bin uses decimal strings of this ID
	// for serialization compatibility, while BoltDB cache uses 128-bit hex strings.
	ID              uint64
	Title           string
	NormalizedTitle string // Lowercase title for search
	Link            string
	Description     string
	Tags            []string
	NormalizedTags  []string // Lowercase tags for search
	Content         string   // Raw plain text for snippet extraction
	Version         string   // Version scoping
}

// IndexedPost bundles a search record with pre-computed word frequencies for BM25
type IndexedPost struct {
	Record          PostRecord
	SourcePath      string `msgp:"-"`
	WordFreqs       map[string]int
	DocLen          int
	StemMap         map[string]string // original word -> stem
	PositionalIndex map[string][]int  // used during indexing; encoded to uint32 for storage
	ByteOffsets     map[string][]int  // word -> [start, end, start, end...]
}

const CurrentSchemaVersion = 11

type SearchIndex struct {
	SchemaVersion int64
	Posts         map[string]PostRecord
	DocLens       map[string]int64 // postID (string) -> word count
	AvgDocLen     float64
	TotalDocs     int64
	StemMap       map[string][]string // stemmed -> original forms
	NgramIndex    map[string][]string // trigram -> terms (for fuzzy search)

	// Inverted Index: word -> postID (string) -> delta-encoded positions
	// Deltas: [pos1, pos2-pos1, pos3-pos2, ...] (v11+)
	Inverted map[string]map[string][]uint32

	// Byte offsets map: word -> postID (string) -> delta-encoded offsets
	// Format: [start1, length1, start2-start1, length2, ...] (v11+)
	Offsets map[string]map[string][]uint32
}

// DecodePositions decodes delta-encoded positions into absolute positions
func DecodePositions(deltas []uint32) []int {
	if len(deltas) == 0 {
		return nil
	}
	positions := make([]int, len(deltas))
	positions[0] = int(deltas[0])
	for i := 1; i < len(deltas); i++ {
		positions[i] = positions[i-1] + int(deltas[i])
	}
	return positions
}

// DecodeOffsets decodes delta-encoded byte offsets into [start, end, start, end, ...]
func DecodeOffsets(deltas []uint32) []int {
	if len(deltas) == 0 {
		return nil
	}
	n := len(deltas) / 2
	result := make([]int, 0, n*2)
	absStart := int(deltas[0])
	for i := 0; i < n; i++ {
		length := int(deltas[i*2+1])
		result = append(result, absStart, absStart+length)
		if i+1 < n {
			absStart += int(deltas[i*2+2])
		}
	}
	return result
}

// EncodePositions encodes absolute positions into delta format
func EncodePositions(positions []int) []uint32 {
	if len(positions) == 0 {
		return nil
	}
	deltas := make([]uint32, len(positions))
	deltas[0] = uint32(positions[0])
	for i := 1; i < len(positions); i++ {
		deltas[i] = uint32(positions[i] - positions[i-1])
	}
	return deltas
}

// EncodeOffsets encodes [start, end, start, end, ...] into delta format
func EncodeOffsets(pairs []int) []uint32 {
	if len(pairs) == 0 {
		return nil
	}
	deltas := make([]uint32, 0, len(pairs))
	for i := 0; i < len(pairs); i += 2 {
		start := pairs[i]
		end := pairs[i+1]
		length := end - start
		if i == 0 {
			deltas = append(deltas, uint32(start), uint32(length))
		} else {
			deltas = append(deltas, uint32(start-pairs[i-2]), uint32(length))
		}
	}
	return deltas
}
