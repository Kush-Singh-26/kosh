---
title: "v7.0 Documentation"
description: "Kosh v7.0 Documentation - Latest Version"
weight: 1
---

# Welcome to Kosh v7.0

Kosh v7.0 is the **latest version** of our high-performance Static Site Generator.

## Quick Start

Get started in minutes:

```bash
# Install Kosh
go install github.com/kosh/kosh@latest

# Create a new site
kosh init my-docs
cd my-docs

# Start development server
kosh serve --dev
```

## v7.0 Highlights

### 🚀 Performance Improvements

- **BLAKE3 Hashing** - Faster, more secure content hashing
- **Memory Pools** - Reduced GC pressure during builds
- **Parallel Processing** - Multi-threaded builds
- **Incremental Builds** - Only rebuild what changed

### 🔍 Enhanced Search

- **BM25 Scoring** - Industry-standard relevance ranking
- **Fuzzy Matching** - Typo tolerance for better UX
- **Phrase Search** - Use quotes for exact phrases
- **Msgpack Encoding** - 30% smaller index, 2.5x faster decode

### 📚 Developer Experience

- **Hot Reload** - Live preview during development
- **Social Cards** - Auto-generated Open Graph images
- **D2 Diagrams** - Server-side rendered diagrams
- **LaTeX Support** - Math equations via KaTeX

## Documentation Sections

| Section | Description |
|---------|-------------|
| [Getting Started](../getting-started.md) | Installation and basic usage |
| [Installation](../installation.md) | Detailed installation guide |
| [Configuration](../configuration.md) | Configuration options |
| [Features](../features.md) | Feature overview |
| [Tutorial](../tutorial.md) | Step-by-step tutorial |

## What's New in v7.0

### Search Engine Upgrade

The search engine has been completely overhauled:

- **Msgpack** replaces GOB for smaller, faster index encoding
- **Porter Stemmer** for better word matching ("running" → "run")
- **Fuzzy Search** handles typos automatically
- **Stop Words** filtering for cleaner results

### Build Optimizations

- Object pooling reduces memory allocations
- TTL-based template cache reduces filesystem checks
- Auto-calibrating BoltDB mmap size
- Pre-allocated slices for better performance

## Other Versions

| Version | Status |
|---------|--------|
| v7.0 | ✅ **Current (Latest)** |
| [v6.0](../v6.0/index.md) | Previous |
| [v5.0](../v5.0/index.md) | Older |
| [v4.0](../v4.0/index.md) | Legacy |
