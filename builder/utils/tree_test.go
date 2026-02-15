package utils

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestBuildSiteTree(t *testing.T) {
	posts := []models.PostMetadata{
		// Root level
		{Link: "http://site.com/about.html", Title: "About", Weight: 10},

		// Section A
		{Link: "http://site.com/docs/intro.html", Title: "Introduction", Weight: 5},
		{Link: "http://site.com/docs/setup.html", Title: "Setup", Weight: 4},

		// Section B (Nested)
		{Link: "http://site.com/api/v1/auth.html", Title: "Auth API", Weight: 2},
		{Link: "http://site.com/api/v1/users.html", Title: "Users API", Weight: 1},

		// Section C (Index)
		{Link: "http://site.com/guides/index.html", Title: "Guides Index", Weight: 20},
		{Link: "http://site.com/guides/advanced.html", Title: "Advanced Guide", Weight: 10},
	}

	roots := BuildSiteTree(posts)

	// Expected Structure:
	// - About
	// - docs
	//   - Introduction
	//   - Setup
	// - api
	//   - v1
	//     - Auth API
	//     - Users API
	// - Guides Index (guides section)
	//   - Advanced Guide

	if len(roots) != 4 {
		t.Errorf("expected 4 root nodes, got %d", len(roots))
		for _, r := range roots {
			t.Logf("Root: %s", r.Title)
		}
	}

	// Helper to find node by title
	findNode := func(nodes []*models.TreeNode, title string) *models.TreeNode {
		for _, n := range nodes {
			if n.Title == title {
				return n
			}
		}
		return nil
	}

	// 1. Check Root Page
	about := findNode(roots, "About")
	if about == nil {
		t.Error("Root node 'About' not found")
	}

	// 2. Check Simple Section
	docs := findNode(roots, "Docs") // Auto-titled
	if docs == nil {
		t.Error("Section 'Docs' not found")
	} else if len(docs.Children) != 2 {
		t.Errorf("Docs should have 2 children, got %d", len(docs.Children))
	}

	// 3. Check Nested Section
	api := findNode(roots, "Api")
	if api == nil {
		t.Error("Section 'Api' not found")
	} else {
		v1 := findNode(api.Children, "V1")
		if v1 == nil {
			t.Error("Nested section 'V1' not found")
		} else if len(v1.Children) != 2 {
			t.Errorf("V1 should have 2 children, got %d", len(v1.Children))
		}
	}

	// 4. Check Index Page Logic
	guides := findNode(roots, "Guides Index")
	if guides == nil {
		t.Error("Section 'Guides Index' not found (index merging failed)")
	} else {
		// Note: BuildSiteTree implementation keeps IsSection=true for virtual nodes
		// but if we merged an index page, it takes the index page's title.
		// IsSection might still be true or false depending on logic.
		// The current logic just updates Title/Link/Weight but leaves IsSection=true if created as virtual first.
		if len(guides.Children) != 1 {
			t.Errorf("Guides should have 1 child (Advanced), got %d", len(guides.Children))
		}
	}
}

func TestSortTree(t *testing.T) {
	nodes := []*models.TreeNode{
		{Title: "B", Weight: 10},
		{Title: "A", Weight: 10},
		{Title: "C", Weight: 20},
	}

	SortTree(nodes)

	if nodes[0].Title != "C" {
		t.Errorf("Expected first node 'C' (Weight 20), got '%s'", nodes[0].Title)
	}
	if nodes[1].Title != "A" {
		t.Errorf("Expected second node 'A' (Weight 10, Title 'A'), got '%s'", nodes[1].Title)
	}
	if nodes[2].Title != "B" {
		t.Errorf("Expected third node 'B' (Weight 10, Title 'B'), got '%s'", nodes[2].Title)
	}
}
