---
# Primary Metadata
title: "The Kitchen Sink"
date: "2026-04-21"
description: "A demonstration of every feature supported by Kosh SSG."
draft: false
pinned: true
weight: 1
layout: "post"

# SEO Overrides
seo_title: "Kosh Kitchen Sink - All Features Demo"
meta_title: "Kitchen Sink Meta Title"

# Taxonomies (Flat structure)
tags: ["documentation", "kitchen-sink", "features"]
categories: ["Guide"]
series: ["Getting Started"]

# Custom Fields (Accessible via .Meta in templates)
author: "Kosh Maintainer"
hero_image: "/static/logo.svg"
---

# The Kitchen Sink

This page demonstrates **every major feature** of the Kosh Markdown parser. Use it as a reference for your own content.

## 1. Shortcodes

### YouTube
{{< youtube id="dQw4w9WgXcQ" >}}

### Callouts
{{< callout type="tip" title="Did you know?" >}}
Kosh can render LaTeX and D2 diagrams on the server side!
{{< /callout >}}

{{< callout type="warning" title="Caution" >}}
Mismatched shortcode tags will render as raw text.
{{< /callout >}}

### Figures
{{< figure src="/static/logo.svg" alt="Kosh Logo" caption="The official Kosh logo in SVG format." size="200px" >}}

### Details / Accordion
{{< details summary="Show technical specs" >}}
- Build Time: < 100ms
- Search: Go+WASM
- SSR: LaTeX, D2
{{< /details >}}

## 2. Scientific & Technical Tools

### LaTeX Math
Inline math: $a^2 + b^2 = c^2$.

Block math:
$$
\frac{1}{n} \sum_{i=1}^{n} x_i
$$

### D2 Diagrams
```d2
shape: sequence_diagram
Alice -> Bob: What's up?
Bob -> Alice: Not much, just using Kosh!
```

### Syntax Highlighting
```go
package main

import "fmt"

func main() {
    fmt.Println("Kosh is awesome!")
}
```

## 3. Automated Content

### Image with Auto-Caption
Adding an image on its own line automatically generates a figure.

![A beautiful sunset over the mountains](https://images.unsplash.com/photo-1464822759023-fed622ff2c3b)

### Semantic HTML
Kosh supports standard GFM (GitHub Flavored Markdown) including tables:

| Feature | Support |
| :--- | :--- |
| Markdown | Full GFM |
| LaTeX | SSR |
| D2 | SSR |
| Search | WASM |

## 4. Frontmatter Features

This page has `weight: 1` and `pinned: true`, which means it will appear at the top of list pages and is featured in the navigation. It also uses `seo_title` for search engines and social cards.

### Complete List of Supported Fields:
- `title`: The page title.
- `date`: Publication date (YYYY-MM-DD).
- `description`: Short summary.
- `draft`: Boolean (if true, only appears in dev mode).
- `pinned`: Boolean (marks as featured).
- `weight`: Sorting integer.
- `layout`: Template name (e.g., `post`, `index`).
- `seo_title`: SEO override.
- `meta_title`: SEO override.
- `[taxonomy-keys]`: Lists (e.g., `tags`, `categories`, `series`).
- `[custom-keys]`: Any other keys you want to use in your templates!
