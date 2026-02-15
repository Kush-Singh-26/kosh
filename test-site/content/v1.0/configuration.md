---
title: "Configuration v1.0"
description: "Configuration for v1.0"
weight: 80
---

# Configuration v1.0

v1.0 configuration options.

> **Warning:** This page only exists in v1.0. For the latest config, see [Configuration](../configuration.md).

## Basic Configuration

```yaml
# kosh.yaml
baseURL: "https://example.com"
title: "My Site"
theme: "docs"
```

## Available Options

| Option | Type | Description |
|--------|------|-------------|
| `baseURL` | string | Site base URL |
| `title` | string | Site title |
| `theme` | string | Theme name |
| `languageCode` | string | Language code |

## Theme Selection

### Blog Theme

```yaml
theme: "blog"
```

Features:
- Chronological posts
- Tag pages
- RSS feed

### Docs Theme

```yaml
theme: "docs"
```

Features:
- Sidebar navigation
- Table of contents
- Search (basic)

## v1.0 Limitations

Not available in v1.0:
- Version support (added in v2.0)
- WASM search (added in v2.0)
- Service layer (added in v3.0)
- BLAKE3 hashing (added in v4.0)

## Related

- [Getting Started](./getting-started.md)
- [Installation](./installation.md)
- [Tutorial](./guides/tutorial.md)

## Upgrade

See [v4.0 Configuration](../configuration.md) for the latest options.
