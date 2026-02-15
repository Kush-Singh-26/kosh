package utils

import (
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// BuildSiteTree constructs a hierarchical tree from a flat list of posts
// It infers structure from file paths (e.g., docs/section/page.md)
// currentPath optionally specifies the current page to mark as Active
func BuildSiteTree(posts []models.PostMetadata, currentPath string) []*models.TreeNode {
	// Map to track created nodes by path -> node
	// Path used here is the logical section path (e.g., "docs/section")
	nodeMap := make(map[string]*models.TreeNode)

	// Root nodes
	var roots []*models.TreeNode

	// 1. Create nodes for all posts
	for _, p := range posts {
		// remove baseurl to get relative link
		// Link: https://example.com/docs/page.html -> /docs/page.html
		// But better to use the file path or ID logic?
		// PostMetadata doesn't strictly have the relative Source path here easily accessible
		// unless we pass it. But we can infer from the Link if it follows structure.
		// However, Link includes BaseURL.
		// Let's rely on the fact that `posts` are usually sorted.

		// Actually, we need the relative path to build the tree correctly.
		// The Link might be permalinked differently.
		// But for a Sidebar, we usually want to follow the URL structure.

		// Let's assume the Link structure reflects the hierarchy.
		// We'll strip BaseURL if present logic fails, but we can't easily.
		// Standard way: Organize by implied path components.

		// Parse URL path
		// Remove protocol/domain
		path := p.Link
		if strings.HasPrefix(path, "http") {
			// Find 3rd slash
			parts := strings.SplitN(path, "/", 4)
			if len(parts) >= 4 {
				path = parts[3] // "docs/page.html"
			} else {
				path = "" // Root?
			}
		}
		path = strings.TrimPrefix(path, "/")

		// If this is a versioned post, ignore the version prefix for tree structure
		// e.g. "v2.0/advanced/config.html" -> "advanced/config.html"
		if p.Version != "" {
			path = strings.TrimPrefix(path, p.Version+"/")
		}

		// Clean the path: remove .html
		cleanPath := strings.TrimSuffix(path, ".html")

		components := strings.Split(cleanPath, "/")

		// If it's a root page (e.g. "about"), it's a root node.
		if len(components) == 1 && components[0] != "" {
			node := &models.TreeNode{
				Title:     p.Title,
				Link:      p.Link,
				Weight:    p.Weight,
				IsSection: false,
			}
			roots = append(roots, node)
			continue
		}

		// Find or Create Parent
		var parent *models.TreeNode
		currentPath := ""

		for i := 0; i < len(components)-1; i++ {
			comp := components[i]
			if currentPath == "" {
				currentPath = comp
			} else {
				currentPath = currentPath + "/" + comp
			}

			// Check if this section already exists as a root or child
			if existing, ok := nodeMap[currentPath]; ok {
				parent = existing
			} else {
				// Create virtual section node
				newNode := &models.TreeNode{
					Title:     cases.Title(language.English).String(comp), // Fallback title
					Link:      "",                                         // Section might not have a link if no _index.md
					Weight:    0,
					IsSection: true,
					Children:  []*models.TreeNode{},
				}
				nodeMap[currentPath] = newNode

				if parent == nil {
					roots = append(roots, newNode)
				} else {
					parent.Children = append(parent.Children, newNode)
				}
				parent = newNode
			}
		}

		// Add the leaf node (the post itself)
		leafNode := &models.TreeNode{
			Title:     p.Title,
			Link:      p.Link,
			Weight:    p.Weight,
			IsSection: false,
			Active:    currentPath != "" && p.Link == currentPath,
		}

		// Check if this leaf is actually an _index for the parent?
		// e.g. docs/section/index.html
		lastComp := components[len(components)-1]
		if (lastComp == "index" || lastComp == "_index") && parent != nil {
			// Enhance the parent section with this post's info
			parent.Title = p.Title
			parent.Link = p.Link
			parent.Weight = p.Weight
			// Don't add as child
		} else {
			if parent != nil {
				parent.Children = append(parent.Children, leafNode)
			} else {
				roots = append(roots, leafNode)
			}
		}
	}

	// Sort recursively
	SortTree(roots)

	return roots
}

func SortTree(nodes []*models.TreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		// Sort by Weight desc, then Title asc
		if nodes[i].Weight != nodes[j].Weight {
			return nodes[i].Weight > nodes[j].Weight
		}
		return nodes[i].Title < nodes[j].Title
	})

	for _, n := range nodes {
		if n.Children != nil {
			SortTree(n.Children)
		}
	}
}
