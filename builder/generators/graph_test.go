package generators

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/testutil"

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

	resultPath, _, err := GenerateGraph(GraphOptions{
		Sink:       sink,
		BaseURL:    baseURL,
		Posts:      posts,
		OutputPath: outputPath,
		Config:     models.GraphConfig{IsEnabled: true, ShowsTags: true},
		SiteTitle:  "Test Site",
	})
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
	// 1 root + 2 posts + 3 unique tags (go, testing, web) = 6 nodes
	if len(data.Nodes) != 6 {
		t.Errorf("Expected 6 nodes, got %d", len(data.Nodes))
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
	// Root -> tags (3 links: root to go, testing, web)
	// Post 1 -> tags (2 links: post1 to go, testing)
	// Post 2 -> tags (2 links: post2 to go, web)
	// Total 7 links
	if len(data.Links) != 7 {
		t.Errorf("Expected 7 links, got %d", len(data.Links))
	}

	foundLink := false
	for _, link := range data.Links {
		if link.Source == "https://example.com/post1.html" && link.Target == "tag-testing" {
			foundLink = true
			if link.Type != "tag" {
				t.Errorf("Expected link type 'tag', got %s", link.Type)
			}
			break
		}
	}
	if !foundLink {
		t.Error("Link from Post 1 to 'testing' tag missing")
	}
}

func TestGenerateGraph_Empty(t *testing.T) {
	sink := testutil.NewMemSink()
	_, _, err := GenerateGraph(GraphOptions{
		Sink:       sink,
		BaseURL:    "https://example.com",
		Posts:      []models.PostMetadata{},
		OutputPath: "empty.json",
		Config:     models.GraphConfig{IsEnabled: true, ShowsTags: true},
		SiteTitle:  "Test Site",
	})
	if err != nil {
		t.Fatalf("GenerateGraph failed with empty posts: %v", err)
	}

	content := sink.Files["empty.json"]
	var data models.GraphData
	_ = json.Unmarshal(content, &data)

	if len(data.Nodes) != 1 || len(data.Links) != 0 {
		t.Error("Expected 1 root node and 0 links for empty input")
	}
}

func TestGenerateGraph_DisableTags(t *testing.T) {
	sink := testutil.NewMemSink()
	posts := []models.PostMetadata{
		{
			Title:   "Post 1",
			Link:    "https://example.com/post1.html",
			Tags:    []string{"Go"},
			DateObj: time.Now(),
		},
	}

	_, _, err := GenerateGraph(GraphOptions{
		Sink:       sink,
		BaseURL:    "https://example.com",
		Posts:      posts,
		OutputPath: "graph.json",
		Config:     models.GraphConfig{IsEnabled: true, ShowsTags: false},
		SiteTitle:  "Test Site",
	})
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content := sink.Files["graph.json"]
	var data models.GraphData
	_ = json.Unmarshal(content, &data)

	// 1 root + 1 post = 2 nodes (no tags when disabled)
	if len(data.Nodes) != 2 {
		t.Errorf("Expected 2 nodes (root + post), got %d", len(data.Nodes))
	}

	if len(data.Links) != 0 {
		t.Errorf("Expected 0 links (tags disabled), got %d", len(data.Links))
	}
}

func TestComputeGraphHash_DifferentPosts(t *testing.T) {
	postsA := []models.PostMetadata{
		{Title: "Post 1", Link: "/post1.html", Tags: []string{"Go"}},
		{Title: "Post 2", Link: "/post2.html", Tags: []string{"Go"}},
	}

	postsB := []models.PostMetadata{
		{Title: "Post 1", Link: "/post1.html", Tags: []string{"Go", "Testing"}},
		{Title: "Post 2", Link: "/post2.html", Tags: []string{"Go"}},
	}

	hashA, err := ComputeGraphHash(postsA)
	if err != nil {
		t.Fatalf("ComputeGraphHash failed: %v", err)
	}

	hashB, err := ComputeGraphHash(postsB)
	if err != nil {
		t.Fatalf("ComputeGraphHash failed: %v", err)
	}

	if hashA == hashB {
		t.Error("Hashes should differ when post tags change")
	}
}
