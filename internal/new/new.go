package new

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// slugRegex matches characters that are unsafe for filenames/URLs
var slugRegex = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// sanitizeSlug converts a title to a safe filename slug
func sanitizeSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)
	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove or replace unsafe characters
	slug = slugRegex.ReplaceAllString(slug, "")
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
	if _, err := os.Stat(filename); err == nil {
		fmt.Println("❌ Error: File already exists:", filename)
		return
	}

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Println("Error creating file:", err)
		return
	}

	fmt.Printf("✅ Created: %s\n", filename)
}
