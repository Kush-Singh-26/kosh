package generators

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func GenerateGraph(sink utils.ArtifactSink, baseURL string, posts []models.PostMetadata, outputPath string) (string, error) {
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
