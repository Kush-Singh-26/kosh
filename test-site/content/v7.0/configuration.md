---
title: "Configuration"
description: "Configure your Kosh site"
weight: 90
---

# Configuration

Kosh uses a `kosh.yaml` file in your project root for configuration.

## Basic Configuration

```yaml
baseURL: "https://example.com"
title: "My Documentation"
languageCode: "en-us"
theme: "docs"

author:
  name: "Your Name"
  email: "you@example.com"
```

## Site Settings

| Setting | Type | Description |
|---------|------|-------------|
| `baseURL` | string | The base URL of your site |
| `title` | string | Site title displayed in header |
| `logo` | string | Path to logo image |
| `theme` | string | Theme name (`blog` or `docs`) |
| `languageCode` | string | Language code for the site |

## Version Configuration

For documentation sites with multiple versions:

```yaml
versions:
  - name: "v4.0"
    path: ""
    isLatest: true
  - name: "v3.0"
    path: "v3.0"
  - name: "v2.0"
    path: "v2.0"
  - name: "v1.0"
    path: "v1.0"
```

## Features

Enable or disable specific features:

```yaml
features:
  rawMarkdown: true
  generators:
    sitemap: true
    rss: false
    search: true
    pwa: true
```

## Advanced Options

For more configuration options, see:

- [Advanced Configuration](./advanced/configuration.md) - Performance and caching
- [Deployment](./advanced/deployment.md) - Deploy your site

## Version-Specific Config

Different versions may have different configuration options:

- [v3.0 Configuration](./v3.0/index.md)
- [v2.0 Configuration](./v2.0/advanced/configuration.md)
- [v1.0 Configuration](./v1.0/configuration.md)
