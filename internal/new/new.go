package new

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	maxSlugLength      = 100
	newContentFileMode = 0644
)

type ConfigGetter interface {
	GetContentDir() string
}

// sanitizeSlug converts a title to a safe filename slug
func sanitizeSlug(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := b.String()
	// Remove consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")
	// Limit length to prevent excessively long filenames
	if len(slug) > maxSlugLength {
		slug = slug[:maxSlugLength]
	}
	return slug
}

// RunFs creates a new content file using the provided filesystem
func RunFs(fs afero.Fs, args []string) {
	if len(args) < 1 {
		slog.Info("Usage: kosh new \"My New Content Title\"")
		return
	}

	title := args[0]
	// Create a safe filename slug
	slug := sanitizeSlug(title)
	if slug == "" {
		slog.Error("Title produces empty slug after sanitization")
		return
	}

	contentDir := "content"
	// Basic try-load of kosh.yaml to find ContentDir
	// We don't want to fail if not in a kosh project, but we want to be helpful
	if data, err := afero.ReadFile(fs, "kosh.yaml"); err == nil {
		contentDir = parseContentDirFromYAML(string(data))
	}

	filename := fmt.Sprintf("%s/%s.md", contentDir, slug)

	// Basic Frontmatter template
	content := fmt.Sprintf(`---
title: "%s"
date: "%s"
description: "Enter a short description here..."
tags: []
pinned: false
draft: false
weight: 0
---

## Introduction

Start writing here...
`, title, time.Now().Format("2006-01-02"))

	// Check if file exists to avoid overwriting
	exists, _ := afero.Exists(fs, filename)
	if exists {
		slog.Error("File already exists", "path", filename)
		return
	}

	if err := afero.WriteFile(fs, filename, []byte(content), newContentFileMode); err != nil {
		slog.Error("Error creating file", "error", err)
		return
	}

	slog.Info("Created", "path", filename)
}

func parseContentDirFromYAML(data string) string {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "contentDir:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				dir := strings.TrimSpace(parts[1])
				dir = strings.Trim(dir, "\"")
				dir = strings.Trim(dir, "'")
				if dir != "" {
					return dir
				}
			}
		}
	}
	return "content"
}

// Run creates a new content file in the local filesystem
func Run(args []string) {
	RunFs(afero.NewOsFs(), args)
}
