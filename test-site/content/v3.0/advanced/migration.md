---
title: "Migration to v3.0"
description: "Migrate from v2.0 to v3.0"
weight: 70
---

# Migration to v3.0

Guide for migrating from v2.0 to v3.0.

> **Note:** This is v3.0 documentation. For the latest version, see [v4.0 Documentation](../../v4.0/index.md).

## Overview

v3.0 introduces architectural changes that may require updates to your configuration.

## Breaking Changes

### Configuration

No breaking changes to `kosh.yaml` format.

### Service Layer

If you were using internal APIs directly:

```go
// Old (v2.0)
builder := run.NewBuilder(cfg)
builder.Cache.Get(key)

// New (v3.0)
builder := run.NewBuilder(cfg)
builder.CacheService.Get(key)  // Renamed
```

## New Features to Adopt

### Structured Logging

Replace `log.Printf` with `slog`:

```go
// Old
log.Printf("Processing %d posts", count)

// New
slog.Info("Processing posts", "count", count)
```

### Context Support

Use context for cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := builder.BuildWithContext(ctx)
```

## Deprecations

| Feature | Status | Replacement |
|---------|--------|-------------|
| `log.Printf` | Deprecated | `slog` |
| Direct cache access | Changed | Use `CacheService` |

## Testing

After migration, verify:

1. Build completes without errors
2. All pages render correctly
3. Search functionality works
4. Version navigation works

## Need Help?

- [v3.0 Documentation](../index.md)
- [v4.0 (Latest)](../../v4.0/index.md)
- [v2.0 Documentation](../../v2.0/index.md)
