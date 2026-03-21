// Package models provides shared data structures for SSR (Server-Side Rendering) artifacts.
// This package defines types for D2 diagrams and LaTeX math expressions that are cached
// at the infrastructure layer (builder/cache) without creating upward dependencies.
package models

//go:generate msgp

// SSRArtifactType represents the type of SSR content
type SSRArtifactType int

const (
	// SSRTypeD2 represents D2 diagram SVG output
	SSRTypeD2 SSRArtifactType = iota
	// SSRTypeMath represents LaTeX math HTML output
	SSRTypeMath
)

// String returns the string representation of SSR artifact type
func (t SSRArtifactType) String() string {
	switch t {
	case SSRTypeD2:
		return "d2"
	case SSRTypeMath:
		return "math"
	default:
		return "unknown"
	}
}

// ParseSSRType parses a string into SSR artifact type
func ParseSSRType(s string) SSRArtifactType {
	switch s {
	case "d2":
		return SSRTypeD2
	case "math":
		return SSRTypeMath
	default:
		return SSRTypeD2
	}
}

// MathExpression represents a LaTeX expression with its metadata.
// This structure is used by both the cache layer and renderer to store
// and retrieve server-side rendered math expressions.
type MathExpression struct {
	// LaTeX is the raw LaTeX source code
	LaTeX string `json:"latex" msg:"latex"`
	// DisplayMode indicates whether to render in display mode (block) or inline mode
	DisplayMode bool `json:"displayMode" msg:"displayMode"`
	// Hash is the content hash used for caching and deduplication
	Hash string `json:"-" msg:"hash"`
}

// SSRThemePair stores both light and dark versions together for atomic access.
// This structure is primarily used for D2 diagrams which have separate themes.
type SSRThemePair struct {
	Light string `msg:"light"`
	Dark  string `msg:"dark"`
}
