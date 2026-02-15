---
title: "Advanced Setup v2.0"
description: "Advanced setup for v2.0"
weight: 70
---

# Advanced Setup v2.0

Advanced configuration options for v2.0.

> **Note:** This is v2.0 documentation. For the latest version, see [Advanced Configuration](../advanced/configuration.md).

## Configuration

### Version Settings

```yaml
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
```

### Search Settings

```yaml
features:
  generators:
    search: true
```

## Directory Structure

```
content/
├── getting-started.md      # Latest
├── features.md
├── v1.0/
│   ├── index.md
│   └── getting-started.md  # v1.0 specific
└── v2.0/                   # Only if v2.0 != latest
```

## Theme Configuration

### Docs Theme

```yaml
theme: "docs"
```

Features:
- Version selector
- Recursive sidebar
- Search modal
- Breadcrumbs

### Blog Theme

```yaml
theme: "blog"
```

Features:
- Chronological feed
- Tag pages
- RSS feed

## v2.0 Advanced Pages

- [Configuration](./configuration.md) - v2.0 specific config
- [Migration Details](./migration.md) - Migration details

## Other Versions

- [v4.0 Advanced](../../advanced/configuration.md) - Latest config
- [v1.0 Configuration](../../v1.0/configuration.md) - Legacy config
