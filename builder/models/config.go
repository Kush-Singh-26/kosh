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

// UnmarshalYAML implements custom unmarshalling to support legacy aliases.
func (c *NavbarIdentityConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type alias NavbarIdentityConfig
	var aux struct {
		*alias `yaml:",inline"`
		Posts  *NavbarContextConfig `yaml:"posts,omitempty"`
		Blog   *NavbarContextConfig `yaml:"blog,omitempty"`
	}
	aux.alias = (*alias)(c)

	if err := unmarshal(&aux); err != nil {
		return err
	}

	// Fallback to legacy config keys if section is empty
	if c.Section.Title == "" && c.Section.BtnLabel == "" {
		if aux.Posts != nil {
			c.Section = *aux.Posts
		} else if aux.Blog != nil {
			c.Section = *aux.Blog
		}
	}
	return nil
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
