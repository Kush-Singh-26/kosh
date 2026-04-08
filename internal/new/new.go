package new

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	maxSlugLength   = 100
	newPostFileMode = 0644
)

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

// Run creates a new blog post file
func Run(args []string) {
	RunFs(afero.NewOsFs(), args)
}

// RunFs creates a new blog post file using the provided filesystem
func RunFs(fs afero.Fs, args []string) {
	if len(args) < 1 {
		slog.Info("Usage: kosh new \"My New Post Title\"")
		return
	}

	title := args[0]
	// Create a safe filename slug
	slug := sanitizeSlug(title)
	if slug == "" {
		slog.Error("Title produces empty slug after sanitization")
		return
	}
	filename := fmt.Sprintf("content/%s.md", slug)

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

	if err := afero.WriteFile(fs, filename, []byte(content), newPostFileMode); err != nil {
		slog.Error("Error creating file", "error", err)
		return
	}

	slog.Info("Created", "path", filename)
}
