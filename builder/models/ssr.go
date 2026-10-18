// Package models provides shared data structures for SSR (Server-Side Rendering) artifacts.
// This package defines types for D2 diagrams and LaTeX math expressions that are cached
// at the infrastructure layer (builder/cache) without creating upward dependencies.
package models

import (
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2target"
)

//go:generate msgp

// SSRArtifactType represents the type of SSR content
type SSRArtifactType int

const (
	// SSRTypeD2 represents D2 diagram SVG output
	SSRTypeD2 SSRArtifactType = iota
	// SSRTypeMath represents LaTeX math HTML output
	SSRTypeMath
	// SSRTypeMermaid represents Mermaid diagram SVG output
	SSRTypeMermaid
)

// String returns the string representation of SSR artifact type
func (t SSRArtifactType) String() string {
	switch t {
	case SSRTypeD2:
		return "d2"
	case SSRTypeMath:
		return "math"
	case SSRTypeMermaid:
		return "mermaid"
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
	case "mermaid":
		return SSRTypeMermaid
	default:
		return SSRTypeD2
	}
}

// D2Expression represents a D2 diagram with its metadata.
// This structure is used by both the cache layer and renderer to store
// and retrieve server-side rendered D2 diagrams.
type D2Expression struct {
	// Code is the raw D2 source code
	Code string `json:"code" msg:"code"`
	// Hash is the content hash used for caching and deduplication
	Hash string `json:"-" msg:"hash"`
}

// MermaidExpression represents a Mermaid diagram with its metadata.
// This structure is used by both the cache layer and renderer to store
// and retrieve server-side rendered Mermaid diagrams.
type MermaidExpression struct {
	// Code is the raw Mermaid source code
	Code string `json:"code" msg:"code"`
	// Hash is the content hash used for caching and deduplication
	Hash string `json:"-" msg:"hash"`
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
	// Line is the line number in the source file
	Line int `json:"-" msg:"line"`
	// Error is any rendering error encountered
	Error string `json:"-" msg:"error"`
}

// SSRThemePair stores both light and dark versions together for atomic access.
// This structure is primarily used for D2 diagrams which have separate themes.
type SSRThemePair struct {
	Light string `msg:"light"`
	Dark  string `msg:"dark"`
}

// D2Config controls D2 diagram rendering behavior.
// Fields map to d2svg.RenderOpts options.
type D2Config struct {
	LightThemeID int64   `yaml:"lightThemeID" json:"lightThemeID"`
	DarkThemeID  int64   `yaml:"darkThemeID" json:"darkThemeID"`
	Scale        float64 `yaml:"scale" json:"scale"`
	Pad          int64   `yaml:"pad" json:"pad"`
	Sketch       bool    `yaml:"sketch" json:"sketch"`
	// StrokeWidth scales the rendered diagram line/shape thickness (1.0 = default).
	// Values below 1.0 make diagrams thinner; 0 (unset) is treated as 1.0.
	StrokeWidth float64 `yaml:"strokeWidth" json:"strokeWidth"`
	// LightOverrides retints the light D2 theme to match the site palette.
	LightOverrides *D2ThemeOverrides `yaml:"lightOverrides" json:"lightOverrides"`
	// DarkOverrides retints the dark D2 theme to match the site palette.
	DarkOverrides *D2ThemeOverrides `yaml:"darkOverrides" json:"darkOverrides"`
}

