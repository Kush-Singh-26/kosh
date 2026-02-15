---
title: "Tutorial"
description: "Step-by-step tutorial for Kosh"
weight: 85
---

# Tutorial

Follow this tutorial to create a complete documentation site with Kosh.

## Prerequisites

Make sure you have [installed Kosh](./installation.md) before starting.

## Step 1: Create a New Site

Create a new project directory:

```bash
mkdir my-docs
cd my-docs
```

Initialize with a configuration file:

```yaml
# kosh.yaml
baseURL: "http://localhost:2604"
title: "My Documentation"
theme: "docs"
```

## Step 2: Create Content

Create your first documentation page:

```bash
mkdir content
```

Create `content/getting-started.md`:

```markdown
---
title: "Getting Started"
weight: 100
---

# Getting Started

Welcome to my documentation!

## Quick Start

1. Step one
2. Step two
3. Step three
```

## Step 3: Start Development Server

Run the development server:

```bash
kosh serve --dev
```

Open `http://localhost:2604` in your browser.

## Step 4: Add More Pages

Create additional pages:

```
content/
├── getting-started.md
├── installation.md
├── configuration.md
└── advanced/
    └── performance.md
```

## Step 5: Build for Production

Build the static site:

```bash
kosh build
```

Output will be in the `public/` directory.

## Next Steps

- [Configuration](./configuration.md) - Customize your site
- [Features](./features.md) - Explore all features
- [API Reference](./api/reference.md) - API documentation
