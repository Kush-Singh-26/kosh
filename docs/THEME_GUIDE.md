# Kosh Theme Development Guide

Kosh uses a **Base Chrome + Theme Slot** architecture. This means the engine provides the structural shell (the "Chrome"), and themes provide the content and styling logic through a modular slot system.

## Theme Directory Structure

All themes must reside in the `themes/` directory and follow this structure:

```text
themes/[theme-name]/
├── static/
│   ├── css/
│   │   └── layout.css      <-- Required: Main Design System
│   ├── js/
│   │   └── main.js         <-- Optional: Theme-specific logic
│   └── images/
│       └── logo.png        <-- Recommended: High-res branding
├── templates/
│   ├── layout.html         <-- Required: Slot for single posts/pages
│   ├── index.html          <-- Required: Slot for home page
│   └── 404.html            <-- Required: Slot for error page
└── theme.yaml              <-- Required: Theme metadata
```

## The Base Chrome Shell

The Base Chrome handles the heavy lifting involved in modern static sites. By inheriting from it, your theme automatically gets:

- **SEO**: Meta tags, OpenGraph, Twitter Cards, and JSON-LD structured data.
- **PWA**: Automated Service Worker registration and manifest handling.
- **Dev Workflow**: Live-reloading via Server-Sent Events (SSE).
- **Navigation**: Desktop and mobile menus, progress bars, and "back to top" logic.
- **Search**: Built-in Go+WASM search UI and index handling.
- **Graph**: Native knowledge graph UI integration.

## Procedure for Creating a New Theme

### 1. Define the Design System (`layout.css`)

Kosh is hardcoded to link `/static/css/layout.css` as the primary stylesheet. Use this file to define your global aesthetic.

> [!TIP]
> **Dynamic Dark Mode**: The Base Chrome automatically handles theme switching. Target the `[data-theme="light"]` attribute on the `<html>` element to define mode-specific colors.

```css
:root {
  --accent-primary: #111113;
  --bg-color: #ffffff;
  --text-primary: #1a1a1a;
}

[data-theme="light"] {
  --accent-primary: #ffffff;
  --bg-color: #f8f9fa;
  --text-primary: #333333;
}
```

### 2. Implement Template Slots

Unlike traditional SSGs, you do not write complete HTML documents. Instead, you define two primary slots: `content` and `head-extra`.

#### `layout.html` (Single Post/Page)
This template defines how a single article should look inside the shell.

```html
{{ define "content" }}
<article class="post-container">
    <header class="post-header">
        <h1>{{ .Title }}</h1>
        <div class="post-meta">
            <span>{{ .DateObj.Format "January 2, 2006" }}</span>
            <span class="dot">&bull;</span>
            <span>{{ .ReadingTime }} min read</span>
        </div>
    </header>
    <div class="post-content">
        {{ .Content }}
    </div>
</article>
{{ end }}
```

#### `index.html` (Home Page)
This template defines the landing page experience.

```html
{{ define "content" }}
<section class="hero">
    <h1>Welcome to {{ .Config.GetSiteTitle }}</h1>
    <p>{{ .Description }}</p>
</section>

<div class="post-list">
    {{ range .Posts }}
    {{ template "partials/post-card.html" . }}
    {{ end }}
</div>
{{ end }}
```

### 3. Reusable Components (Partials)

Kosh supports **Partials** — small, reusable template snippets stored in `templates/partials/`. These are pre-loaded and available to every page template using Go's native `template` action.

#### Example: `templates/partials/post-card.html`
```html
<div class="post-card">
    <h3>{{ .Title }}</h3>
    <p>{{ .Description }}</p>
    <a href="{{ relativize $.BaseURL $.RelativePrefix .Link }}">Read more</a>
</div>
```

#### Calling a Partial
To use a partial, call it by its path relative to the `templates` directory, which always starts with `partials/`.

```html
<!-- In any page template -->
{{ range .Posts }}
    {{ template "partials/post-card.html" . }}
{{ end }}
```

> [!NOTE]
> **Context Passing**: The second argument (the `.` in the example above) is the data context passed to the partial. Inside the partial, `.` will refer precisely to what you passed (in this case, a single Post object).

### 4. Data Files

Kosh allows you to store structured data in a `data/` directory at the project root. This data is automatically loaded and made available to all templates via `.SiteData`.

#### Example
1. Create `data/projects.yaml`:
```yaml
- name: "Kosh SSG"
  stars: 420
- name: "Go"
  stars: 120000
```

