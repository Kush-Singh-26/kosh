---
title: "Best Practices v1.0"
description: "Best practices for v1.0"
weight: 60
---

# Best Practices v1.0

Recommended practices for Kosh v1.0.

> **Warning:** This is v1.0 documentation. For the latest practices, see the current documentation.

## Content Organization

### Directory Structure

```
content/
├── getting-started.md    # Entry point
├── features.md           # Feature overview
├── guides/               # Tutorials
│   ├── tutorial.md
│   └── advanced.md
└── api/                  # Reference
    └── reference.md
```

### Front Matter

Always include front matter:

```yaml
---
title: "Page Title"
description: "Page description"
weight: 100
---
```

## Configuration

### Use Environment Variables

```bash
export KOSH_BASE_URL="https://example.com"
kosh build
```

### Keep It Simple

v1.0 works best with minimal configuration:

```yaml
baseURL: "https://example.com"
title: "My Site"
theme: "docs"
```

## Performance

### Enable Caching

Caching is automatic in v1.0. The `.kosh-cache/` directory stores:
- Rendered HTML
- Asset hashes
- Post metadata

### Limit Content

For large sites, consider splitting into multiple projects.

## Deployment

### Static Hosting

Deploy `public/` to any static host:
- Netlify
- Vercel
- GitHub Pages

### CI/CD

```yaml
# .github/workflows/deploy.yml
- run: kosh build
- uses: peaceiris/actions-gh-pages@v3
```

## Known Limitations

v1.0 limitations to work around:

| Limitation | Workaround |
|------------|------------|
| No versions | Use separate sites |
| Basic search | External search service |
| Manual assets | Use build scripts |

## Upgrade Path

These limitations are resolved in newer versions:

- **v2.0**: Added version support and WASM search
- **v3.0**: Added service layer and better performance
- **v4.0**: Added BLAKE3 hashing and memory pools

See [v2.0 Migration Guide](../../v2.0/migration-guide.md) to start upgrading.
