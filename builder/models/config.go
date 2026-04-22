package models

//go:generate msgp

import (
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
)

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

// SearchOptionsConfig configures search behavior and federated endpoints.
type SearchOptionsConfig struct {
	IsEnabled bool                   `yaml:"isEnabled"`
	Ranking   searchpkg.SearchRankingConfig `yaml:"ranking"`
	Endpoints []string               `yaml:"endpoints"`
}

// GeneratorsConfig enables/disables site-wide generators.
type GeneratorsConfig struct {
	IsSitemapEnabled bool                `yaml:"isSitemapEnabled"`
	IsRSSEnabled     bool                `yaml:"isRSSEnabled"`
	Graph            GraphConfig         `yaml:"graph"`
	IsPWAEnabled     bool                `yaml:"isPWAEnabled"`
	Search           SearchOptionsConfig `yaml:"search"`
}

// FeaturesConfig enables/disables site features.
type FeaturesConfig struct {
	UseRawMarkdown bool             `yaml:"useRawMarkdown"`
	Generators     GeneratorsConfig `yaml:"generators"`
}

// SocialCardsConfig defines visual parameters for social card generation.
type SocialCardsConfig struct {
	Background string   `yaml:"background"`
	Gradient   []string `yaml:"gradient"`
	Angle      int      `yaml:"angle"`
	TextColor  string   `yaml:"textColor"`
}

// NavbarContextConfig defines context-aware branding for the navbar.
type NavbarContextConfig struct {
	Title    string `yaml:"title"`
	BtnLabel string `yaml:"btnLabel"`
}

// NavbarIdentityConfig defines the navbar identity configuration for different contexts.
type NavbarIdentityConfig struct {
	Home    NavbarContextConfig `yaml:"home"`
	Section NavbarContextConfig `yaml:"section"`
}

// TemplateConfig defines the strictly-typed subset of project configuration
// accessible within HTML templates. This prevents tight coupling between
// models and the main config package while restoring type safety.
type TemplateConfig interface {
	GetMenu() []MenuEntry
	GetFooterMenu() []MenuEntry
	GetAuthor() AuthorConfig
	GetSocial() SocialCardsConfig
	GetFeatures() FeaturesConfig
	GetSiteTitle() string
	GetLogo() string
	GetBaseURL() string
	GetContentPrefix() string
	GetTemplateDir() string
	GetStaticDir() string
	GetLayoutsDir() string
	GetContentDir() string
	IsDevMode() bool
	GetSiteData() map[string]any
	GetNavbar() NavbarIdentityConfig
	GetHomeBadge() string
}
