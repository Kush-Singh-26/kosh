package new

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/afero"
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
	if len(slug) > 100 {
		slug = slug[:100]
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
		fmt.Println("Usage: kosh new \"My New Post Title\"")
		return
	}

	title := args[0]
	// Create a safe filename slug
	slug := sanitizeSlug(title)
	if slug == "" {
		fmt.Println("❌ Error: Title produces empty slug after sanitization")
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
---

## Introduction

Start writing here...
`, title, time.Now().Format("2006-01-02"))

	// Check if file exists to avoid overwriting
	exists, _ := afero.Exists(fs, filename)
	if exists {
		fmt.Println("❌ Error: File already exists:", filename)
		return
	}

	if err := afero.WriteFile(fs, filename, []byte(content), 0644); err != nil {
		fmt.Println("Error creating file:", err)
		return
	}

	fmt.Printf("✅ Created: %s\n", filename)
}
