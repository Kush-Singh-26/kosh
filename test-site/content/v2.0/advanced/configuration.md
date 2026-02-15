---
title: "Configuration v2.0"
description: "Configuration options for v2.0"
weight: 65
---

# Configuration v2.0

v2.0 specific configuration options.

> **Note:** This page only exists in v2.0. For the latest config, see [Configuration](../../configuration.md).

## Version Configuration

```yaml
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
```

### Version Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name |
| `path` | string | URL path prefix |
| `isLatest` | bool | Mark as latest |

## Search Configuration

```yaml
features:
  generators:
    search: true
```

### Search Options

| Option | Default | Description |
|--------|---------|-------------|
| `search` | `true` | Enable search index |

## Theme Configuration

### Docs Theme

```yaml
theme: "docs"
themeDir: "themes"
```

### Available Themes

- `blog` - Chronological blog theme
- `docs` - Documentation theme

## Build Options

```yaml
build:
  drafts: false
```

## v2.0 Specific Notes

- Version system introduced in v2.0
- WASM search requires JavaScript
- Sidebar uses recursive tree

## Related

- [Setup](./setup.md) - Advanced setup
- [Migration](./migration.md) - Migration details
- [v4.0 Configuration](../../configuration.md) - Latest config
