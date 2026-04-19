# Kosh User Guide

> [!TIP]
> **Documentation Index**
> - [Usage Guide](file:///c:/Users/KIIT0001/blogs/docs/USAGE_GUIDE.md) (This file)
> - [Full Feature List](file:///c:/Users/KIIT0001/blogs/docs/FEATURES.md) (Shortcodes, Math, D2)
> - [Configuration Reference](file:///c:/Users/KIIT0001/blogs/docs/KOSH_EXAMPLE.yaml) (Exhaustive `kosh.yaml`)
> - [Kitchen Sink Example](file:///c:/Users/KIIT0001/blogs/docs/EXAMPLES/KITCHEN_SINK.md) (See everything in action)
> - [Project Structure](file:///c:/Users/KIIT0001/blogs/docs/EXAMPLES/PROJECT_STRUCTURE.md) (Where do files go?)
> - [Developer's Guide](file:///c:/Users/KIIT0001/blogs/docs/DEVELOPERS.md) (Core architecture & WASM)
> - [Theme Guide](file:///c:/Users/KIIT0001/blogs/docs/THEME_GUIDE.md) (Building custom templates)

Welcome to **Kosh**, a high-performance, general-purpose static site generator (SSG) built in Go. This guide will walk you through setting up a new project, managing content, and building your site.


## Installation

Currently, Kosh is built from source. Ensure you have [Go](https://golang.org/dl/) installed (v1.26 or later).

```bash
# Clone the repository
git clone https://github.com/Kush-Singh-26/kosh.git
cd kosh

# Build the binary
go build -o kosh ./cmd/kosh

# (Optional) Install to your GOPATH
go install ./cmd/kosh
```

## Quick Start

1. **Initialize a site**:
   Create a directory for your project and add a `kosh.yaml` configuration file.
   (See [KOSH_EXAMPLE.yaml](file:///c:/Users/KIIT0001/blogs/docs/KOSH_EXAMPLE.yaml) for a full template).

2. **Add Content**:
   Create a `content/` folder and add your first Markdown file:
   ```markdown
   ---
   title: "Hello Kosh"
   date: 2026-04-16
   tags: ["intro"]
   ---
   Welcome to my new site!
   ```

3. **Start Development Server**:
   ```bash
   kosh serve --dev
   ```
   This will build your site and start a local server with live-reloading. Changes to content or assets will trigger instant rebuilds.

## Core Concepts

### 1. Project Structure
Kosh follows a simple convention-based structure:
- `kosh.yaml`: Site configuration.
- `content/`: Markdown files for your pages and blog posts.
- `static/`: Global static assets (images, fonts, scripts).
- `themes/`: Theme templates and theme-specific assets.
- `layouts/`: (Optional) Site-level template overrides.
- `public/`: The generated static site (output folder).

### 2. Execution Modes
Kosh provides three primary commands for different workflows:

- `kosh build`: A standard production build. Uses atomic staging to ensure your site is never in a partial state.
- `kosh serve --dev`: Starts a local server at `http://localhost:8080`. Enables watch-mode for incremental fast rebuilds.
- `kosh doctor`: Runs a diagnostic suite to verify site health, performance, and asset integrity.
- `kosh clean`: Removes the `public/` folder but preserves the cache for fast subsequent builds.
- `kosh clean --cache`: A "cold build" trigger. Removes both the output and the `.kosh-cache` directory. Use this when you make major configuration changes.

### 3. Kosh HUD (Dev Dashboard)
While running `kosh serve --dev`, you can access the **Kosh HUD** at `http://localhost:2604/_kosh` (or your configured port).
- **Live Health Stats**: Monitor cache performance and image conversion.
- **Diagnostics**: View A11y warnings and build logs in real-time.

## Advanced Features

### 1. Data-Driven Pages
You can generate pages directly from YAML or JSON files in your `data/` directory. If a file contains a `slug` and a `layout` (referencing a template in your theme), Kosh will build a standalone page for it.

**Example `data/portfolio.yaml`**:
```yaml
slug: "portfolio"
layout: "home"
projects:
  - name: "Kosh SSG"
    desc: "A high-performance Go SSG"
```

### 2. Taxonomies & Sorting
Kosh supports native "Series" and "Events" taxonomies. 
- **Series**: Automatically sorted chronologically (ascending) to maintain reading order.
- **Events**: Sorted chronologically for timeline views.
- **Others**: (Tags, Categories) continue to be sorted reverse-chronologically.

## Site Branding
Kosh supports context-aware branding. You can define different titles and button labels for the Home page vs the Blog section in your `kosh.yaml`. This is useful for professional portfolios that also host a blog.

## Next Steps
- Learn about [Shortcodes and Advanced Features](file:///c:/Users/KIIT0001/blogs/docs/FEATURES.md).
- See the [Kitchen Sink](file:///c:/Users/KIIT0001/blogs/docs/EXAMPLES/KITCHEN_SINK.md) for a full demonstration of Markdown capabilities.
- If you're building a custom theme, check the [Theme Guide](file:///c:/Users/KIIT0001/blogs/docs/THEME_GUIDE.md).
