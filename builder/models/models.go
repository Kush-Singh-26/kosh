// defines the data structures used by templates and generators
package models

import (
	"encoding/xml"
	"html/template"
	"time"
)

// --- TOC Structure ---
// TOCEntry represents a table of contents entry
// This unified type is used by both models and cache packages to avoid conversions
type TOCEntry struct {
	ID    string `msgpack:"id" json:"id"`
	Text  string `msgpack:"text" json:"text"`
	Level int    `msgpack:"level" json:"level"`
}

// TreeNode represents a node in the site hierarchy (Sidebar)
type TreeNode struct {
	Title     string      `json:"title"`
	Link      string      `json:"link"`
	Weight    int         `json:"weight"`
	Children  []*TreeNode `json:"children"`
	Active    bool        `json:"active"`     // For template helper
	IsSection bool        `json:"is_section"` // True if node has children
}

// Breadcrumb represents a single breadcrumb item
type Breadcrumb struct {
	Title     string
	Link      string
	IsCurrent bool
}

// NavPage represents a navigation link (prev/next)
type NavPage struct {
	Title string
	Link  string
}

// VersionInfo represents a version for the version selector
type VersionInfo struct {
	Name      string
	Path      string // Raw version path (e.g., "v7.0")
	URL       string
	IsLatest  bool
	IsCurrent bool
}

// PostMetadata represents the frontmatter and derived data of a markdown post.
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

// TagData represents a tag and its frequency.
type TagData struct {
	Name  string
	Link  string
	Count int
}

// Paginator holds state for pagination
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
	Meta         map[string]interface{}
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
	Paginator    Paginator
	Assets       map[string]string
	Weight       int
	ReadingTime  int

	// Navigation
	Breadcrumbs []Breadcrumb
	PrevPage    *NavPage
	NextPage    *NavPage

	// Versioning
	CurrentVersion string
	Versions       []VersionInfo
	IsOutdated     bool

	// Config-driven fields
	Config interface{} // To access Config fields in templates (Menu, Author, etc.)
}

// --- Sitemap Structures ---

type UrlSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	Urls    []Url    `xml:"url"`
}

type Url struct {
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

// --- Search Structures ---

type PostRecord struct {
	ID              int      `msgpack:"id"`
	Title           string   `msgpack:"title"`
	NormalizedTitle string   `msgpack:"norm_title"` // Lowercase title for search
	Link            string   `msgpack:"link"`
	Description     string   `msgpack:"desc"`
	Tags            []string `msgpack:"tags"`
	NormalizedTags  []string `msgpack:"norm_tags"` // Lowercase tags for search
	Content         string   `msgpack:"content"`   // Raw plain text for snippet extraction
	Version         string   `msgpack:"ver"`       // Version scoping
}

// IndexedPost bundles a search record with pre-computed word frequencies for BM25
type IndexedPost struct {
	Record    PostRecord     `msgpack:"rec"`
	WordFreqs map[string]int `msgpack:"freqs"`
	DocLen    int            `msgpack:"len"`
}

type SearchIndex struct {
	Posts      []PostRecord           `msgpack:"posts"`
	Inverted   map[string]map[int]int `msgpack:"inv"`  // word -> postID -> frequency
	DocLens    map[int]int            `msgpack:"lens"` // postID -> word count
	AvgDocLen  float64                `msgpack:"avg"`
	TotalDocs  int                    `msgpack:"total"`
	StemMap    map[string][]string    `msgpack:"stem,omitempty"`  // stemmed -> original forms
	NgramIndex map[string][]string    `msgpack:"ngram,omitempty"` // trigram -> terms (for fuzzy search)
}
