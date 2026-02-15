---
title: "Performance Tuning"
description: "Optimize Kosh build performance"
weight: 60
---

# Performance Tuning

Optimize your Kosh build for maximum performance.

## Build Metrics

View build statistics:

```bash
kosh build
# Output: Built 150 posts in 2.3s (cache: 145/5 hits, 97%)
```

## Caching

### Incremental Builds

Kosh caches:
- Rendered HTML
- Asset hashes
- Post metadata

Only changed content is rebuilt.

### Cache Location

```yaml
cacheDir: ".kosh-cache"
```

### Clearing Cache

```bash
kosh clean --cache
```

## Parallelization

### Worker Pools

Kosh uses parallel processing:

- **Post processing** - 12 workers
- **Asset syncing** - 4 workers
- **Image processing** - 4 workers

### Memory Pools

Object pooling reduces allocations:

```go
buf := utils.SharedBufferPool.Get()
defer utils.SharedBufferPool.Put(buf)
```

## Tips for Large Sites

### 1. Use Incremental Builds

```bash
kosh serve --dev  # Watch mode with fast rebuilds
```

### 2. Optimize Images

Place large images in `static/` to skip processing:

```yaml
# Static assets are copied as-is
staticDir: "static"
```

### 3. Reduce Plugins

Disable unused features:

```yaml
features:
  generators:
    sitemap: true
    rss: false
    pwa: false
    search: false
```

## Benchmarking

Run the benchmark suite:

```bash
go test -bench=. -benchmem ./builder/benchmarks/
```

## Related

- [Advanced Configuration](./configuration.md) - Configuration options
- [Deployment](./deployment.md) - Deploy your site
- [Changelog](../changelog.md) - Performance improvements
