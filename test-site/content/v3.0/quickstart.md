---
title: "Quickstart v3.0"
description: "Quickstart guide for v3.0"
weight: 90
---

# Quickstart v3.0

Get up and running with v3.0 quickly.

> **Note:** This is v3.0 documentation. For the latest version, see [Getting Started](../getting-started.md).

## Installation

```bash
go install github.com/kosh/kosh@v3.0
```

## Create a Site

```bash
mkdir my-site
cd my-site
```

Create `kosh.yaml`:

```yaml
baseURL: "http://localhost:2604"
title: "My Site"
theme: "docs"
```

## Start Development

```bash
kosh serve --dev
```

## v3.0 Specific Features

### Service Layer

v3.0 introduced a service layer pattern:

```go
builder := run.NewBuilder(cfg)
```

### Context Support

Graceful shutdown with context:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
builder.BuildWithContext(ctx)
```

## Next Steps

- [What's New](./whats-new.md) - New features
- [Migration Guide](./advanced/migration.md) - Migrate from v2.0

## Other Versions

- [v4.0 (Latest)](../getting-started.md) - Current version
- [v2.0](../v2.0/getting-started.md) - Previous version
- [v1.0](../v1.0/getting-started.md) - Legacy version
