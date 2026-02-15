---
title: "What's New in v3.0"
description: "New features in v3.0"
weight: 80
---

# What's New in v3.0

This page documents features introduced in v3.0.

> **Note:** This page only exists in v3.0. For the latest features, see [Changelog](../changelog.md).

## Architecture Changes

### Service Layer Pattern

v3.0 introduced a service layer with clear separation:

```
services/
├── interfaces.go      # Service contracts
├── post_service.go    # Markdown processing
├── cache_service.go   # Caching logic
├── asset_service.go   # Asset management
└── render_service.go  # Template rendering
```

### Dependency Injection

Services are injected via constructors:

```go
func NewBuilder(cfg *config.Config) *Builder {
    cacheService := services.NewCacheService(cfg)
    postService := services.NewPostService(cfg, cacheService)
    // ...
}
```

## New Features

### Structured Logging

Using Go's `slog` package:

```go
logger.Info("building site", "posts", count)
logger.Warn("cache miss", "key", key)
logger.Error("build failed", "error", err)
```

### Context Propagation

All long operations support context:

```go
func (s *PostService) Process(ctx context.Context, path string) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // process
    }
}
```

## Improvements

- **Better Error Handling** - Wrapped errors with context
- **Improved Cache** - More durable settings
- **Cleaner API** - Consistent method signatures

## Migration

See the [Migration Guide](./advanced/migration.md) for upgrading from v2.0.

## Version History

- [v4.0 Changelog](../changelog.md) - Latest changes
- [v2.0 Features](../v2.0/new-in-v2.md) - Previous version
