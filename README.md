# Kosh - High-Performance Static Site Generator

A high-performance, parallelized Static Site Generator (SSG) built in Go. Designed for personal blogs and documentation sites with a focus on speed, security, and modern Go practices.

## Features

### Core Capabilities
- **Blazing Fast Incremental Builds**: Persistent metadata caching system that intelligently skips re-parsing and re-reading unchanged Markdown files
- **Parallel Build System**: Adaptive worker pools maximize throughput
- **Live Reloading**: Built-in development server with file watching for instant browser refresh
- **Asset Pipeline**: Automatic minification and content-hash fingerprinting for CSS & JS files
- **BoltDB Cache System**: High-performance metadata cache using BoltDB with content-addressed artifact storage
- **Native Rendering**: LaTeX equations and D2 diagrams rendered server-side as inline SVG
- **WASM Search Engine**: Fast, full-text search powered by Go and WebAssembly with BM25 ranking
- **SEO Ready**: Auto-generates `sitemap.xml`, `rss.xml`, and fully optimized meta tags
- **PWA Support**: Service worker with stale-while-revalidate caching

### Content Features
- **Pinned Posts**: Highlight important content with `pinned: true` in frontmatter
- **Pagination**: Automatic splitting of post lists with navigation controls
- **Reading Time Estimation**: Automatic calculation for each article
- **Table of Contents**: Auto-generated from heading tags
- **Image Optimization**: Parallel WebP conversion with progress tracking
- **Knowledge Graph**: Interactive force-directed graph visualization
- **Draft System**: Exclude WIP posts with `draft: true`
- **Weighted Ordering**: Custom sort order for documentation

### Security & Stability
- **BLAKE3 Hashing**: Cryptographically secure content addressing (replaced MD5)
- **Path Validation**: Prevents directory traversal attacks
- **Graceful Shutdown**: Proper cleanup on SIGINT/SIGTERM
- **Context Propagation**: All long-running operations respect cancellation
- **Race Condition Free**: Thread-safe operations using sync.Map and mutexes

### Modern Architecture
- **Service Layer**: Decoupled services (PostService, CacheService, AssetService, RenderService)
- **Dependency Injection**: Constructor-based DI for testability
- **Go Generics**: Type-safe cache operations with `getCachedItem[T any]`
- **Object Pooling**: Reusable `bytes.Buffer` instances to reduce GC pressure
- **Worker Pools**: Generic concurrent processing with context cancellation

## Quick Start

### Prerequisites
- **Go 1.23+** (stable version)

### Installation

```bash
# Install via go install
go install github.com/Kush-Singh-26/kosh/cmd/kosh@latest

# Verify installation
kosh version
```

### Initialize a New Site

```bash
# Initialize project structure
kosh init my-site
cd my-site

# Install a theme (required)
git clone https://github.com/Kush-Singh-26/kosh-theme-blog themes/blog
```

### Theme Structure

A valid theme requires:

```
themes/<theme-name>/
├── templates/
│   ├── layout.html    # Base template (required)
│   ├── index.html     # Home page template (required)
│   ├── 404.html       # Error page (optional)
│   └── graph.html     # Graph view (optional)
├── static/
│   ├── css/           # Stylesheets
│   └── js/            # JavaScript
└── theme.yaml         # Theme metadata (optional)
```

### Minimal theme.yaml

```yaml
name: "My Theme"
supportsVersioning: false
```

## Usage

### Development Mode (Recommended)

For content creation and theme development:

```bash
kosh serve --dev
# Serving on http://localhost:2604 (Auto-reload enabled)
```

- **Speed**: Incremental rebuilds (< 100ms)
- **Features**: File watching, auto-reload, draft preview with `-drafts`

### Production Build

```bash
kosh build
```

Minifies HTML/CSS/JS, compresses images, generates search index.

### Content Management

```bash
# Create a new post
kosh new "Title of new blog"

# Clean build artifacts
kosh clean

# Clean everything including cache (force full rebuild)
kosh clean --cache

# Show version and build info
kosh version
```

### Available Commands

| Command | Description | Flags |
|---------|-------------|-------|
| `build` | Build static site | `-baseurl`, `-drafts`, `--cpuprofile`, `--memprofile` |
| `serve` | Start preview server | `--dev`, `-host`, `-port`, `-drafts` |
| `new` | Create new post | (takes title as argument) |
| `clean` | Clean output | `--cache` (include cache dir) |
| `version` | Show version info | - |
| `cache` | Cache management | `stats`, `gc`, `verify`, `rebuild`, `clear`, `inspect` |

## Architecture

### Service Layer (Refactored)

The codebase follows a modular service-oriented architecture:

