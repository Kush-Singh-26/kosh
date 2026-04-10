package generators

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"encoding/hex"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/zeebo/xxh3"
)

type postGraphInfo struct {
	Title       string   `json:"title"`
	Link        string   `json:"link"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
}

const (
	graphRootGroup     = 0
	graphRootValue     = 22
	graphTagGroup      = 2
	graphTagValue      = 10
	graphPostGroup     = 1
	graphPostValue     = 7
	graphRootTagWeight = 1
	graphDateFormat    = "Jan 02, 2006"
)

// ComputeGraphHash computes a stable hash for the knowledge graph data
func ComputeGraphHash(posts []models.PostMetadata) (string, error) {
	graphInfo := make([]postGraphInfo, 0, len(posts))
	for _, p := range posts {
		graphInfo = append(graphInfo, postGraphInfo{
			Title:       p.Title,
			Link:        p.Link,
			Tags:        p.Tags,
			Description: p.Description,
		})
	}

	data, err := json.Marshal(graphInfo)
	if err != nil {
		return "", err
	}

	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:]), nil
}

// GraphOptions configures knowledge graph generation.
type GraphOptions struct {
	Sink       fspkg.ArtifactSink
	BaseURL    string
	Posts      []models.PostMetadata
	OutputPath string
	Config     models.GraphConfig
	SiteTitle  string
}

// GenerateGraph builds the knowledge graph JSON and writes it to disk.
func GenerateGraph(opts GraphOptions) (string, string, error) {
	sink := opts.Sink
	baseURL := opts.BaseURL
	posts := opts.Posts
	outputPath := opts.OutputPath
	cfg := opts.Config
	siteTitle := opts.SiteTitle

	slog.Info("Generating knowledge graph data", "output", outputPath)

	nodes := []models.GraphNode{}
	links := []models.GraphLink{}
	nodeExists := make(map[string]bool)

	// Add root node for hub-and-spoke layout
	rootID := "root"
	nodes = append(nodes, models.GraphNode{
		ID: rootID, Label: siteTitle, Group: graphRootGroup, Value: graphRootValue, URL: baseURL,
	})
	nodeExists[rootID] = true

	// Collect all unique tags first
	tagNodes := make(map[string]models.GraphNode)
	for _, p := range posts {
		if cfg.ShowsTags {
			for _, t := range p.Tags {
				slug := timeutil.Slugify(t)
				tagID := "tag-" + slug
				if !nodeExists[tagID] {
					tagNodes[tagID] = models.GraphNode{
						ID: tagID, Label: "#" + t, Group: graphTagGroup, Value: graphTagValue,
						URL: fmt.Sprintf("%s/tags/%s.html", baseURL, slug),
					}
					nodeExists[tagID] = true
				}
			}
		}
	}

	// Add root -> tag links and add tags to nodes
	for _, tag := range tagNodes {
		nodes = append(nodes, tag)
		links = append(links, models.GraphLink{Source: rootID, Target: tag.ID, Weight: graphRootTagWeight})
	}

	// Add post nodes and tag -> post links
	for _, p := range posts {
		postURL := p.Link
		if baseURL != "" && postURL != "" && postURL[0] == '/' {
			postURL = baseURL + postURL
		}
		if !nodeExists[p.Link] {
			nodes = append(nodes, models.GraphNode{
				ID:      p.Link,
				Label:   p.Title,
				Group:   graphPostGroup,
				Value:   graphPostValue,
				URL:     postURL,
				Excerpt: p.Description,
				Date:    p.DateObj.Format(graphDateFormat),
			})
			nodeExists[p.Link] = true
		}
		if cfg.ShowsTags {
			for _, t := range p.Tags {
				tagID := "tag-" + timeutil.Slugify(t)
				links = append(links, models.GraphLink{Source: p.Link, Target: tagID, Type: "tag"})
			}
		}
	}

	output, err := json.Marshal(models.GraphData{Nodes: nodes, Links: links})
	if err != nil {
		return "", "", err
	}
	if err := sink.WriteFile(outputPath, output); err != nil {
		return "", "", err
	}

	hash, err := ComputeGraphHash(posts)
	if err != nil {
		return "", "", err
	}
	return outputPath, hash, nil
}
