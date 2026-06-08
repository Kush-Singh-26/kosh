package generators

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zeebo/xxh3"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
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
	graphTaxonomyGroup = 2
	graphTaxonomyValue = 10
	graphItemGroup     = 1
	graphItemValue     = 7
	graphRootTagWeight = 1
	graphDateFormat    = "Jan 02, 2006"
)

// GraphOptions configures knowledge graph generation.
type GraphOptions struct {
	Sink          fspkg.ArtifactSink
	BaseURL       string
	Items         []models.ContentMetadata
	OutputPath    string
	Config        models.GraphConfig
	SiteTitle     string
	ContentPrefix string
}

// ComputeGraphHash computes a stable hash for the knowledge graph data
func ComputeGraphHash(items []models.ContentMetadata) (string, error) {
	graphInfo := make([]postGraphInfo, 0, len(items))
	for _, p := range items {
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

// GenerateGraph builds the knowledge graph JSON and writes it to disk.
func GenerateGraph(opts GraphOptions) (string, string, error) {
	slog.Info("Generating knowledge graph data", "output", opts.OutputPath)

	nodes, links, nodeExists := initializeGraphNodes(opts)
	termNodes := collectTaxonomyTerms(opts, nodeExists)
	addTermNodes(&nodes, &links, termNodes)
	addPostNodes(&nodes, &links, opts, nodeExists)

	output, err := json.Marshal(models.GraphData{Nodes: nodes, Links: links})
	if err != nil {
		return "", "", err
	}
	if err := opts.Sink.WriteFile(opts.OutputPath, output); err != nil {
		return "", "", err
	}

	hash, err := ComputeGraphHash(opts.Items)
	if err != nil {
		return "", "", err
	}
	return opts.OutputPath, hash, nil
}

// initializeGraphNodes creates the root node and initializes data structures
func initializeGraphNodes(opts GraphOptions) ([]models.GraphNode, []models.GraphLink, map[string]bool) {
	nodes := []models.GraphNode{}
	links := []models.GraphLink{}
	nodeExists := make(map[string]bool)

	rootID := "root"
	nodes = append(nodes, models.GraphNode{
		ID: rootID, Label: opts.SiteTitle, Group: graphRootGroup, Value: graphRootValue, URL: opts.BaseURL,
	})
	nodeExists[rootID] = true
	return nodes, links, nodeExists
}

// collectTaxonomyTerms collects all unique taxonomy terms from posts
func collectTaxonomyTerms(opts GraphOptions, nodeExists map[string]bool) map[string]models.GraphNode {
	termNodes := make(map[string]models.GraphNode)
	if !opts.Config.ShowsTaxonomies {
		return termNodes
	}

	for _, p := range opts.Items {
		for taxKey, terms := range p.Taxonomies {
			for _, t := range terms {
				slug := taxKey + "-" + timeutil.Slugify(t)
				termID := "term-" + slug
				if !nodeExists[termID] {
					termNodes[termID] = createTermNode(termID, taxKey, t, opts.BaseURL, opts.ContentPrefix)
					nodeExists[termID] = true
				}
			}
		}
	}
	return termNodes
}

// createTermNode creates a taxonomy term node
func createTermNode(termID, taxKey, term, baseURL, contentPrefix string) models.GraphNode {
	prefix := strings.Trim(contentPrefix, "/")
	if prefix != "" {
		prefix = "/" + prefix
	}

	return models.GraphNode{
		ID:    termID,
		Label: taxKey + ":" + term,
		Group: graphTaxonomyGroup,
		Value: graphTaxonomyValue,
		URL:   fmt.Sprintf("%s%s/%s/%s.html", baseURL, prefix, taxKey, timeutil.Slugify(term)),
	}
}

// addTermNodes adds term nodes and root->term links
func addTermNodes(nodes *[]models.GraphNode, links *[]models.GraphLink, termNodes map[string]models.GraphNode) {
	for _, term := range termNodes {
		*nodes = append(*nodes, term)
		*links = append(*links, models.GraphLink{Source: "root", Target: term.ID, Weight: graphRootTagWeight})
	}
}

// addPostNodes adds item nodes and taxonomy links
func addPostNodes(nodes *[]models.GraphNode, links *[]models.GraphLink, opts GraphOptions, nodeExists map[string]bool) {
	for _, p := range opts.Items {
		itemURL := p.Link
		if opts.BaseURL != "" && itemURL != "" && itemURL[0] == '/' {
			itemURL = opts.BaseURL + itemURL
		}
		if !nodeExists[p.Link] {
			*nodes = append(*nodes, models.GraphNode{
				ID:      p.Link,
				Label:   p.Title,
				Group:   graphItemGroup,
				Value:   graphItemValue,
				URL:     itemURL,
				Excerpt: p.Description,
				Date:    p.DateObj.Format(graphDateFormat),
			})
			nodeExists[p.Link] = true
		}
		if opts.Config.ShowsTaxonomies {
			for taxKey, terms := range p.Taxonomies {
				for _, t := range terms {
					termID := "term-" + taxKey + "-" + timeutil.Slugify(t)
					*links = append(*links, models.GraphLink{Source: p.Link, Target: termID, Type: taxKey})
				}
			}
		}
	}
}