```
builder/
├── services/              # Business logic layer
│   ├── post_service.go      # Interface + Process() orchestration
│   ├── post_cache_render.go # RenderCachedPosts() method
│   ├── post_single.go       # ProcessSingle() method
│   ├── social_cards.go     # Social card generation
│   ├── cache_service.go    # Thread-safe cache operations
│   ├── asset_service.go    # Static asset processing
│   └── render_service.go   # HTML template rendering
├── run/                  # Orchestration layer
│   ├── builder.go         # Builder initialization
│   ├── build.go           # Build orchestration
│   └── incremental.go      # Watch mode & fast rebuilds
└── cache/                # Data layer (Refactored)
    ├── cache.go            # Lifecycle: Open, Close, Manager struct
    ├── cache_reads.go     # Get* methods (GetPostByPath, GetPostsByIDs, etc.)
    ├── cache_writes.go    # Write methods (BatchCommit, StoreHTML, etc.)
    ├── cache_queries.go   # Query methods (Stats, ListAllPosts, Hash ops)
    ├── cache_dirty.go     # Dirty tracking (MarkDirty, IsDirty)
    ├── gc_config.go       # GC configuration & ShouldRunGC()
    ├── gc_run.go          # RunGC() core logic
    ├── gc_verify.go       # Verify() integrity checks
    ├── gc_maintenance.go  # Clear(), Rebuild(), IncrementBuildCount()
    ├── store.go           # Content-addressed storage
    └── types.go           # Type definitions
```

### Refactoring Summary (Phases 1-3)

| Phase | Package | Before | After | Max File |
|-------|---------|--------|-------|----------|
| 1 | services/post_service.go | 1,002 lines | 4 files | 579 lines |
| 2 | cache/cache.go | 814 lines | 4 files | 236 lines |
| 3 | cache/gc.go | 399 lines | 4 files | 214 lines |

All large files (>300 lines) have been split into focused, single-responsibility modules.

### Memory Management
   - `utils.BufferPool`: Reusable buffers for markdown rendering
   - `strings.Builder`: Efficient string concatenation
   - Sync.Pool for batch operations

### Search Engine
   - Pre-computed `NormalizedTitle` and `NormalizedTags`
   - No runtime `strings.ToLower` in search hot path
   - BM25 scoring with pre-computed word frequencies

3. **Build Pipeline**
   - Two-pass architecture: Collect metadata → Render HTML
   - Parallel static asset processing
   - Content-addressed storage (BLAKE3 hashes)
   - Inline HTML for small posts (< 32KB)

### Project Structure

```
.
├── builder/               # Core SSG Logic
│   ├── assets/           # Asset processing (esbuild, images)
│   ├── cache/            # BoltDB cache with BLAKE3 hashing (refactored)
│   │   ├── cache.go            # Lifecycle & Manager struct
│   │   ├── cache_reads.go      # Get* methods
│   │   ├── cache_writes.go     # Write methods
│   │   ├── cache_queries.go    # Query & Hash methods
│   │   ├── cache_dirty.go      # Dirty tracking
│   │   ├── gc_config.go        # GC configuration
│   │   ├── gc_run.go           # GC core logic
│   │   ├── gc_verify.go        # Integrity verification
│   │   ├── gc_maintenance.go    # Cache maintenance
│   │   ├── store.go            # Content-addressed storage
│   │   └── types.go            # Type definitions
│   ├── config/           # Configuration loading
│   ├── generators/       # RSS, Sitemap, Graph, Social Cards
│   ├── models/           # Data structures
│   ├── parser/           # Markdown parsing (Goldmark)
│   ├── renderer/         # HTML template rendering
│   ├── run/              # Build orchestration
│   ├── search/           # Search engine (WASM & server-side)
│   ├── services/         # Business logic services (refactored)
│   │   ├── post_service.go      # Interface + Process()
│   │   ├── post_cache_render.go
│   │   ├── post_single.go
│   │   └── social_cards.go
│   └── utils/            # Utilities (pools, fs, hashing)
├── cmd/
│   ├── kosh/            # Main CLI entry point
│   └── search/          # WASM search engine
├── internal/            # Internal packages
├── content/             # Markdown source files
├── public/              # Build output
├── static/              # Static assets
├── themes/              # Theme files
│   ├── blog/           # Blog theme
│   └── docs/           # Documentation theme
└── templates/           # HTML templates
```

## Configuration

### kosh.yaml

```yaml
# Site Configuration
title: "My Blog"
description: "A description of my blog"
logo: "static/images/logo.png"
baseURL: "https://example.com"
language: "en"

# Author
author:
  name: "Author Name"
  url: "https://example.com"

# Navigation
menu:
  - name: "Home"
    url: "/"
  - name: "Tags"
    url: "/tags/index.html"

# Paths
contentDir: "content"
outputDir: "public"
cacheDir: ".kosh-cache"

# Theme
theme: "blog"
themeDir: "themes"

# Versioning (for docs theme)
versions:
  - name: "v2.0 (latest)"
    path: "v2.0"
    isLatest: true

# Features
features:
  rawMarkdown: true
  generators:
    sitemap: true
    rss: true
    graph: true
    pwa: true
    search: true

# Build Settings
postsPerPage: 10
compressImages: true
imageWorkers: 24
```

### Post Frontmatter

