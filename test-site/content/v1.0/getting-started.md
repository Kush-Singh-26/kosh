---
title: "Getting Started v1.0"
description: "Getting started with v1.0"
weight: 90
---

# Getting Started v1.0

This is the **v1.0** Getting Started guide.

> **Warning:** This version is no longer maintained. [View Latest Version](../getting-started.md).

## Quick Start

### Install v1.0

```bash
go install github.com/kosh/kosh@v1.0
```

### Create Site

```bash
mkdir my-site
cd my-site
```

Create `kosh.yaml`:

```yaml
baseURL: "http://localhost:2604"
title: "My Site"
theme: "docs"
```

### Start Server

```bash
kosh serve
```

## v1.0 Limitations

v1.0 has limited features compared to newer versions:

| Feature | v1.0 | v4.0 |
|---------|------|------|
| Version support | No | Yes |
| Client search | Basic | WASM |
| Service layer | No | Yes |
| Performance | Standard | Optimized |

## Upgrade Recommendation

We strongly recommend upgrading to v4.0:

- Better performance
- More features
- Active maintenance

See the [Migration Guide](../v2.0/migration-guide.md) to start upgrading.

## v1.0 Pages

- [Installation](./installation.md) - Install v1.0
- [Configuration](./configuration.md) - Configure v1.0
- [Tutorial](./guides/tutorial.md) - Learn v1.0
- [Best Practices](./guides/best-practices.md) - Recommendations
