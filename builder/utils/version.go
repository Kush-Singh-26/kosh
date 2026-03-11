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
	if idx := strings.Index(path, "content/"); idx != -1 {
		path = path[idx:]
	}

	// Check if path contains version folder
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Version folders usually start with 'v' and followed by numbers (e.g. v1.0, v2.0)
		// Or they might be just version numbers if configured that way,
		// but Kosh convention seems to be v-prefix.
		if strings.HasPrefix(part, "v") && len(part) >= 2 {
			// Check if it looks like a version (v followed by digit)
			if len(part) > 1 && part[1] >= '0' && part[1] <= '9' {
				version = part
				relPath = strings.Join(parts[i+1:], "/")
				return version, relPath
			}
		}
	}

	// No version found, return empty and full path after content/ if present
	if len(parts) > 0 && parts[0] == "content" {
		relPath = strings.Join(parts[1:], "/")
	} else {
		relPath = path
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
