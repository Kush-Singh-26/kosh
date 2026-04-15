# Kosh Configuration Reference

This document provides a comprehensive reference for the `kosh.yaml` configuration file.

## Core Metadata

| Field | Description | Default |
| :--- | :--- | :--- |
| `title` | The global site title. | `Kosh Site` |
| `description` | Site description used for SEO and meta tags. | `A Kosh site` |
| `baseURL` | The root URL for your site (no trailing slash). | `""` (root-relative) |

## Navigation & Branding

Kosh supports context-aware branding through the `navbar` section. This allows you to differentiate the identity displayed on the homepage versus the main blog section.

```yaml
navbar:
  home:
    title: "John Doe"
    btnLabel: "My Work"
  posts:
    title: "Modern SSG"
    btnLabel: "Home"
```

| Field | Description | Default |
| :--- | :--- | :--- |
| `logo` | Path to the site logo (relative to `static/`). | `""` |
| `contentPrefix` | URL segment for the main content section. | `blogs` |

## Build Options

| Field | Description | Default |
| :--- | :--- | :--- |
| `theme` | Name of the theme to use (must exist in `themes/`). | `default` |
| `themeDir` | Directory containing themes. | `themes` |
| `contentDir` | Directory containing markdown content. | `content` |
| `staticDir` | Directory containing global static assets. | `static` |
| `outputDir` | Directory where the site will be built. | `public` |
| `postsPerPage` | Number of posts to show per page on indices. | `10` |
| `shouldCompressImages` | Convert raster images to WebP. | `true` |
| `shouldMinifySVGs` | Minify SVG assets during build. | `true` |
| `imageWorkers` | Number of parallel image processing workers. | `8` |

## Features & Generators

Enable or disable specific SSG sub-modules.

```yaml
features:
  rawMarkdown: true      # Emits .md files alongside .html
  generators:
    sitemap: true        # Generate sitemap.xml
    rss: true            # Generate index.xml (Atom/RSS)
    graph: true          # Generate knowledge graph data
    pwa: true            # Generate PWA manifest and service worker
    search: true         # Generate Go+WASM search index
```

## Taxonomies

Define custom taxonomies for your content.

| Field | Description | Default |
| :--- | :--- | :--- |
| `taxonomies` | Map of internal names to plural URL segments. | `tags: tags` |

## Advanced & Performance

| Field | Description | Default |
| :--- | :--- | :--- |
| `parserWorkers` | Parallel markdown parser workers. | `NumCPU` |
| `webpQuality` | Quality setting for WebP conversion (1-100). | `80` |
