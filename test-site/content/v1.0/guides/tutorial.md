---
title: "Tutorial v1.0"
description: "Step-by-step tutorial for v1.0"
weight: 70
---

# Tutorial v1.0

Step-by-step tutorial for Kosh v1.0.

> **Warning:** This is v1.0 documentation. For the latest tutorial, see [Tutorial](../tutorial.md).

## Prerequisites

- [Installed Kosh v1.0](./installation.md)
- Basic command line knowledge

## Step 1: Create Project

```bash
mkdir my-docs
cd my-docs
```

## Step 2: Create Configuration

Create `kosh.yaml`:

```yaml
baseURL: "http://localhost:2604"
title: "My Documentation"
theme: "docs"

author:
  name: "Your Name"
```

## Step 3: Create Content

Create `content/getting-started.md`:

```markdown
---
title: "Getting Started"
---

# Getting Started

Welcome to my documentation!
```

## Step 4: Start Server

```bash
kosh serve
```

Open `http://localhost:2604` in your browser.

## Step 5: Add More Pages

```
content/
├── getting-started.md
├── features.md
└── guides/
    └── tutorial.md
```

## Step 6: Build

```bash
kosh build
```

Output in `public/` directory.

## v1.0 Limitations

- No version support
- Basic search only
- Manual asset management

## Next Steps

- [Best Practices](./best-practices.md) - Recommendations
- [Configuration](../configuration.md) - More options

## Upgrade

Consider [upgrading to v4.0](../tutorial.md) for more features.
