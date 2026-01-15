package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/new/main.go \"My New Post Title\"")
		return
	}

	title := os.Args[1]
	// Create a filename like: content/my-new-post-title.md
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	// Remove special chars to be safe
	slug = strings.ReplaceAll(slug, "?", "")
	slug = strings.ReplaceAll(slug, ":", "")
	filename := fmt.Sprintf("content/%s.md", slug)

	// Basic Frontmatter template
	content := fmt.Sprintf(`---
title: "%s"
date: "%s"
description: "Enter a short description here..."
tags: []
pinned: false
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
