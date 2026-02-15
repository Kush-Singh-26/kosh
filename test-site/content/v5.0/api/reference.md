---
title: "API Reference"
description: "Complete API reference"
weight: 50
---

# API Reference

Complete API documentation for Kosh.

## CLI Commands

### `kosh build`

Build the static site.

```bash
kosh build [flags]

Flags:
  --config string   Config file (default "kosh.yaml")
  --dev             Development mode
```

### `kosh serve`

Start development server.

```bash
kosh serve [flags]

Flags:
  --dev             Enable live reload and watch mode
  --port int        Port number (default 2604)
```

### `kosh clean`

Clean output directories.

```bash
kosh clean [flags]

Flags:
  --cache           Also clean cache directory
```

### `kosh version`

Display version information.

```bash
kosh version [flags]

Flags:
  --verbose         Show detailed build info
```

## Configuration API

### Site Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `baseURL` | string | - | Base URL of the site |
| `title` | string | - | Site title |
| `theme` | string | `blog` | Theme name |
| `languageCode` | string | `en-us` | Language code |
| `contentDir` | string | `content` | Content directory |
| `outputDir` | string | `public` | Output directory |
| `cacheDir` | string | `.kosh-cache` | Cache directory |

### Version Configuration

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name |
| `path` | string | URL path prefix |
| `isLatest` | bool | Mark as latest version |

## Front Matter

Page metadata in YAML format:

```yaml
---
title: "Page Title"
description: "Page description"
date: "2026-02-13"
weight: 100
tags: ["tag1", "tag2"]
draft: false
---
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Page title |
| `description` | string | Meta description |
| `date` | string | Publication date |
| `weight` | int | Navigation order |
| `tags` | []string | Page tags |
| `draft` | bool | Skip in production |

## Related

- [API Examples](./examples.md) - Code examples
- [SDK](./sdk.md) - SDK documentation
- [Getting Started](../getting-started.md) - Quick start
