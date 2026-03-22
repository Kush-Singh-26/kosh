package generators

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"encoding/hex"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/zeebo/xxh3"
)

type postGraphInfo struct {
	Title string   `json:"title"`
	Link  string   `json:"link"`
	Tags  []string `json:"tags"`
}

// ComputeGraphHash computes a stable hash for the knowledge graph data
func ComputeGraphHash(posts []models.PostMetadata) (string, error) {
	graphInfo := make([]postGraphInfo, 0, len(posts))
	for _, p := range posts {
		graphInfo = append(graphInfo, postGraphInfo{
			Title: p.Title,
			Link:  p.Link,
			Tags:  p.Tags,
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

func GenerateGraph(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, outputPath string) (string, error) {
	slog.Info("Generating knowledge graph data")

	nodes := []models.GraphNode{}
	links := []models.GraphLink{}
	nodeExists := make(map[string]bool)

	for _, p := range posts {
		if !nodeExists[p.Link] {
			nodes = append(nodes, models.GraphNode{
				ID: p.Link, Label: p.Title, Group: 1, Value: 10, URL: p.Link,
			})
			nodeExists[p.Link] = true
		}
		for _, t := range p.Tags {
			tagID := "tag-" + strings.ToLower(strings.TrimSpace(t))
			if !nodeExists[tagID] {
				nodes = append(nodes, models.GraphNode{
					ID: tagID, Label: "#" + strings.TrimSpace(t), Group: 2, Value: 5,
					URL: fmt.Sprintf("%s/tags/%s.html", baseURL, strings.ToLower(strings.TrimSpace(t))),
				})
				nodeExists[tagID] = true
			}
			links = append(links, models.GraphLink{Source: p.Link, Target: tagID})
		}
	}
	output, err := json.Marshal(models.GraphData{Nodes: nodes, Links: links})
	if err != nil {
		return "", err
	}
	if err := sink.WriteFile(outputPath, output); err != nil {
		return "", err
	}
	return outputPath, nil
}
