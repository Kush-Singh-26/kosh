package utils

import (
	"path/filepath"
	"strings"
	"unicode"

	"my-ssg/builder/models"
)

// simpleTitle capitalizes the first letter of each word
// Simple replacement for deprecated strings.Title
func simpleTitle(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

// BuildBreadcrumbs generates breadcrumbs from URL path and SiteTree
// urlPath: "/v2.0/advanced/configuration.html" or "/advanced/configuration.html" or full URL
// siteTree: tree nodes for the current version
// baseURL: "http://localhost:2604"
// Returns breadcrumb trail excluding the version prefix
func BuildBreadcrumbs(urlPath string, siteTree []*models.TreeNode, baseURL string) []models.Breadcrumb {
	// Strip baseURL if present to get just the path
	pathOnly := strings.TrimPrefix(urlPath, baseURL)

	// Extract version and clean path
	_, cleanPath := GetVersionFromURL(pathOnly)
	cleanPath = strings.TrimSuffix(cleanPath, ".html")
	cleanPath = strings.Trim(cleanPath, "/")

	breadcrumbs := []models.Breadcrumb{
		{Title: "Home", Link: baseURL + "/", IsCurrent: false},
	}

	if cleanPath == "" {
		// Home page
		breadcrumbs[0].IsCurrent = true
		return breadcrumbs
	}

	// Split path into segments
	segments := strings.Split(cleanPath, "/")

	// Build breadcrumb trail by traversing SiteTree
	currentNodes := siteTree
	var currentPath string

	for i, segment := range segments {
		isLast := i == len(segments)-1
		currentPath = filepath.Join(currentPath, segment)

		// Find matching node in current level
		var found *models.TreeNode
		for _, node := range currentNodes {
			nodePath := strings.TrimSuffix(node.Link, ".html")
			nodePath = strings.TrimPrefix(nodePath, baseURL+"/")
			nodePath = strings.TrimPrefix(nodePath, "/")

			if strings.HasSuffix(nodePath, segment) || strings.HasSuffix(nodePath, currentPath) {
				found = node
				break
			}
		}

		if found != nil {
			breadcrumb := models.Breadcrumb{
				Title:     found.Title,
				IsCurrent: isLast,
			}

			if !isLast {
				breadcrumb.Link = found.Link
			}

			breadcrumbs = append(breadcrumbs, breadcrumb)

			// Move to children for next iteration
			currentNodes = found.Children
		} else {
			// Node not found in tree, use segment as title
			title := strings.ReplaceAll(segment, "-", " ")
			title = simpleTitle(title)

			breadcrumb := models.Breadcrumb{
				Title:     title,
				IsCurrent: isLast,
			}

			if !isLast {
				breadcrumb.Link = baseURL + "/" + currentPath + ".html"
			}

			breadcrumbs = append(breadcrumbs, breadcrumb)
		}
	}

	return breadcrumbs
}

// SimpleBreadcrumb creates a simple breadcrumb from URL segments
// Used as fallback when SiteTree is not available
func SimpleBreadcrumbs(urlPath string, baseURL string) []models.Breadcrumb {
	// Strip baseURL if present to get just the path
	pathOnly := strings.TrimPrefix(urlPath, baseURL)
	_, cleanPath := GetVersionFromURL(pathOnly)
	cleanPath = strings.TrimSuffix(cleanPath, ".html")
	cleanPath = strings.Trim(cleanPath, "/")

	breadcrumbs := []models.Breadcrumb{
		{Title: "Home", Link: baseURL + "/", IsCurrent: cleanPath == ""},
	}

	if cleanPath == "" {
		return breadcrumbs
	}

	segments := strings.Split(cleanPath, "/")
	var currentPath string

	for i, segment := range segments {
		isLast := i == len(segments)-1
		currentPath = filepath.Join(currentPath, segment)

		title := strings.ReplaceAll(segment, "-", " ")
		title = simpleTitle(title)

		breadcrumb := models.Breadcrumb{
			Title:     title,
			IsCurrent: isLast,
		}

		if !isLast {
			breadcrumb.Link = baseURL + "/" + currentPath + ".html"
		}

		breadcrumbs = append(breadcrumbs, breadcrumb)
	}

	return breadcrumbs
}
