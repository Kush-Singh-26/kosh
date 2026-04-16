package models

import (
	"time"
)

// NodeType defines the role of a node in the site hierarchy.
type NodeType int

const (
	// NodePage represents a standalone markdown or HTML page.
	NodePage NodeType = iota
	// NodeSection represents a directory containing other nodes.
	NodeSection
	// NodeTaxonomy represents a list page for a taxonomy term (e.g., /tags/go/).
	NodeTaxonomy
	// NodeHome represents the root entry point of the site.
	NodeHome
)

// Resource represents any content item processed by the SSG.
// This is a generalized version of ContentMetadata that can represent
// blog posts, documentation pages, or portfolio items.
type Resource struct {
	Title          string              `json:"title"`
	Link           string              `json:"link"`
	Description    string              `json:"description"`
	Taxonomies     map[string][]string `json:"taxonomies"`
	Weight         int                 `json:"weight"`
	ReadingTime    int                 `json:"reading_time"`
	IsPinned       bool                `json:"is_pinned"`
	IsDraft        bool                `json:"is_draft"`
	Date           time.Time           `json:"date"`
	ContentHTML    string              `json:"content_html,omitempty"`
	Type           NodeType            `json:"type"`
	Layout         string              `json:"layout"`
	Metadata       map[string]any      `json:"metadata"`
	RelPath        string              `json:"rel_path"`
	RelativePrefix string              `json:"relative_prefix"`
}

// NodeTree represents the hierarchical structure of the site.
type NodeTree struct {
	Resource Resource
	Children []*NodeTree
	Parent   *NodeTree
}
