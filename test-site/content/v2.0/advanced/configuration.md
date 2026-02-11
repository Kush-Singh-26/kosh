---
title: "Configuration"
description: "Configuration guide - VERSION 2.0"
date: "2025-06-20"
weight: 5
---

# Configuration (v2.0)

This is the **v2.0** version of the Configuration guide.

## Configuration Options

In v2.0, we introduced several new configuration options:

### New in v2.0

```yaml
# kosh.yaml - v2.0 features
version: "2.0"
features:
  - search
  - versioning
  - breadcrumbs
  
search:
  enabled: true
  indexAllVersions: true
```

### Changed from v1.0

- Option A: Now accepts arrays instead of strings
- Option B: Default value changed from `false` to `true`
- Option C: Removed (use Option D instead)

## Breaking Changes

When migrating from v1.0 to v2.0:

1. Update your config file format
2. Replace deprecated options
3. Test in development mode first

## See Also

- [Getting Started](../getting-started.html) - Overview
- [New in v2.0](../new-in-v2.html) - v2.0 specific features
