# Kosh Features & Markdown Extensions

Kosh extends standard Markdown with powerful features for scientific writing, diagrams, and multimedia.

## Social Cards (OG Images)
Kosh automatically generates high-quality Open Graph images for every page.
- **Auto-generated**: Uses the page title, description, and site branding.
- **Customizable**: Configure gradients and colors in `kosh.yaml`.
- **Manual Override**: Set `image: "path/to/custom-card.webp"` in any page's frontmatter to use a specific image.

## Search
Kosh includes a high-performance, client-side search engine.
- **Go + WASM**: The search logic is written in Go and compiled to WebAssembly for near-native performance.
- **Full Text**: Searches through titles, descriptions, and page content.
- **Zero-Config**: Just enable `isEnabled` in the search section of `kosh.yaml`.
- **Federated Search**: Aggregates search results from multiple configured `endpoints` (e.g., across a network of Kosh sites).
- **Tuneable Ranking**: Customize BM25 parameters and field-specific boosts (Title, Tags, Description) for your specific content.

## Shortcodes
Shortcodes allow you to insert complex HTML components into your Markdown with a simple syntax: `{{< name attr="val" >}}`.

### 1. YouTube
Embed videos with a responsive container.
```markdown
{{< youtube id="dQw4w9WgXcQ" >}}
```

### 2. Figure
A semantic `<figure>` element with an image and optional caption.
```markdown
{{< figure src="/static/images/sunset.jpg" alt="A beautiful sunset" caption="Sunset over the mountains" size="800px" >}}
```

### 3. Callout
Highlight important information with styled boxes.
```markdown
{{< callout type="tip" title="Pro Tip" >}}
Kosh builds faster when you use the `--dev` flag during development!
{{< /callout >}}
```
Supported types: `note`, `tip`, `warning`, `caution`.

### 4. Details
A collapsible accordion for extra information.
```markdown
{{< details summary="Click to see more" >}}
Hidden treasure is here!
{{< /details >}}
```

## Scientific & Technical Features

### LaTeX Math (SSR)
Kosh renders Math using server-side rendering, so it works even if the user has JavaScript disabled. Use standard LaTeX syntax:
- **Inline**: `$E = mc^2$`
- **Block**:
  ```markdown
  $$
  \int_{a}^{b} f(x) dx
  $$
  ```

### D2 Diagrams (SSR)
Render beautiful diagrams using the [D2 declarative language](https://d2lang.com/).
- Just use a fenced code block with the `d2` language.
- Kosh generates optimized SVGs during the build process.

```d2
direction: right
Browser -> Server: HTTP Request
Server -> Database: Query
Database -> Server: Result
Server -> Browser: HTTP Response
```

## Knowledge Graph
Kosh automatically generates an interactive knowledge graph of your site.
- **Interactive View**: Accessible at `/graph.html`.
- **Data Export**: Raw JSON available at `/graph.json`.
- **Connections**: Maps links between pages and shared taxonomy terms.

## Advanced Markdown
Kosh supports powerful Markdown extensions for technical writing:
- **Attributes**: Add `{#id .class}` to any element.
- **Enhanced Code Blocks**: Support for `title` and `.nolang` attributes.
- **Smart Figures**: Automatic wrapping of images in figures with captions.

*See the [Markdown Guide](./MARKDOWN_GUIDE.md) and [Advanced Features](./ADVANCED_FEATURES.md) for more.*

## Automated Features

### 1. Table of Contents (TOC)
Kosh automatically generates a Table of Contents for every page.
- **Slugification**: Headers are automatically given IDs (e.g., `## My Header` becomes `id="my-header"`).
- **Depth**: Includes headers from Level 2 to Level 6.

### 2. Image Captions
Images alone on a line are automatically wrapped in a `<figure>` with their `alt` text used as a `<figcaption>`.

### 3. WebP Conversion
All `.png`, `.jpg`, and `.jpeg` images in your `static/` folder are automatically converted to optimized `.webp` format during the build process, reducing page load times.

### 4. Accessibility (A11y) Linting
Kosh performs basic accessibility checks during the build:
- Warns if images are missing `alt` text.
- Warns if links are missing descriptive labels.

## System & Reliability

### 1. Kosh Doctor
Kosh includes a built-in diagnostic tool to verify site health.
- **Diagnostics**: Runs a full build pass to check for broken links, missing assets, and performance bottlenecks.
- **Health Score**: Provides a clear health score (0-100) based on build warnings, errors, and timing metrics.
- **JSON Export**: Support for CI/CD integrations via `--json` output.

### 2. Data-Driven Templates
Automatically generate pages from structured data in your `data/` directory.
- **Automated Routing**: If a YAML/JSON file in `data/` contains a `slug` and `layout`, Kosh will automatically render it as a standalone page.
- **Global Context**: Data files are accessible to all templates via the `Data` object, allowing for powerful site-wide data management.

### 3. Kosh HUD (Dev Dashboard)
A built-in development dashboard available at `/_kosh` during `kosh serve --dev`.
- **Real-time Health**: Live statistics on cache size, image conversion, and build timings.
- **Diagnostics**: Visibility into search index sync status, A11y warnings, and error logs.
- **Micro-Frontend**: Powered by a lightweight API at `/api/health`.
