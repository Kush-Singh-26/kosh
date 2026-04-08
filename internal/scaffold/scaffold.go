package scaffold

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/afero"
)

const (
	scaffoldDirMode  = 0755
	scaffoldFileMode = 0644
)

const defaultKoshYaml = `# Site Configuration
title: "My Kosh Site"
description: "A new site built with Kosh"
baseURL: "http://localhost:2604"
language: "en"

author:
  name: "Author Name"
  url: "https://example.com"

# Navigation
menu:
  - name: "Home"
    url: "/"
  - name: "Tags"
    url: "/tags/index.html"

# Features
postsPerPage: 10
compressImages: true

# Theme Configuration
theme: "blog"
themeDir: "themes"
# templateDir and staticDir will default to themes/<theme>/templates and themes/<theme>/static
`

const firstPost = `---
title: "Hello World"
date: "%s"
tags: ["kosh", "welcome"]
draft: false
---

# Welcome to Kosh!

This is your first post. You can edit this file in ` + "`content/hello-world.md`" + `.

## Getting Started

1.  **Themes**: Kosh requires a theme. Install the official blog theme:
    ` + "```bash" + `
    git clone https://github.com/Kush-Singh-26/kosh-theme-blog themes/blog
    ` + "```" + `
    
    Or create your own theme with this structure:
    ` + "```" + `
    themes/your-theme/
    ├── templates/
    │   ├── layout.html
    │   └── index.html
    ├── static/
    │   ├── css/
    │   └── js/
    └── theme.yaml
    ` + "```" + `

2.  **Run**: Start the dev server.
    ` + "```bash" + `
    kosh serve --dev
    ` + "```" + `
`

// Run initializes a new Kosh project
func Run(args []string) {
	RunFs(afero.NewOsFs(), args)
}

// RunFs initializes a new Kosh project using the provided filesystem
func RunFs(fs afero.Fs, args []string) {
	slog.Info("Initializing new Kosh project...")

	// 1. Create Directories
	dirs := []string{
		"content",
		"themes",
		"public",
		"static",
	}

	for _, dir := range dirs {
		if err := fs.MkdirAll(dir, scaffoldDirMode); err != nil {
			slog.Error("Failed to create directory", "dir", dir, "error", err)
			return
		}
		slog.Info("Created directory", "path", dir+"/")
	}

	// 2. Create kosh.yaml
	exists, _ := afero.Exists(fs, "kosh.yaml")
	if !exists {
		if err := afero.WriteFile(fs, "kosh.yaml", []byte(defaultKoshYaml), scaffoldFileMode); err != nil {
			slog.Error("Failed to create kosh.yaml", "error", err)
			return
		}
		slog.Info("Created kosh.yaml")
	} else {
		slog.Warn("kosh.yaml already exists, skipping")
	}

	// 3. Create first post
	exists, _ = afero.Exists(fs, "content/hello-world.md")
	if !exists {
		content := fmt.Sprintf(firstPost, time.Now().Format("2006-01-02"))
		if err := afero.WriteFile(fs, "content/hello-world.md", []byte(content), scaffoldFileMode); err != nil {
			slog.Error("Failed to create first post", "error", err)
		} else {
			slog.Info("Created content/hello-world.md")
		}
	}

	slog.Info("Project initialized successfully!")
	slog.Info("👉 Clone a theme into themes/ to get started.")
}
