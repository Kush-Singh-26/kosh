---
title: "Advanced Configuration"
description: "Advanced configuration options"
weight: 70
---

# Advanced Configuration

This page covers advanced configuration options for power users.

## Cache Settings

Kosh uses BoltDB for caching. Configure cache behavior:

```yaml
# kosh.yaml
cacheDir: ".kosh-cache"
```

### Dev Mode

Development mode uses optimized cache settings:

- `NoGrowSync: true` - Faster writes
- `NoSync: true` - Less durability, more speed

## Build Performance

### Worker Pools

Control concurrency:

```yaml
build:
  workers: 8  # Number of parallel workers
```

### Memory Pools

Kosh uses object pooling to reduce GC pressure:

- `BufferPool` - Reusable byte buffers
- `EncodedPostPool` - Reusable post slices

## Output Options

### Minification

HTML, CSS, and JS are minified automatically in production builds.

### Image Optimization

Images are compressed with configurable quality:

```yaml
images:
  quality: 85
  format: webp
```

## Directory Structure

Override default directories:

```yaml
contentDir: "content"      # Source content
outputDir: "public"        # Build output
cacheDir: ".kosh-cache"    # Cache directory
```

## Related Pages

- [Performance Tuning](./performance.md) - Optimize build speed
- [Deployment](./deployment.md) - Deploy your site

## Version-Specific Config

Configuration options vary by version:

- [v2.0 Configuration](../v2.0/advanced/configuration.md) - v2.0 options
- [v1.0 Configuration](../v1.0/configuration.md) - Legacy config

## Cross-Reference

- [Getting Started](../getting-started.md) - Basic setup
- [Installation](../installation.md) - Install Kosh
- [API Reference](../api/reference.md) - API docs
