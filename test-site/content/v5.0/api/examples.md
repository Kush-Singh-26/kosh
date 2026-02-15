---
title: "API Examples"
description: "Code examples and snippets"
weight: 45
---

# API Examples

Practical code examples for using Kosh.

## Basic Build

```bash
# Build site with default config
kosh build

# Build with custom config
kosh build --config my-config.yaml

# Build in development mode
kosh build --dev
```

## Development Server

```bash
# Start server with live reload
kosh serve --dev

# Start on custom port
kosh serve --port 3000 --dev
```

## Front Matter Examples

### Blog Post

```yaml
---
title: "My First Post"
description: "Introduction to my blog"
date: "2026-02-13"
tags: ["tutorial", "getting-started"]
weight: 100
---

# My First Post

Content goes here...
```

### Documentation Page

```yaml
---
title: "Configuration"
description: "Configure your site"
weight: 90
---

# Configuration

Configuration options...
```

## Version Configuration

```yaml
# kosh.yaml
versions:
  - name: "v4.0"
    path: ""
    isLatest: true
  - name: "v3.0"
    path: "v3.0"
  - name: "v2.0"
    path: "v2.0"
  - name: "v1.0"
    path: "v1.0"
```

## Cross-Version Links

### From Latest to Version

```markdown
[v2.0 Getting Started](./v2.0/getting-started.md)
[v1.0 Installation](./v1.0/installation.md)
```

### From Version to Latest

```markdown
[Latest Documentation](../index.md)
[Getting Started (Latest)](../getting-started.md)
```

## GitHub Actions CI/CD

```yaml
name: Build and Deploy

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Install Kosh
        run: go install github.com/kosh/kosh@latest
      
      - name: Build
        run: kosh build
      
      - name: Deploy
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./public
```

## Related

- [API Reference](./reference.md) - Full API docs
- [SDK](./sdk.md) - SDK documentation
- [Tutorial](../tutorial.md) - Step-by-step guide
