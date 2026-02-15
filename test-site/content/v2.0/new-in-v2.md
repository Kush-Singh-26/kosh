---
title: "What's New in v2.0"
description: "Features introduced in version 2.0"
weight: 85
---

# What's New in v2.0

This page documents features that were **introduced in v2.0**.

> **Note:** This page only exists in v2.0. For the latest features, see [Changelog](../changelog.md).

## Version System

v2.0 introduced the documentation versioning system.

### Configuration

```yaml
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
```

### Features

- Multiple documentation versions
- Version-specific navigation trees
- Outdated version banners
- Version selector dropdown
- Client-side version switching

## Search Engine

### WASM-Based Search

v2.0 introduced client-side search:

```
cmd/search/     → WASM bridge
builder/search/ → BM25 ranking
```

### Features

- **BM25 Ranking** - Relevance scoring
- **Snippets** - Context in results
- **Keyboard Navigation** - Power user friendly
- **Version Scoping** - Search within version

## Theme Enhancements

### Docs Theme

- Recursive sidebar navigation
- Version selector in header
- Breadcrumb navigation
- Prev/Next page links

### Blog Theme

- Chronological feed
- Tag support
- RSS generation

## Comparison

| Feature | v1.0 | v2.0 |
|---------|------|------|
| Version support | No | Yes |
| Client search | No | Yes |
| Recursive sidebar | No | Yes |
| Version selector | No | Yes |

## Migration

See the [Migration Guide](./migration-guide.md) for upgrading from v1.0.

## Version History

- [v4.0 Changelog](../changelog.md) - Latest changes
- [v3.0 What's New](../v3.0/whats-new.md) - Next version
