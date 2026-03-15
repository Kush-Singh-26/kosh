package generators

import (
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestGenerateGraph(t *testing.T) {
	sink := testutil.NewMemSink()
	baseURL := "https://example.com"
	outputPath := "graph.json"

	posts := []models.PostMetadata{
		{
			Title:   "Post 1",
			Link:    "https://example.com/post1.html",
			Tags:    []string{"Go", "Testing"},
			DateObj: time.Now(),
		},
		{
			Title:   "Post 2",
			Link:    "https://example.com/post2.html",
			Tags:    []string{"Go", "Web"},
			DateObj: time.Now(),
		},
	}

	resultPath, err := GenerateGraph(sink, baseURL, posts, outputPath)
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	if resultPath != outputPath {
		t.Errorf("Expected result path %s, got %s", outputPath, resultPath)
	}

	content, ok := sink.Files[outputPath]
	if !ok {
		t.Fatalf("Graph file not found in sink at %s", outputPath)
	}

	var data models.GraphData
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("Failed to unmarshal graph data: %v", err)
	}

	// Verify Nodes
	// 2 posts + 3 unique tags (go, testing, web) = 5 nodes
	if len(data.Nodes) != 5 {
		t.Errorf("Expected 5 nodes, got %d", len(data.Nodes))
	}

	nodeMap := make(map[string]models.GraphNode)
	for _, node := range data.Nodes {
		nodeMap[node.ID] = node
	}

	// Check post node
	post1, ok := nodeMap["https://example.com/post1.html"]
	if !ok {
		t.Error("Post 1 node missing")
	} else {
		if post1.Label != "Post 1" {
			t.Errorf("Expected label Post 1, got %s", post1.Label)
		}
		if post1.Group != 1 {
			t.Errorf("Expected group 1 for post, got %d", post1.Group)
		}
	}

	// Check tag node
	tagGo, ok := nodeMap["tag-go"]
	if !ok {
		t.Error("Tag 'go' node missing")
	} else {
		if tagGo.Label != "#Go" {
			t.Errorf("Expected label #Go, got %s", tagGo.Label)
		}
		if tagGo.Group != 2 {
			t.Errorf("Expected group 2 for tag, got %d", tagGo.Group)
		}
		if !strings.Contains(tagGo.URL, "/tags/go.html") {
			t.Errorf("Expected tag URL to contain /tags/go.html, got %s", tagGo.URL)
		}
	}

	// Verify Links
	// Post 1: Go, Testing (2 links)
	// Post 2: Go, Web (2 links)
	// Total 4 links
	if len(data.Links) != 4 {
		t.Errorf("Expected 4 links, got %d", len(data.Links))
	}

	foundLink := false
	for _, link := range data.Links {
		if link.Source == "https://example.com/post1.html" && link.Target == "tag-testing" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Error("Link from Post 1 to 'testing' tag missing")
	}
}

func TestGenerateGraph_Empty(t *testing.T) {
	sink := testutil.NewMemSink()
	_, err := GenerateGraph(sink, "https://example.com", []models.PostMetadata{}, "empty.json")
	if err != nil {
		t.Fatalf("GenerateGraph failed with empty posts: %v", err)
	}

	content := sink.Files["empty.json"]
	var data models.GraphData
	_ = json.Unmarshal(content, &data)

	if len(data.Nodes) != 0 || len(data.Links) != 0 {
		t.Error("Expected empty nodes and links for empty input")
	}
}
