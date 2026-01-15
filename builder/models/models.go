// defines the data structures used by templates and generators
package models

import (
	"encoding/xml"
	"html/template"
	"time"
)

// --- TOC Structure ---
type TOCEntry struct {
	ID    string
	Text  string
	Level int
}

// PostMetadata represents the frontmatter and derived data of a markdown post.
type PostMetadata struct {
	Title       string
	TabTile     string
	Link        string
	Description string
	Tags        []string
	ReadingTime int
	Pinned      bool
	Draft       bool
	DateObj     time.Time
	HasMath     bool
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
	HasMath      bool
	LayoutCSS    template.CSS
	ThemeCSS     template.CSS
	Permalink    string
	Image        string
	TOC          []TOCEntry
	Paginator    Paginator
	Assets       map[string]string

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
	ID          int
	Title       string
	Link        string
	Description string
	Tags        []string
	Content     string // Raw plain text for snippet extraction
}

type SearchIndex struct {
	Posts     []PostRecord
	Inverted  map[string]map[int]int // word -> postID -> frequency
	DocLens   map[int]int            // postID -> word count
	AvgDocLen float64
	TotalDocs int
}
