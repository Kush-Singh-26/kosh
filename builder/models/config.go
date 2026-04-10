package models

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
	IsSitemapEnabled bool        `yaml:"isSitemapEnabled"`
	IsRSSEnabled     bool        `yaml:"isRSSEnabled"`
	Graph            GraphConfig `yaml:"graph"`
	IsPWAEnabled     bool        `yaml:"isPWAEnabled"`
	IsSearchEnabled  bool        `yaml:"isSearchEnabled"`
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
