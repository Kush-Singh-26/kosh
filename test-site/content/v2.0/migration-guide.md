---
title: "Migration Guide v2.0"
description: "Migrate from v1.0 to v2.0"
weight: 70
---

# Migration Guide v2.0

Guide for migrating from v1.0 to v2.0.

> **Note:** This is v2.0 documentation. For the latest version, see [v4.0 Documentation](../v4.0/index.md).

## Overview

v2.0 introduces versioning and search features. Most v1.0 configurations will work without changes.

## Configuration Changes

### New: Versions Section

Add version configuration:

```yaml
# kosh.yaml
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
```

### Content Structure

Organize versioned content:

```
content/
├── getting-started.md    # Latest (v2.0)
├── features.md
└── v1.0/
    └── getting-started.md  # v1.0 specific
```

## Breaking Changes

### Theme Update

If using the docs theme, update templates:

```html
<!-- Add version selector -->
{{ if .Versions }}
<select id="version-selector">
  {{ range .Versions }}
  <option value="{{ .URL }}">{{ .Name }}</option>
  {{ end }}
</select>
{{ end }}
```

### Search Integration

Add search scripts:

```html
<script src="/static/js/wasm_exec.js"></script>
<script src="/static/js/search.js"></script>
```

## New Features to Adopt

### Version Support

Create versioned content:

```markdown
---
title: "Getting Started v1.0"
---

This is v1.0 specific content.
```

### Client Search

Enable search:

```yaml
features:
  generators:
    search: true
```

## Testing Checklist

- [ ] Version selector appears
- [ ] Version switching works
- [ ] Search returns results
- [ ] Sidebar shows correct tree
- [ ] Old URLs still work

## Need Help?

- [v2.0 Documentation](./index.md)
- [What's New](./new-in-v2.md)
- [v4.0 (Latest)](../v4.0/index.md)
