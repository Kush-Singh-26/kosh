---
title: "Migration Details v2.0"
description: "Detailed migration from v1.0"
weight: 60
---

# Migration Details v2.0

Detailed migration steps from v1.0 to v2.0.

> **Note:** This is v2.0 documentation. For the latest version, see [v4.0 Documentation](../../v4.0/index.md).

## Step 1: Backup

```bash
cp -r content content.backup
cp kosh.yaml kosh.yaml.backup
```

## Step 2: Update Config

Add version configuration:

```yaml
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
```

## Step 3: Restructure Content

Move v1.0 specific content:

```bash
mkdir -p content/v1.0
mv content/getting-started-v1.md content/v1.0/getting-started.md
```

## Step 4: Update Theme

If using docs theme:

1. Update templates to include version selector
2. Add search scripts
3. Update sidebar templates

## Step 5: Enable Search

```yaml
features:
  generators:
    search: true
```

## Step 6: Test

1. Build site: `kosh build`
2. Check version selector works
3. Verify search functionality
4. Test version navigation

## Common Issues

### Missing Version Pages

If a page doesn't exist in a version, users are redirected to the version home.

### Search Not Working

Ensure WASM files are generated:

```bash
kosh build  # Generates search.wasm
```

## Related

- [Migration Guide](../migration-guide.md) - Overview
- [Setup](./setup.md) - Advanced setup
