# Kosh Advanced Features

This guide covers advanced organizational and data-driven features in Kosh.

## 1. Cascade Metadata
Kosh supports directory-wide metadata inheritance via the `cascade` key in an `_index.md` file.

**Example `content/blog/_index.md`**:
```yaml
---
title: "My Blog"
cascade:
  author: "Jane Doe"
  type: "post"
  layout: "post"
---
```
Every Markdown file in `content/blog/` will now automatically inherit these fields unless they explicitly override them in their own frontmatter.

---

## 2. Content Assets
You can colocate images, PDFs, and other resources directly within your `content/` folders.

**Example Structure**:
```text
content/
  my-post/
    index.md
    diagram.png
    report.pdf
```
Kosh will copy `diagram.png` and `report.pdf` to the same output directory as `index.html`. In your Markdown, you can simply refer to them as:
```markdown
![Diagram](diagram.png)
[Download PDF](report.pdf)
```

---

## 3. Knowledge Graph
Kosh automatically analyzes the connections between your pages (links and tags) to generate a knowledge graph.
- **Interactive View**: Accessible at `/graph.html`.
- **Raw Data**: Available at `/graph.json`.
- **Configuration**: Use `features.generators.graph` in `kosh.yaml` to toggle this or include/exclude taxonomies.

---

## 4. Data-Driven Pages
Kosh can generate standalone pages from structured data in the `data/` directory.

**Requirements**:
1. A YAML or JSON file in `data/` (e.g., `data/projects.yaml`).
2. The data object must contain a `slug` (or `id`) and a `layout` key.

**Example `data/projects.yaml`**:
```yaml
- title: "Project Alpha"
  slug: "alpha"
  layout: "home"
  description: "A great project"
```
Kosh will automatically render `/alpha.html` using the `home` template and passing the project data as context.
