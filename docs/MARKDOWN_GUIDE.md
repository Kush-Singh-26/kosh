# Kosh Markdown Guide

Kosh extends standard Markdown with powerful features for technical writing and semantic layout.

## 1. Attributes
Kosh supports standard attribute syntax (`{#id .class key=val}`) for all Markdown elements.

### Headings
```markdown
## Custom Heading {#my-custom-id .special-heading}
```

### Links & Images
```markdown
[Link with ID](https://example.com){#external-link .btn}
![Alt text](/static/img.png){.rounded-image}
```

---

## 2. Code Block Enhancements
Kosh provides a custom header bar for fenced code blocks with support for titles and language label toggling.

### Adding a Title
Use the `title` attribute to add a filename or description to the code block header.
```markdown
```go {title="main.go"}
package main
```
```

### Hiding the Language Label
Use the `.nolang` or `.hide-lang` class to suppress the language label in the header.
```markdown
```go {.nolang}
// Language label will be hidden
```
```

---

## 3. Automatic Transformations

### Link Normalization
Kosh automatically processes all links:
- **External Links**: Automatically receive `target="_blank"` and `rel="noopener noreferrer"`.
- **Markdown Links**: Internal `.md` links are converted to `.html` and lowercased automatically.
- **Path Cleaning**: Prepends `baseURL` and handles relative path normalization.

### Smart Figures
If an image is the sole content of a paragraph, Kosh automatically wraps it in a `<figure>` element.
- The `alt` text is used as the `<figcaption>`.
- **Note**: This is skipped if the alt text is empty or literally "image".

### Lazy Loading
All images (except the site logo) automatically receive `loading="lazy"` and `decoding="async"` attributes to improve page performance.

---

## 4. Shortcodes
Shortcodes allow you to insert complex components. Kosh supports two syntaxes:
- `{{< name ... >}}`: Raw HTML output.
- `{{% name ... %}}`: Inner content is processed as Markdown.

*See [FEATURES.md](./FEATURES.md) for a full list of built-in shortcodes.*
