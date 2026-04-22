# Frontmatter Reference

Kosh uses YAML frontmatter to manage metadata for each Markdown file. This metadata influences build behavior, SEO, sorting, and how your content is rendered in templates.

## Standard Fields

These fields are natively supported by the Kosh engine and have specific internal behaviors.

| Field | Type | Description |
| :--- | :--- | :--- |
| `title` | `string` | The display title of the page. If omitted, Kosh fallbacks to the filename (slugified). |
| `date` | `string` | Publication date in `YYYY-MM-DD` format. Used for sorting and displayed in social cards/badges. |
| `description` | `string` | A concise summary used for search engine snippets, RSS feeds, and social media cards. |
| `draft` | `bool` | If `true`, the page is excluded from production builds. It will only be visible when running `kosh serve --dev`. |
| `pinned` | `bool` | High-priority flag. Pinned items are often featured at the top of lists or in a special "Pinned" section in templates. |
| `weight` | `int` | Manual sorting order. Items with lower weight are shown first. Useful for ordering documentation or non-chronological lists. |
| `layout` | `string` | Specifies which theme template to use (e.g., `post`, `home`, `index`). Overrides the default section layout. |
| `seo_title` | `string` | Overrides the browser tab `<title>` and the primary heading in social media cards. Defaults to `title`. |
| `meta_title` | `string` | An alias for `seo_title`. Both can be used interchangeably. |

## Taxonomies

Taxonomies are defined in your `kosh.yaml` and allow you to group content. Standard examples include:

| Field | Type | Description |
| :--- | :--- | :--- |
| `tags` | `slice` | A list of strings used to categorize content (e.g., `["go", "ssg"]`). |
| `categories`| `slice` | A list of strings for broader classification. |
| `series` | `slice` | Group related posts. Posts in a series are automatically sorted chronologically (ascending). |
| `events` | `slice` | Group events. Automatically sorted chronologically for timeline views. |

> [!NOTE]
> You can define custom taxonomy keys in `kosh.yaml`, and they will be available as slices in frontmatter.

## Section Metadata (`_index.md`)

When placed in an `_index.md` file, the `cascade` field allows you to pass metadata down to all files within that directory and its subdirectories.

```yaml
---
title: "Tutorials"
cascade:
  layout: "tutorial"
  author: "Kosh Team"
---
```

## Custom Fields

Any field not listed above is treated as a **Custom Field**. These are decoded and made available to your HTML templates via the `.Meta` object.

```yaml
---
title: "Custom Example"
author: "Kush Singh"
hero_image: "/static/images/hero.webp"
show_sidebar: true
---
```

In your theme templates, you can access these using:
`{{ .Meta.author }}` or `{{ if .Meta.show_sidebar }}...{{ end }}`.

## Technical Details

- **Case Sensitivity:** Core fields like `layout` and `title` are handled case-insensitively.
- **Normalization:** Date strings are automatically parsed into Go `time.Time` objects (`DateObj`) for precise sorting and formatting in templates.
- **Hashing:** Kosh computes a unique hash for the frontmatter to detect changes and determine if a page needs a partial or full rebuild.
