---
title: "Getting Started v2.0"
description: "Getting started with v2.0"
weight: 90
---

# Getting Started v2.0

This is the **v2.0** Getting Started guide.

> **Note:** This is v2.0 documentation. For the latest version, see [Getting Started](../getting-started.md).

## Quick Start

### Install v2.0

```bash
go install github.com/kosh/kosh@v2.0
```

### Create Site

```bash
mkdir my-site && cd my-site
```

Create `kosh.yaml`:

```yaml
baseURL: "http://localhost:2604"
title: "My Site"
theme: "docs"

versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
```

### Start Server

```bash
kosh serve --dev
```

## v2.0 Features

### Version System

v2.0 introduced the version system:

```yaml
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
```

### Client-Side Search

WASM-powered search:

```html
<script src="/static/js/wasm_exec.js"></script>
<script src="/static/js/search.js"></script>
```

## Next Steps

- [What's New](./new-in-v2.md) - Features in v2.0
- [Migration Guide](./migration-guide.md) - Migrate from v1.0
- [Advanced Setup](./advanced/setup.md) - Configuration

## Other Versions

- [v4.0 (Latest)](../getting-started.md) - Current version
- [v3.0](../v3.0/quickstart.md) - Previous version
- [v1.0](../v1.0/getting-started.md) - Legacy version
