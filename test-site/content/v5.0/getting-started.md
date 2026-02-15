---
title: "Getting Started"
description: "Get started with Kosh in 5 minutes"
weight: 100
---

# Getting Started

Welcome to Kosh! This guide will help you get up and running quickly.

## Prerequisites

- Go 1.23 or later
- A text editor
- Basic command line knowledge

## Quick Start

### 1. Install Kosh

```bash
go install github.com/kosh/kosh@latest
```

### 2. Create a New Site

```bash
mkdir my-docs
cd my-docs
```

Create `kosh.yaml`:

```yaml
baseURL: "http://localhost:2604"
title: "My Documentation"
theme: "docs"
```

### 3. Create Content

Create `content/getting-started.md`:

```markdown
---
title: "Getting Started"
---

Welcome to my documentation!
```

### 4. Start Development Server

```bash
kosh serve --dev
```

Open `http://localhost:2604` in your browser.

## Next Steps

- [Installation Guide](./installation.md) - Detailed installation options
- [Configuration](./configuration.md) - Configure your site
- [Tutorial](./tutorial.md) - Step-by-step guide

## Features Overview

- **Fast builds** - Incremental compilation with caching
- **Multiple themes** - Blog and documentation themes
- **Version support** - Multiple documentation versions
- **Search** - WASM-powered client-side search

See [Features](./features.md) for the complete list.

## Version Navigation

This is the **latest version (v4.0)**. Previous versions:

- [v3.0 Documentation](./v3.0/index.md) - Previous stable
- [v2.0 Documentation](./v2.0/index.md) - Older version
- [v1.0 Documentation](./v1.0/index.md) - Legacy version
