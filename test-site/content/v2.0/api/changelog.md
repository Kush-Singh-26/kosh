---
title: "API Changelog v2.0"
description: "API changes in v2.0"
weight: 45
---

# API Changelog v2.0

API changes introduced in v2.0.

> **Note:** This is v2.0 documentation. For the latest changes, see [Changelog](../../changelog.md).

## Added

### Version API

New template variables for version support:

```go
type VersionInfo struct {
    Name      string
    URL       string
    IsLatest  bool
    IsCurrent bool
}
```

### Search API

WASM-based client search:

```javascript
// wasm_engine.js
function search(query) {
    return wasmSearch(query);
}
```

### Template Functions

```html
{{ range .Versions }}
  {{ .Name }} - {{ .URL }}
{{ end }}

{{ if .IsOutdated }}
  <div class="version-banner">...</div>
{{ end }}
```

## Changed

### PageData Structure

Added fields:

```go
type PageData struct {
    // Existing
    Title   string
    Content string
    
    // New in v2.0
    Version    string
    Versions   []VersionInfo
    IsOutdated bool
}
```

### Site Tree

Recursive tree structure:

```go
type TreeNode struct {
    Title    string
    Link     string
    Children []TreeNode
    Active   bool
}
```

## Removed

None. v2.0 maintains backward compatibility.

## Migration

See [Migration Guide](../migration-guide.md) for details.

## Related

- [API Overview](./overview.md)
- [v4.0 Changelog](../../changelog.md)
