# Custom Go SSG

A high-performance, parallelized Static Site Generator (SSG) built in Go. Designed for personal blogs with focus on speed.

## Features

- **Parallel Build System**: Uses Go routines to process files concurrently, maximizing CPU usage for fast builds.
- **Live Reloading**: Built-in development server with Server-Sent Events (SSE) to instantly reload the browser when files change.
- **Incremental Builds**: Intelligently skips processing files that haven't changed to speed up build times.
- **Frontmatter Caching**: Uses a hash-based caching system to detect frontmatter changes, preventing unnecessary regeneration of social cards and graph data during incremental builds.
- **Pinned Posts**: Highlight important content by setting `pinned: true` in the frontmatter.
- **Pagination**: Automatically splits the post list into manageable pages with navigation controls.
- **Reading Time Estimation**: Automatically calculates and displays estimated reading time for each article.
- **Table of Content Generation**: Automatically generates the TOC based on the heading tags like `#` (`<h1>`), `##` (`<h2>`), etc.
- **Image Optimization**: Automatically converts local images to WebP and generates social sharing cards.
- **Knowledge Graph**: Generates an interactive force-directed graph visualizing connections between posts and tags.
- **WASM Search Engine**: Fast, full-text search powered by Go and WebAssembly with BM25 ranking and tag filtering.
- **Math Support**: LaTeX support using KaTeX for rendering complex mathematical equations.
- **SEO Ready**: Auto-generates `sitemap.xml` (in `/sitemap/`), `rss.xml`, and fully optimized meta tags.
- **PWA Support**: Supports Progressive Web App (PWA) allowing offline use.

---

## Installation & Setup

Ensure you have **Go 1.25+** installed.

1. **Clone the repository**
```bash
git clone "https://github.com/kush-singh-26/blogs.git"
cd blogs
```

2. **Build the Binaries**

```bash
# Build the site builder and search engine (conditional build)
go run cmd/build/main.go

# Build the development server 
go build -o bin/server.exe ./cmd/server/main.go

# Build the content helper 
go build -o bin/new.exe ./cmd/new/main.go
```

---

## Usage

### 1. Development (Live Reload)

For the best experience, run the server and let `air` handle rebuilding.

**Terminal 1 (File Watcher/Builder):**

```bash
# Using Air, build compressed version
air
```

**Terminal 2 (Server):**

```bash
.\bin\server.exe
# Serving on http://localhost:2604 (Auto-reload enabled)
```

### 2. Production Build

To build the site for deployment (minifies HTML/CSS/JS and compresses images):

```bash
.\bin\builder.exe -compress
```

### 3. Content Management

Create a new markdown post with frontmatter automatically populated:

```bash
.\bin\new.exe "<Title of new blog>"
```

**Post Metadata (Frontmatter):**

```yaml
title: "Modern AI Architectures"
description: "Exploring Transformers and MoE"
date: "2026-01-14"
tags: ["AI", "Architecture"]
pinned: true
draft: false
```

**Draft System:**
Set `draft: true` in the frontmatter to exclude a post from the build.

```yaml
title: "WIP Post"
date: "2026-01-10"
draft: true
```

---

## Project Structure

```txt
.
├── .air.toml              # Live-reloading configuration
├── .github/
│   └── workflows/
│       └── deploy.yml     # CI/CD Pipeline
├── .gitignore
├── bin/                   # Compiled executables (ignored by git)
├── builder/               # Core SSG Logic (Packages)
│   ├── assets/
│   │   └── fonts/         # Fonts (Inter) for generating social cards
│   ├── config/            # Configuration loading & CLI flags
│   ├── generators/        # Generators for RSS, Sitemap, Graph, & Social Images
│   ├── models/            # Shared Go structs (PostMetadata, PageData)
│   ├── parser/            # Markdown parsing & context handling
│   ├── renderer/          # HTML template rendering logic
│   ├── search/            # Search engine logic
│   └── utils/             # Utilities (Minification, Hashing, File Ops)
├── cmd/                   # CLI Entry Points
│   ├── build/             # Build orchestrator
│   ├── builder/           # Site builder CLI
│   ├── new/               # Content helper CLI
│   ├── search/            # WASM search engine
│   └── server/            # Local development server
├── content/               # Markdown content files (.md)
├── public/                # Output directory (Generated site)
├── static/                # Static assets
│   ├── css/               # Stylesheets (theme, layout, graph, katex)
│   ├── images/            # Images, icons, & generated social cards
│   ├── js/                # Client-side scripts (graph, katex, search)
│   └── wasm/              # WebAssembly binaries 
├── templates/             # Go HTML templates
│   ├── 404.html           # Error page
│   ├── graph.html         # Knowledge graph visualization
│   ├── index.html         # Home page 
│   └── layout.html        # Master layout wrapper
├── go.mod                 # Go module definition
└── go.sum                 # Go module checksums
```

## Configuration

The `builder` accepts the following flags:

| Flag | Description | Default |
| --- | --- | --- |
| `-compress` | Enables minification and WebP conversion | `false` |
| `-output` | Custom output directory | `public` |

The `server` accepts:

| Flag | Description | Default |
| --- | --- | --- |
| `-host` | Host to bind to (use `0.0.0.0` for LAN) | `localhost` |
| `-port` | Port to listen on | `2604` |

## Dependencies

- **Markdown Engine**: `github.com/yuin/goldmark`
- **Frontmatter Parsing**: `github.com/yuin/goldmark-meta`
- **Syntax Highlighting**: `github.com/yuin/goldmark-highlighting/v2`
- **LaTeX Passthrough**: `github.com/gohugoio/hugo-goldmark-extensions/passthrough`
- **Minification**: `github.com/tdewolff/minify/v2`
- **Image Processing**: `github.com/disintegration/imaging`
- **WebP Encoding**: `github.com/chai2010/webp`fication**: `github.com/tdewolff/minify/v2`
- **Image Processing**: `github.com/disintegration/imaging`
- **WebP Encoding**: `github.com/chai2010/webp`