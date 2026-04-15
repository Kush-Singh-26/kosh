package generators

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/zeebo/xxh3"
)

type postGraphInfo struct {
	Title       string              `json:"title"`
	Link        string              `json:"link"`
	Taxonomies  map[string][]string `json:"taxonomies"`
	Description string              `json:"description"`
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
			Taxonomies:  p.Taxonomies,
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
	Config        models.GraphConfig
	SiteTitle     string
	ContentPrefix string
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

	// Collect all unique taxonomy terms first
	termNodes := make(map[string]models.GraphNode)
	for _, p := range posts {
		if cfg.ShowsTags {
			for taxKey, terms := range p.Taxonomies {
				// For now, we prefix the slug with taxonomy key to avoid collisions
				for _, t := range terms {
					slug := taxKey + "-" + timeutil.Slugify(t)
					termID := "term-" + slug
					if !nodeExists[termID] {
						label := taxKey + ":" + t
						
						prefix := strings.Trim(opts.ContentPrefix, "/")
						if prefix != "" {
							prefix = "/" + prefix
						}

						termNodes[termID] = models.GraphNode{
							ID: termID, Label: label, Group: graphTagGroup, Value: graphTagValue,
							URL: fmt.Sprintf("%s%s/%s/%s.html", baseURL, prefix, taxKey, timeutil.Slugify(t)),
						}
						nodeExists[termID] = true
					}
				}
			}
		}
	}

	// Add root -> term links
	for _, term := range termNodes {
		nodes = append(nodes, term)
		links = append(links, models.GraphLink{Source: rootID, Target: term.ID, Weight: graphRootTagWeight})
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
			for taxKey, terms := range p.Taxonomies {
				for _, t := range terms {
					termID := "term-" + taxKey + "-" + timeutil.Slugify(t)
					links = append(links, models.GraphLink{Source: p.Link, Target: termID, Type: taxKey})
				}
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
