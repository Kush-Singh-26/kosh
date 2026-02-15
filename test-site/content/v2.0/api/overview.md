---
title: "API Overview v2.0"
description: "API overview for v2.0"
weight: 50
---

# API Overview v2.0

API documentation for v2.0.

> **Note:** This is v2.0 documentation. For the latest API, see [API Reference](../../api/reference.md).

## New in v2.0

### Version API

Access version information in templates:

```html
{{ range .Versions }}
<option value="{{ .URL }}" {{ if .IsCurrent }}selected{{ end }}>
  {{ .Name }}
</option>
{{ end }}
```

### Search API

Client-side search with WASM:

```javascript
// Initialize search
await initSearch('/search.bin');

// Perform search
const results = search('query');
```

## Template Variables

### Version Info

| Field | Type | Description |
|-------|------|-------------|
| `Name` | string | Display name |
| `URL` | string | Version URL |
| `IsLatest` | bool | Is latest version |
| `IsCurrent` | bool | Is current page version |

### Page Data

```go
type PageData struct {
    Title       string
    Content     string
    Version     string
    Versions    []VersionInfo
    IsOutdated  bool
    SiteTree    []TreeNode
}
```

## v2.0 API Pages

- [Changelog](./changelog.md) - API changes

## Cross-Version API

- [v4.0 API Reference](../../api/reference.md) - Latest API
- [v1.0 Tutorial](../../v1.0/guides/tutorial.md) - Legacy guide

## Related

- [v2.0 Documentation](../index.md)
- [Changelog](./changelog.md)
