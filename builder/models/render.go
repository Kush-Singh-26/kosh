package models

//go:generate msgp
//msgp:ignore PageContext Navbar Breadcrumb NavPage PageData TermData TaxonomyData ContentMetadata

import (
	"html/template"
	"time"
)

// PageContext represents the rendering context for a page (home or section).
type PageContext string

const (
	// ContextHome indicates the home page context.
	ContextHome PageContext = "home"
	// ContextSection indicates a section page context.
	ContextSection PageContext = "section"
)

// Navbar represents the navigation bar configuration.
type Navbar struct {
	Title        string
	TitleURL     string
	BtnLabel     string
	BtnURL       string
	IdentityHTML template.HTML // Pre-rendered fragment
}

// TOCEntry represents a table of contents entry
type TOCEntry struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
}

// Breadcrumb represents a single breadcrumb item for navigation.
type Breadcrumb struct {
	Title     string
	Link      string
	IsCurrent bool
}

// NavPage represents a previous or next navigation page.
type NavPage struct {
	Title string
	Link  string
}

// ContentMetadata represents the frontmatter and derived data of a markdown item
// for template rendering. It is the primary data structure passed to HTML templates
// for displaying item lists, navigation, and page content.
type ContentMetadata struct {
	Section     string
	Title       string
	Link        string
	Description string
	Taxonomies  map[string][]string // Generalized taxonomy terms
	Weight      int
	ReadingTime int
	IsPinned    bool
	IsDraft     bool
	DateObj     time.Time
	ContentHTML string
}

// TermData contains display data for a single term in a taxonomy (e.g., a specific tag).
type TermData struct {
	Name  string
	Link  string
	Count int
}

// TaxonomyData represents an entire taxonomy (e.g., 'tags' or 'categories').
type TaxonomyData struct {
	Name   string
	Plural string
	Terms  []TermData
}

// Paginator describes pagination state for templates.
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
	Title       string
	TabTitle    string
	Description string
	BaseURL     string
	Content     template.HTML
	// Meta contains template-accessible frontmatter values.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	Meta            map[string]any
	IsIndex         bool
	IsTaxonomyIndex bool
	IsGraphPage     bool
	Context         PageContext
	Navbar          Navbar
	ContentPrefix   string
	SectionIndexURL string
	Items           []ContentMetadata
	PinnedItems     []ContentMetadata
	Taxonomies      map[string]TaxonomyData // All aggregated taxonomies
	ItemTaxonomies  map[string][]string     // Specific taxonomy terms for the current page (e.g. current post's tags)
	BuildVersion    int64
	Permalink       string
	Image           string
	TOC             []TOCEntry
	Paginator       Paginator
	Assets          map[string]string
	Weight          int
	ReadingTime     int
	HasImages       bool
	Section         string

	// SiteData holds structured data from the data/ directory
	SiteData map[string]any

	// Navigation
	Breadcrumbs    []Breadcrumb
	NavigationTree *NodeTree
	PrevPage       *NavPage
	NextPage       *NavPage

	// Depth-aware pathing
	RelativePrefix string // e.g., "../" for depth 1

	// Config-driven fields
	Config TemplateConfig // To access Config fields in templates (Menu, Author, etc.)

	// SEO
	JSONLD template.HTML

	// Universal Fragment Cache
	Fragments    map[string]template.HTML
	IsCleanBuild bool

	// SSR Replacement Maps (for late-pass rendering in TOC/Fragments)
	SSRMath map[string]string       `json:"-"`
	SSRD2   map[string]SSRThemePair `json:"-"`
}
