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

// BuildURL creates a version-aware URL
func BuildURL(baseURL, version, relPath string) string {
	// Handle protocol carefully to avoid stripping slashes from http:// or https://
	protocol := ""
	if strings.Contains(baseURL, "://") {
		parts := strings.SplitN(baseURL, "://", 2)
		protocol = parts[0] + "://"
		baseURL = parts[1]
	}

	baseURL = strings.TrimSuffix(baseURL, "/")
	relPath = strings.TrimPrefix(relPath, "/")

	res := protocol + baseURL
	if version != "" {
		res += "/" + version
	}
	res += "/" + relPath
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
