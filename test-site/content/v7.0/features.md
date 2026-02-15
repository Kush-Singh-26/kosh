---
title: "Features"
description: "Complete feature overview"
weight: 80
---

Kosh is a high-performance Static Site Generator with powerful features.

## Core Features

### Fast Builds

- **Incremental compilation** - Only rebuild what changed
- **Parallel processing** - Multi-threaded asset processing
- **Smart caching** - BoltDB-powered content cache

### Markdown Support

- **Goldmark engine** - Full CommonMark compliance
- **Admonitions** - Callout blocks (note, warning, tip)
- **Syntax highlighting** - Chroma-based code highlighting
- **Custom extensions** - Transform URLs, server-side rendering

### Themes

- **Blog theme** - Chronological content with tags
- **Docs theme** - Documentation hub with versioning

## Documentation Features

### Version Support

- Multiple documentation versions
- Version-specific navigation
- Outdated version banners
- Client-side version switching

### Search

- **WASM-powered** - Client-side search engine
- **BM25 ranking** - Relevant search results
- **Snippets** - Context in search results
- **Keyboard navigation** - Power user friendly

### Navigation

- **Recursive sidebar** - Nested navigation tree
- **Breadcrumbs** - Show current location
- **Prev/Next** - Sequential navigation
- **Table of contents** - In-page navigation

## Developer Experience

### CLI Commands

```bash
kosh build          # Build the site
kosh serve --dev    # Development server with live reload
kosh clean          # Clean output directory
kosh version        # Show version info
```

### Configuration

YAML-based configuration with sensible defaults:

```yaml
baseURL: "https://example.com"
title: "My Site"
theme: "docs"

features:
  generators:
    sitemap: true
    search: true
    pwa: true
```

## Performance Features

| Feature | Description |
|---------|-------------|
| Memory Pools | Reusable buffers reduce GC pressure |
| Worker Pools | Concurrent task processing |
| BLAKE3 Hashing | Fast cryptographic hashing |
| Content-addressed Storage | Deduplication of content |

## PWA Support

- **Service Worker** - Offline capability
- **Web App Manifest** - Install on mobile
- **Asset hashing** - Cache busting

## Cross-Reference Links

- [Getting Started](./getting-started.md) - Quick start guide
- [Configuration](./configuration.md) - Configuration options
- [API Reference](./api/reference.md) - API documentation
- [Changelog](./changelog.md) - Version history

## Version-Specific Features

Different versions have different capabilities:

- [v3.0 Features](./v3.0/index.md) - Previous stable
- [v2.0 Features](./v2.0/new-in-v2.md) - What was new in v2.0
- [v1.0 Features](./v1.0/index.md) - Legacy version
