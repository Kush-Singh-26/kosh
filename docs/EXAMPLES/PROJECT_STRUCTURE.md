# Typical Kosh Project Structure

This guide visualizes how a Kosh project is organized on disk.

## Directory Tree

```text
my-kosh-site/
├── .kosh-cache/           # Build artifacts and persistent cache (BoltDB)
├── content/               # Your Markdown content
│   ├── index.md           # The root index page (home)
│   ├── about.md           # A standalone page (/about/)
│   └── blog/              # A content section (/blog/)
│       ├── post-1.md
│       └── post-2.md
├── static/                # Global assets (built into public/static/)
│   ├── css/
│   ├── js/
│   └── images/
├── themes/                # Theme definitions
│   └── my-theme/
│       ├── theme.yaml     # Theme metadata
│       ├── static/        # Theme-specific assets
│       └── templates/     # HTML templates (Main, Index, Page, etc.)
├── layouts/               # (Optional) User-level template overrides
├── data/                  # (Optional) YAML/JSON data files for templates
├── kosh.yaml              # The main configuration file
└── public/                # THE OUTPUT (Deploy this folder)
```

## Key Files Explained

### 1. `kosh.yaml`
The brain of your site. It defines the title, baseURL, menus, and which features (like Search or PWA) are enabled.

### 2. `content/`
This is where you spend most of your time. Kosh mirrors the structure of this folder in the output.
- `content/blog/hello-world.md` becomes `public/blog/hello-world/index.html`.

### 3. `static/`
Anything here is copied directly to the output. Images placed here can be referenced in your Markdown as `/static/images/hero.jpg`.

### 4. `themes/`
Themes are modular. You can swap themes by changing the `theme` field in your `kosh.yaml`. A theme must contain a `templates/` folder with at least `layout.html` and `index.html`.

### 5. `data/`
Structured data files (YAML/JSON). Any file placed here is available to all templates via `.SiteData`.
- **Pages**: Files with `slug` and `layout` keys (e.g., `data/events.yaml`) automatically generate standalone pages.

### 6. `public/`
This directory is generated when you run `kosh build`. It contains the final, minified, static HTML/CSS/JS files ready for deployment to any host (GitHub Pages, Netlify, Vercel, S3).
