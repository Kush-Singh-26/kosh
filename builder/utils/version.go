package utils

import (
	"path/filepath"
	"strings"
)

// GetVersionFromPath extracts version from file path
// Input: "content/v2.0/getting-started.md"
// Output: "v2.0", "getting-started.md"
func GetVersionFromPath(path string) (version, relPath string) {
	// Normalize path separators
	path = filepath.ToSlash(path)

	// Check if path contains version folder
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if i == 0 {
			continue // Skip "content"
		}
		if strings.HasPrefix(part, "v") && len(part) > 2 {
			// Found version folder
			version = part
			relPath = strings.Join(parts[i+1:], "/")
			return version, relPath
		}
	}

	// No version found, return empty and full path after content/
	if len(parts) > 1 {
		relPath = strings.Join(parts[1:], "/")
	}
	return "", relPath
}

// BuildPostLink creates a version-aware link for a post
func BuildPostLink(baseURL, version, cleanHtmlRelPath string) string {
	// Handle protocol carefully to avoid stripping slashes from http:// or https://
	// Split protocol from the rest of the URL if present
	protocol := ""
	if strings.Contains(baseURL, "://") {
		parts := strings.SplitN(baseURL, "://", 2)
		protocol = parts[0] + "://"
		baseURL = parts[1]
	}

	baseURL = strings.TrimSuffix(baseURL, "/")
	cleanHtmlRelPath = strings.TrimPrefix(cleanHtmlRelPath, "/")

	res := protocol + baseURL
	if version != "" {
		res += "/" + version
	}
	res += "/" + cleanHtmlRelPath
	return res
}

// GetVersionFromURL extracts version from URL path
// Input: "/v2.0/advanced/configuration.html"
// Output: "v2.0", "/advanced/configuration.html"
func GetVersionFromURL(urlPath string) (version, cleanPath string) {
	urlPath = filepath.ToSlash(urlPath)
	urlPath = strings.TrimPrefix(urlPath, "/")

	parts := strings.Split(urlPath, "/")
	if len(parts) > 0 && strings.HasPrefix(parts[0], "v") && len(parts[0]) > 2 {
		version = parts[0]
		cleanPath = "/" + strings.Join(parts[1:], "/")
		return version, cleanPath
	}

	return "", "/" + urlPath
}

// BuildVersionedURL creates a URL for a specific version
// baseURL: "http://localhost:2604"
// version: "v2.0", ""
// relPath: "advanced/configuration.html"
// Returns: "http://localhost:2604/v2.0/advanced/configuration.html"
func BuildVersionedURL(baseURL, version, relPath string) string {
	// Handle protocol carefully to avoid stripping slashes from http:// or https://
	protocol := ""
	if strings.Contains(baseURL, "://") {
		parts := strings.SplitN(baseURL, "://", 2)
		protocol = parts[0] + "://"
		baseURL = parts[1]
	}

	baseURL = strings.TrimSuffix(baseURL, "/")
	relPath = strings.TrimPrefix(relPath, "/")

	if version == "" {
		return protocol + baseURL + "/" + relPath
	}

	return protocol + baseURL + "/" + version + "/" + relPath
}

// CleanVersionFromLink removes version prefix from a link for tree building
// Input: "http://localhost:2604/v2.0/getting-started.html"
// Output: "http://localhost:2604/getting-started.html"
func CleanVersionFromLink(link string) string {
	// Check for version pattern in URL
	// Match: /v2.0/, /v1.0/, etc.
	if strings.Contains(link, "/v") {
		parts := strings.Split(link, "/")
		for i, part := range parts {
			if i > 0 && strings.HasPrefix(part, "v") && len(part) > 2 {
				// Remove this part from the URL
				newParts := append(parts[:i], parts[i+1:]...)
				return strings.Join(newParts, "/")
			}
		}
	}
	return link
}