2. Use it in any template:
```html
<ul>
{{ range .SiteData.projects }}
    <li>{{ .name }} ({{ .stars }} stars)</li>
{{ end }}
</ul>
```

Supported formats: `.yaml`, `.yml`, `.json`. The filename (minus extension) becomes the key in `.SiteData`.

### 5. Shortcodes

Shortcodes are reusable snippets used inside your Markdown files. They follow the Hugo-style `{{< name args >}}` syntax.

#### Built-in Shortcodes

- `{{< youtube id="VIDEO_ID" >}}`: Embeds a responsive YouTube video.
- `{{< figure src="/img.jpg" caption="Description" size="400px" >}}`: Images with captions and custom sizing.
- `{{< callout type="tip" title="Pro Tip" >}} Content... {{< /callout >}}`: Admonition boxes. Types: `note`, `tip`, `warning`, `caution`.
- `{{< details summary="Click to expand" >}} Hidden content... {{< /details >}}`: Collapsible sections.

#### Custom Shortcodes

To create a custom shortcode, add an HTML file to `templates/shortcodes/name.html`.

**Example: `templates/shortcodes/alert.html`**
```html
<div class="alert alert-{{ .Args.type }}">
    {{ .Inner }}
</div>
```

**Usage:**
```markdown
{{< alert type="danger" >}}
This is a custom alert!
{{< /alert >}}
```

Available variables in shortcode templates:
- `.Args`: A map of string arguments.
- `.Inner`: The nested content (for block shortcodes).
- `.Name`: The name of the shortcode.

### 6. Adding Extra Assets (`head-extra`)

If your theme requires third-party fonts (like Google Fonts) or additional stylesheets, use the `head-extra` block. This block is injected at the end of the `<head>` tag in the shell.

```html
{{ define "head-extra" }}
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;700&display=swap" rel="stylesheet">
<link rel="stylesheet" href="{{ relativize .BaseURL .RelativePrefix "/static/css/special.css" }}">
{{ end }}
```

### 4. Customizing the Chrome

To style the header, footer, or search modal provided by the engine, target these specific class names in your CSS:

| Part | Description | Class / ID |
| :--- | :--- | :--- |
| **Header** | The sticky navigation bar | `header`, `.site-title`, `.site-logo` |
| **Navigation** | Links and mobile menu | `.desktop-nav`, `.mobile-menu`, `.menu-toggle` |
| **Search** | The modal search interface | `#search-modal`, `.search-results` |
| **Graph** | Knowledge graph UI | `.graph-ui`, `.graph-panel` |
| **Footer** | The site colophon | `footer.colophon`, `.colophon-grid` |

## Validation and Quality

Kosh employs a mix of automated and manual checks to ensure theme quality.

### Automated Checks
The engine automatically verifies several requirements during the build process:

- **Directory Structure**: Kosh validates the presence of `layout.html`, `index.html`, and `theme.yaml`. The build will fail if these are missing.
- **A11y Linting**: During HTML rendering, the engine scans for `<img>` tags. It will issue a `slog.Warn` if it finds an image without an `alt` attribute (or an empty one that isn't clearly decorative).
- **Template Syntax**: Go template syntax is validated at parse time.
- **Asset Resolution**: The `relativize` function ensures all internal links are correct based on the deployment subpath.

### Manual Requirements
Some quality standards cannot be easily automated and must be verified manually by the theme developer:

- **Color Contrast**: Ensure text-to-background contrast ratios meet WCAG AA standards (4.5:1 for normal text).
- **CSS Variable Consistency**: Maintain a consistent naming convention for CSS tokens (e.g., `--color-primary-500`, `--spacing-md`).
- **Responsive Fluidity**: Verify that layouts do not break at arbitrary viewport widths between standard breakpoints.
- **Semantic HTML**: Use appropriate landmark elements (`<nav>`, `<main>`, `<aside>`, `<footer>`) to ensure a screen-reader-friendly document outline.

## Best Practices

1. **Path Safety**: Always use the `relativize` template function for internal links and assets.
   - `{{ relativize .BaseURL .RelativePrefix "/path/to/asset" }}`
2. **Config Access**: Use the strictly-typed `{{ .Config.Get* }}` methods to access site settings.
   - Example: `{{ .Config.GetSiteTitle }}`, `{{ .Config.GetLogo }}`, `{{ .Config.GetNavbar }}`.
3. **Typography**: Define your base typography on the `html` or `body` element.
4. **Performance**: Avoid large unoptimized scripts. The Base Chrome already includes the necessary logic for search, PWA, and graph interactions.

---

For real-world implementation details, see the reference theme at `themes/blog/`.
