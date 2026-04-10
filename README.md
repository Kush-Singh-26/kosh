![Kosh Logo](docs/logo.svg)

# Kosh

Kosh is a high-performance static site generator written in Go for blogs. It focuses on fast incremental rebuilds, safe atomic publishing, modern frontend bundling, server-side rendering for math/diagrams, and a Go+WASM search engine.

## Current State

- Version: `v1.4.0`
- Status: production-ready
- Best benchmarked image settings on the reference Windows machine:
  - `imageWorkers: 8`

## Core Features

- Markdown-based content with frontmatter
- Incremental rebuilds in `kosh serve --dev`
- Atomic full-build publishing with staging directories
- BoltDB-backed metadata/search/html cache in `.kosh-cache`
- Persistent SSR cache for D2 diagrams and LaTeX math
- CSS/JS bundling and fingerprinting via esbuild
- Image optimization to WebP for eligible local `.png`, `.jpg`, `.jpeg`
- Exact-copy exceptions for assets like logo where required
- Server-side LaTeX and D2 rendering
- Go+WASM search with schema validation and snippet extraction
- RSS, sitemap, graph, PWA, social cards

## Install

Enable CGO and install:
```bash
export CGO_ENABLED=1
go install ./cmd/kosh
```

For local development of Kosh itself, install from this repository root so the latest CLI is used.

## Quick Start

```bash
kosh init my-site
cd my-site
kosh serve --dev
```

## Main Commands

```bash
kosh build
kosh serve --dev
kosh clean
kosh clean --cache
kosh new "My Post"
kosh version --info
```

### Debug Output

Use the `--debug` (or `-d`) flag to enable debug-level logging:

```bash
kosh build --debug
kosh serve --dev --debug
```

This enables verbose output including:
- Debug-level log messages (asset discovery, routing requests)
- Verbose reporter output
- Asset pipeline debug information

### Command Behavior Notes

- `kosh clean`
  - removes output and immediately rebuilds
  - preserves `.kosh-cache`
- `kosh clean --cache`
  - removes output and `.kosh-cache`
  - forces a true cold rebuild
- `kosh serve --dev`
  - builds, watches, serves, and auto-reloads
  - content body-only edits use a true single-post incremental path
  - CSS/JS edits rebuild assets and rerender HTML with fresh hashed references

## Configuration

Minimal `kosh.yaml`:

```yaml
title: "My Site"
description: "A Kosh site"
baseURL: "https://example.com"
theme: "blog"
themeDir: "themes"
contentDir: "content"
outputDir: "public"
cacheDir: ".kosh-cache"

postsPerPage: 10
compressImages: true
imageWorkers: 8

features:
  rawMarkdown: true
  generators:
    sitemap: true
    rss: true
    graph: true
    pwa: true
    search: true
```

### Important Config Notes

- `imageWorkers`
  - controls image work concurrency
- `compressImages: true`
  - converts eligible local raster images to `.webp`
- `rawMarkdown: true`
  - emits `.md` files alongside generated `.html`

## Content and Theme Layout

Kosh uses standard Markdown files with YAML frontmatter.

### Frontmatter Example

```yaml
---
title: "My Post Title"
date: "2026-03-21"
description: "A short summary of the post"
tags: ["go", "static-site"]
pinned: false
draft: false
weight: 10  # Optional: Higher weight posts appear first
---
```

Typical project layout:

```text
content/
static/
themes/
  blog/
    templates/
    static/
kosh.yaml
```

Theme requirements:

```text
themes/<theme>/
  templates/
    layout.html
    index.html
    404.html      # optional
    graph.html    # optional
  static/
    css/
    js/
    images/
```

## Build Model

### Full Build

Full builds perform these major phases:

1. metadata scan
2. asset pipeline
3. markdown parse/render
4. site-wide generators
5. atomic publish

### Incremental Dev Rebuilds

- body-only markdown edits:
  - single-post rebuild
  - cache commit (incremental)
  - search index regeneration
- CSS/JS edits:
  - asset rebuild only
  - HTML rerender with new asset map
- search source edits:
  - rebuild search WASM only when search source actually changed

## Performance Notes

### What is fast now

- warm-cache full rebuilds (`kosh clean`) are fast
- watch-mode markdown rebuilds are true incremental rebuilds
- CSS edits in dev mode no longer lag by one build

### What still dominates cold builds

- `Asset copy root/static`
- markdown parse phase

This is expected for `kosh clean --cache`, because caches are intentionally removed and image transforms must run again.

## GitHub Actions Release Flow

Kosh releases are built by GitHub Actions when a version tag is pushed. The workflow is defined in `.github/workflows/release.yml` and is triggered by tags that match `v*`.

### How to publish a new release

1. Update the version string and build date in `cmd/kosh/version.go`.
2. Update the version in this README if you list it under **Current State**.
3. Commit and push your changes.
4. Create a new tag and push it.

```bash
git status
git add -A
git commit -m "Release v1.4.2"
git push origin main
git tag v1.4.2
git push origin v1.4.2
```

After the tag is pushed, the workflow uploads the binaries to the GitHub Release for that tag. The release artifacts should appear under `Releases` on GitHub.

### If you need to re-tag

Only do this if the tag is wrong and you have not published widely:

```bash
git tag -d v1.4.2
git push origin --delete v1.4.2
git tag v1.4.2
git push origin v1.4.2
```

## Local Theme Development (Windows)

The canonical theme path is the Kosh submodule:

- `C:\Users\KIIT0001\blogs\themes\blog`

Two local junctions point to the same files so edits are shared instantly:

- `C:\Users\KIIT0001\kosh-theme-blog`
- `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src\themes\blog`

If you ever need to recreate the junctions:

```powershell
New-Item -ItemType Junction -Path "C:\Users\KIIT0001\kosh-theme-blog" -Target "C:\Users\KIIT0001\blogs\themes\blog"
New-Item -ItemType Junction -Path "C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src\themes\blog" -Target "C:\Users\KIIT0001\blogs\themes\blog"
```

Commit and push theme changes from the canonical path only.


