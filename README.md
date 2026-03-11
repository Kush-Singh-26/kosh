# Kosh

Kosh is a high-performance static site generator written in Go for blogs and documentation sites. It focuses on fast incremental rebuilds, safe atomic publishing, modern frontend bundling, server-side rendering for math/diagrams, and a Go+WASM search engine.

## Current State

- Version: `v1.3.9`
- Status: production-ready
- Best benchmarked image settings on the reference Windows machine:
  - `imageWorkers: 8`

## Core Features

- Markdown-based content with frontmatter
- Incremental rebuilds in `kosh serve --dev`
- Atomic full-build publishing with staging directories
- BoltDB-backed metadata/search/html cache in `.kosh-cache`
- CSS/JS bundling and fingerprinting via esbuild
- Image optimization to WebP for eligible local `.png`, `.jpg`, `.jpeg` (powered by libvips)
- Exact-copy exceptions for assets like logo/favicon where required
- Server-side LaTeX and D2 rendering
- Go+WASM search with schema validation and snippet extraction
- RSS, sitemap, graph, PWA, social cards

## Install

**Prerequisites:** Kosh requires `libvips` and CGO for high-performance image processing.
- Linux: `apt-get install libvips-dev`
- macOS: `brew install vips`
- Windows: Install vips and set up a GCC toolchain (e.g., MSYS2).

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

## Search

Kosh search uses:

- Go-compiled WASM runtime
- `search.bin` index payload
- explicit schema version validation
- full-body content snippets
- phrase boosting
- prefix/fuzzy support

Important:

- stale `search.wasm` in a site source tree should not override the current deployed WASM
- dev mode keeps `window.buildVersion` in sync so browser fetches stay current

## Image Handling

- eligible local `.png`, `.jpg`, `.jpeg` are converted to `.webp`
- output references are rewritten accordingly
- exact-copy exceptions are preserved where Kosh explicitly requires original files
- benchmarked best settings in this repo were:

```yaml
imageWorkers: 8
```

## Windows Notes

Kosh now uses unique staging/backup directories for clean-build publish. This improves reliability for:

- `kosh clean`
- `kosh clean --cache`

If Windows still reports `Access is denied`, common external causes are:

- Explorer holding the output directory open
- browser tabs reading output files during publish
- antivirus/indexing interference

## Development of Kosh Itself

Install locally after engine changes:

```bash
go install ./cmd/kosh
```

Useful local checks:

```bash
go test ./builder/parser ./builder/services ./builder/run ./builder/utils
go test ./builder/utils ./builder/services ./builder/run ./internal/clean
```

## Repository Structure

```text
builder/
  cache/
  config/
  generators/
  metrics/
  models/
  parser/
  renderer/
  run/
  search/
  services/
  utils/
cmd/
  kosh/
  search/
internal/
themes/
```

## CLI Help Snapshot

- `kosh build` — build the static site
- `kosh serve` — start preview server
- `kosh clean` — clean output directory and rebuild
- `kosh cache` — cache maintenance commands
- `kosh init` — scaffold a new site
- `kosh new` — create a new post
- `kosh version` — version/build information

## License

MIT