```yaml
title: "Modern AI Architectures"
description: "Exploring Transformers and MoE"
date: "2026-01-14"
tags: ["AI", "Architecture"]
pinned: true
weight: 10      # Higher = first in docs
draft: false
image: "/static/images/hero.jpg"  # Custom social card
```

## Development Workflows

### Content & Design Work

When writing posts or tweaking themes:

```bash
kosh serve --dev
```
- Watches `content/`, `themes/`, `static/`, `templates/`
- Auto-reloads browser on changes
- Use `-drafts` to preview unpublished posts

### Core Development (Go Files)

When modifying the SSG engine:

**Terminal 1:**
```bash
air  # Watches Go files, rebuilds kosh
```

**Terminal 2:**
```bash
go run cmd/kosh/main.go serve  # Preview server
```

### Search Engine Development

When modifying the WASM search:

```bash
# Windows PowerShell
$env:GOOS="js"; $env:GOARCH="wasm"; go build -o internal/build/wasm/search.wasm ./cmd/search

# Rebuild CLI (WASM is embedded)
go build -o kosh.exe ./cmd/kosh

# Test
kosh build
```

### Local testing

```powershell
$env:GOPROXY="direct"; go install github.com/Kush-Singh-26/kosh/cmd/kosh@latest
```

```bash
# Install from local directory (not GitHub)
go install ./cmd/kosh
```

## Performance

### Build Times

| Scenario | Time | Notes |
|----------|------|-------|
| Clean Build | ~12s | Full rebuild with cache population |
| Content Edit | ~100ms | Incremental, single post |
| Template Edit | ~1-2s | Invalidates affected posts |
| Image Processing | Parallel | 24 concurrent workers |

### Memory Usage

- **Buffer Pool**: Reusable `bytes.Buffer` instances
- **Object Pooling**: Reduced GC pressure during batch commits
- **Inline HTML**: Small posts (< 32KB) stored in metadata

### Cache Efficiency

- **Cache Hits**: ~95% on typical content edits
- **Cache Storage**: BoltDB with BLAKE3 content addressing
- **Cross-Platform**: Normalized paths (Windows → Linux)

## Security Features

- **BLAKE3 Hashing**: Cryptographically secure (replaced MD5)
- **Path Validation**: Prevents directory traversal
- **Input Sanitization**: All user paths normalized before use
- **Graceful Degradation**: Errors logged, never crash build

## Dependencies

### Core
- **Markdown**: `github.com/yuin/goldmark` v1.7.16
- **Cache**: `go.etcd.io/bbolt` v1.4.3
- **Hashing**: `github.com/zeebo/blake3` v0.2.4
- **Compression**: `github.com/klauspost/compress` v1.18.4

### Extensions
- **Admonitions**: `github.com/stefanfritsch/goldmark-admonitions`
- **Highlighting**: `github.com/yuin/goldmark-highlighting/v2`
- **LaTeX Passthrough**: `github.com/gohugoio/hugo-goldmark-extensions/passthrough`

### Rendering
- **D2 Diagrams**: `oss.terrastruct.com/d2` v0.7.1
- **LaTeX**: `github.com/dop251/goja` (KaTeX via JS)
- **Images**: `github.com/disintegration/imaging`
- **WebP**: `github.com/chai2010/webp`

### Build Tools
- **Minification**: `github.com/tdewolff/minify/v2`
- **Bundling**: `github.com/evanw/esbuild` v0.27.3
- **VFS**: `github.com/spf13/afero`

## Deployment

### GitHub Pages

1. Copy the deployment workflow:
   ```bash
   cp deployment.yaml .github/workflows/deploy.yml
   ```

2. Enable GitHub Pages in repository settings (Source: GitHub Actions)

3. Push to `main` branch triggers automatic deployment

The workflow automatically:
- Builds the Kosh CLI
- Restores cache for incremental builds
- Deploys to `https://<owner>.github.io/<repo>/`

### Custom Domain

To use a custom domain, update `kosh.yaml`:

```yaml
baseURL: "https://yourdomain.com"
```

Or override via CLI:

```bash
kosh build -baseurl https://yourdomain.com
```

### Cache Management

Build caches stored in `.kosh-cache/`:
- Restored between CI runs for incremental builds
- Automatically normalized for cross-platform compatibility
- Use `kosh clean --cache` to force full rebuild

## License

MIT License - See [LICENSE](LICENSE) for details.

## Version History

### v1.1.0 (2026-02-12)
- **Phase 1**: Split `post_service.go` (1,002 → 4 files)
- **Phase 2**: Split `cache/cache.go` (814 → 4 files)
- **Phase 3**: Split `cache/gc.go` (399 → 4 files)
- All large files (>300 lines) refactored into focused modules
- Fixed social card generation on Windows (path separator issue)
- Added tests for parser, search, utils, and cache packages
- TOC transformer test fixed (util.Prioritized import)

### v1.0.0 (2026-02-12)
- Complete security audit and hardening
- Service layer architecture with DI
- Memory optimization with object pooling
- Pre-computed search indexes
- Generic cache operations
- Updated to Go 1.23 with latest dependencies
