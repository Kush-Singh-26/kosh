package models

// GraphNode represents a node in the knowledge graph.
type GraphNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Group       int    `json:"group"` // 0=Root/Hub, 1=Posts, 2=Tags, 3=Categories
	Value       int    `json:"val"`   // Node size
	URL         string `json:"url,omitempty"`
	Date        string `json:"date,omitempty"`
	ReadingTime int    `json:"readingTime,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"`
}

// GraphLink represents a link between nodes in the knowledge graph.
type GraphLink struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Type   string  `json:"type,omitempty"`   // "tag", "wiki", "similarity", "backlink"
	Weight float64 `json:"weight,omitempty"` // for similarity edges
}

// GraphData bundles graph nodes and links.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// GraphConfig defines knowledge graph generation options.
type GraphConfig struct {
	IsEnabled       bool `yaml:"isEnabled"`
	ShowsTags       bool `yaml:"showsTags"`
	MaxNodes        int  `yaml:"maxNodes"`
	MinTagFrequency int  `yaml:"minTagFrequency"`
}

// UnmarshalYAML implements custom unmarshalling for GraphConfig.
func (gc *GraphConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		gc.IsEnabled = b
		gc.ShowsTags = true
		return nil
	}
	type graphConfigAlias GraphConfig
	alias := (*graphConfigAlias)(gc)
	if err := unmarshal(alias); err != nil {
		return err
	}
	if !gc.ShowsTags {
		gc.ShowsTags = true
	}
	return nil
}