// D2ThemeOverrides mirrors d2target.ThemeOverrides. It lets a site retint D2
// diagrams (neutrals N1-N7 darkest->lightest, base colors B1-B6, and accent
// colors AA2/AA4/AA5/AB4/AB5) without coupling the models/cache layer to d2.
// All fields are optional hex strings; nil leaves the base theme color intact.
type D2ThemeOverrides struct {
	N1  *string `yaml:"n1,omitempty" json:"n1,omitempty"`
	N2  *string `yaml:"n2,omitempty" json:"n2,omitempty"`
	N3  *string `yaml:"n3,omitempty" json:"n3,omitempty"`
	N4  *string `yaml:"n4,omitempty" json:"n4,omitempty"`
	N5  *string `yaml:"n5,omitempty" json:"n5,omitempty"`
	N6  *string `yaml:"n6,omitempty" json:"n6,omitempty"`
	N7  *string `yaml:"n7,omitempty" json:"n7,omitempty"`
	B1  *string `yaml:"b1,omitempty" json:"b1,omitempty"`
	B2  *string `yaml:"b2,omitempty" json:"b2,omitempty"`
	B3  *string `yaml:"b3,omitempty" json:"b3,omitempty"`
	B4  *string `yaml:"b4,omitempty" json:"b4,omitempty"`
	B5  *string `yaml:"b5,omitempty" json:"b5,omitempty"`
	B6  *string `yaml:"b6,omitempty" json:"b6,omitempty"`
	AA2 *string `yaml:"aa2,omitempty" json:"aa2,omitempty"`
	AA4 *string `yaml:"aa4,omitempty" json:"aa4,omitempty"`
	AA5 *string `yaml:"aa5,omitempty" json:"aa5,omitempty"`
	AB4 *string `yaml:"ab4,omitempty" json:"ab4,omitempty"`
	AB5 *string `yaml:"ab5,omitempty" json:"ab5,omitempty"`
}

// ToD2Target converts the model overrides into the d2 library's ThemeOverrides
// type. Returns a nil pointer when no overrides are set.
func (o *D2ThemeOverrides) ToD2Target() *d2target.ThemeOverrides {
	if o == nil {
		return nil
	}
	return &d2target.ThemeOverrides{
		N1: o.N1, N2: o.N2, N3: o.N3, N4: o.N4, N5: o.N5, N6: o.N6, N7: o.N7,
		B1: o.B1, B2: o.B2, B3: o.B3, B4: o.B4, B5: o.B5, B6: o.B6,
		AA2: o.AA2, AA4: o.AA4, AA5: o.AA5, AB4: o.AB4, AB5: o.AB5,
	}
}

// signature returns a stable representation of the color overrides so they can
// be folded into the per-diagram cache key.
func (o *D2ThemeOverrides) signature() string {
	if o == nil {
		return ""
	}
	vals := []*string{o.N1, o.N2, o.N3, o.N4, o.N5, o.N6, o.N7,
		o.B1, o.B2, o.B3, o.B4, o.B5, o.B6, o.AA2, o.AA4, o.AA5, o.AB4, o.AB5}
	var b strings.Builder
	for i, v := range vals {
		if i > 0 {
			b.WriteByte(',')
		}
		if v != nil {
			b.WriteString(*v)
		}
	}
	return b.String()
}

// ConfigSignature returns a stable string capturing every D2 rendering setting
// that affects the output SVG: theme IDs, scale, padding, sketch mode, stroke
// width, and color overrides. It is folded into the per-diagram cache key
// (see parser unified transformer) so that changing any of these settings
// invalidates previously cached SVGs instead of serving stale output.
func (c *D2Config) ConfigSignature() string {
	var b strings.Builder
	fmt.Fprintf(&b, "L%d|D%d|S%g|P%d|K%t|W%g",
		c.LightThemeID, c.DarkThemeID, c.Scale, c.Pad, c.Sketch, c.StrokeWidth)
	if c.LightOverrides != nil {
		b.WriteString("|LO:")
		b.WriteString(c.LightOverrides.signature())
	}
	if c.DarkOverrides != nil {
		b.WriteString("|DO:")
		b.WriteString(c.DarkOverrides.signature())
	}
	return b.String()
}

// MermaidConfig controls Mermaid diagram rendering behavior.
type MermaidConfig struct {
	// Enabled toggles Mermaid diagram SSR (default: true).
	Enabled *bool `yaml:"enabled" json:"enabled"`
}

// IsEnabled returns whether Mermaid rendering is enabled (defaults to true).
func (c *MermaidConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// Signature returns a stable string capturing the Mermaid config for cache key generation.
func (c *MermaidConfig) Signature() string {
	if c.IsEnabled() {
		return "enabled"
	}
	return "disabled"
}
