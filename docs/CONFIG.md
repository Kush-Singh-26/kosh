# Kosh Configuration Reference

This document provides a comprehensive reference for the `kosh.yaml` configuration file.

## Core Metadata

| Field | Description | Default |
| :--- | :--- | :--- |
| `title` | The global site title. | `Kosh Site` |
| `description` | Site description used for SEO and meta tags. | `A Kosh site` |
| `baseURL` | The root URL for your site (no trailing slash). | `""` (root-relative) |
| `articleType` | Schema.org type (e.g., `BlogPosting`, `Article`). | `BlogPosting` |
| `homeBadge` | Label for the home page social card badge. | `Latest Items` |

## Navigation & Branding

Kosh supports context-aware branding through the `navbar` section. This allows you to differentiate the identity displayed on the homepage versus the main blog section.

```yaml
navbar:
  home:
    title: "John Doe"
    btnLabel: "My Work"
  section:
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
| `itemsPerPage` | Number of items to show per page on indices. | `10` |
| `shouldCompressImages` | Convert raster images to WebP. | `true` |
| `shouldMinify` | Minify CSS/JS assets and SVGs during build. | `true` |
| `imageWorkers` | Number of parallel image processing workers. | `8` |

## Features & Generators

Enable or disable specific SSG sub-modules.

```yaml
features:
  useRawMarkdown: true   # Emits .md files alongside .html
  generators:
    isSitemapEnabled: true        # Generate sitemap.xml
    isRSSEnabled: true            # Generate index.xml (Atom/RSS)
    graph:
      isEnabled: true    # Generate knowledge graph data
    isPWAEnabled: true            # Generate PWA manifest and service worker
    search:
      isEnabled: true    # Generate Go+WASM search index
      ranking:           # Optional: Tune BM25 and field boosts
        titleBoost: 50.0
        tagBoost: 10.0
        descriptionBoost: 5.0
      endpoints:         # Optional: Federated search indices
        - "/search.bin"
```

| Field | Description | Default |
| :--- | :--- | :--- |
| `useRawMarkdown` | If true, copies raw `.md` files to the output directory alongside `.html` pages. | `false` |
| `generators.search.ranking` | Optional BM25 and field boost weights. | Internal defaults |
| `generators.search.endpoints` | List of URLs to fetch remote search indices from. | `["/search.bin"]` |

## Taxonomies

Define custom taxonomies for your content.

| Field | Description | Default |
| :--- | :--- | :--- |
| `taxonomies` | Map of internal names to plural URL segments. | `tags: tags`, `series: series`, `events: events` |

> [!NOTE]
> **Sorting Specifics**: Taxonomies named `series` or `events` are automatically sorted **chronologically** (earliest first) to support structured narratives and timelines. All other taxonomies are sorted reverse-chronologically.

## Data Directory

Kosh supports a `data/` directory for structured site data (YAML or JSON).

- **Global Context**: Files like `data/team.yaml` are available to templates via `Site.Data.team`.
- **Auto-Generated Pages**: If a data file contains `slug` and `layout` keys at the root, Kosh will automatically render it as a standalone page using the specified layout.

## Advanced & Performance

| Field | Description | Default |
| :--- | :--- | :--- |
| `parserWorkers` | Parallel markdown parser workers. | `NumCPU` |
| `webpQuality` | Quality setting for WebP conversion (1-100). | `80` |
