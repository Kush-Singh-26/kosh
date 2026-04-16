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

	items := []models.ContentMetadata{
		{
			Title:      "Item 1",
			Link:       "https://example.com/item1.html",
			Taxonomies: map[string][]string{"tags": {"Go", "Testing"}},
			DateObj:    time.Now(),
		},
		{
			Title:      "Item 2",
			Link:       "https://example.com/item2.html",
			Taxonomies: map[string][]string{"tags": {"Go", "Web"}},
			DateObj:    time.Now(),
		},
	}

	resultPath, _, err := GenerateGraph(GraphOptions{
		Sink:       sink,
		BaseURL:    baseURL,
		Items:      items,
		OutputPath: outputPath,
		Config:     models.GraphConfig{IsEnabled: true, ShowsTaxonomies: true},
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
	// 1 root + 2 items + 3 unique tags (go, testing, web) = 6 nodes
	if len(data.Nodes) != 6 {
		t.Errorf("Expected 6 nodes, got %d", len(data.Nodes))
	}

	nodeMap := make(map[string]models.GraphNode)
	for _, node := range data.Nodes {
		nodeMap[node.ID] = node
	}

	// Check item node
	item1, ok := nodeMap["https://example.com/item1.html"]
	if !ok {
		t.Error("Item 1 node missing")
	} else {
		if item1.Label != "Item 1" {
			t.Errorf("Expected label Item 1, got %s", item1.Label)
		}
		if item1.Group != 1 {
			t.Errorf("Expected group 1 for item, got %d", item1.Group)
		}
	}

	// Check tag node
	tagGo, ok := nodeMap["term-tags-go"]
	if !ok {
		t.Error("Tag 'go' node missing")
	} else {
		if tagGo.Label != "tags:Go" {
			t.Errorf("Expected label tags:Go, got %s", tagGo.Label)
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
	// Item 1 -> tags (2 links: item1 to go, testing)
	// Item 2 -> tags (2 links: item2 to go, web)
	// Total 7 links
	if len(data.Links) != 7 {
		t.Errorf("Expected 7 links, got %d", len(data.Links))
	}

	foundLink := false
	for _, link := range data.Links {
		if link.Source == "https://example.com/item1.html" && link.Target == "term-tags-testing" {
			foundLink = true
			if link.Type != "tags" {
				t.Errorf("Expected link type 'tags', got %s", link.Type)
			}
			break
		}
	}
	if !foundLink {
		t.Error("Link from Item 1 to 'testing' tag missing")
	}
}

func TestGenerateGraph_Empty(t *testing.T) {
	sink := testutil.NewMemSink()
	_, _, err := GenerateGraph(GraphOptions{
		Sink:       sink,
		BaseURL:    "https://example.com",
		Items:      []models.ContentMetadata{},
		OutputPath: "empty.json",
		Config:     models.GraphConfig{IsEnabled: true, ShowsTaxonomies: true},
		SiteTitle:  "Test Site",
	})
	if err != nil {
		t.Fatalf("GenerateGraph failed with empty items: %v", err)
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
	items := []models.ContentMetadata{
		{
			Title:      "Item 1",
			Link:       "https://example.com/item1.html",
			Taxonomies: map[string][]string{"tags": {"Go"}},
			DateObj:    time.Now(),
		},
	}

	_, _, err := GenerateGraph(GraphOptions{
		Sink:       sink,
		BaseURL:    "https://example.com",
		Items:      items,
		OutputPath: "graph.json",
		Config:     models.GraphConfig{IsEnabled: true, ShowsTaxonomies: false},
		SiteTitle:  "Test Site",
	})
	if err != nil {
		t.Fatalf("GenerateGraph failed: %v", err)
	}

	content := sink.Files["graph.json"]
	var data models.GraphData
	_ = json.Unmarshal(content, &data)

	// 1 root + 1 item = 2 nodes (no tags when disabled)
	if len(data.Nodes) != 2 {
		t.Errorf("Expected 2 nodes (root + item), got %d", len(data.Nodes))
	}

	if len(data.Links) != 0 {
		t.Errorf("Expected 0 links (tags disabled), got %d", len(data.Links))
	}
}

func TestComputeGraphHash_DifferentItems(t *testing.T) {
	itemsA := []models.ContentMetadata{
		{Title: "Item 1", Link: "/item1.html", Taxonomies: map[string][]string{"tags": {"Go"}}},
		{Title: "Item 2", Link: "/item2.html", Taxonomies: map[string][]string{"tags": {"Go"}}},
	}

	itemsB := []models.ContentMetadata{
		{Title: "Item 1", Link: "/item1.html", Taxonomies: map[string][]string{"tags": {"Go", "Testing"}}},
		{Title: "Item 2", Link: "/item2.html", Taxonomies: map[string][]string{"tags": {"Go"}}},
	}

	hashA, err := ComputeGraphHash(itemsA)
	if err != nil {
		t.Fatalf("ComputeGraphHash failed: %v", err)
	}

	hashB, err := ComputeGraphHash(itemsB)
	if err != nil {
		t.Fatalf("ComputeGraphHash failed: %v", err)
	}

	if hashA == hashB {
		t.Error("Hashes should differ when item tags change")
	}
}
