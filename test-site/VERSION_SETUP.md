# Test Site Versioning Setup - Summary

## Configuration (kosh.yaml)

Added version configuration:

```yaml
versions:
  - name: "v3.0 (latest)"
    path: ""
    isLatest: true
  - name: "v2.0"
    path: "v2.0"
  - name: "v1.0"
    path: "v1.0"
```

## Content Structure

### Root folder (content/) - Latest Version (v3.0)
- `getting-started.md` - Original getting started guide
- `docs-test.md` - Documentation test page
- `hello-world.md` - Hello world example
- `advanced/configuration.md` - Configuration guide

### v1.0 folder (content/v1.0/) - Version 1.0
- `getting-started.md` - **Modified** for v1.0 (shows v1.0 specific content)
  - Title: "Getting Started (v1.0)"
  - Describes basic features available in v1.0
  - Includes note about viewing older version

### v2.0 folder (content/v2.0/) - Version 2.0
- `getting-started.md` - **Modified** for v2.0 (shows v2.0 specific content)
  - Title: "Getting Started (v2.0)"
  - Describes new features in v2.0
  - Includes migration guide from v1.0
  
- `new-in-v2.md` - **v2.0 ONLY** page
  - "New in v2.0" - features exclusive to v2.0+
  - Not available in v1.0 (will fall back to latest when accessed)
  
- `advanced/configuration.md` - **Modified** configuration guide for v2.0
  - v2.0 specific configuration options
  - Breaking changes from v1.0

## Generated Output Structure

```
public/
├── index.html                          # Latest version homepage
├── getting-started.html                # Latest (v3.0) getting started
├── docs-test.html                      # Latest docs test
├── hello-world.html                    # Latest hello world
├── advanced/
│   └── configuration.html              # Latest configuration
├── v1.0/
│   └── getting-started.html            # v1.0 specific
└── v2.0/
    ├── getting-started.html            # v2.0 specific
    ├── new-in-v2.html                  # v2.0 only
    └── advanced/
        └── configuration.html          # v2.0 specific
```

## Features Working

### ✅ Version Selector
- Dropdown in header showing all versions
- Current version highlighted
- Switching versions keeps you on the same page

### ✅ Version Banner
- Shows "You are viewing v2.0" for outdated versions
- "View latest version" link takes you to the same page in latest version
- NOT shown for latest version

### ✅ Breadcrumbs
- Shows: Home / Page Name
- Works correctly for all versions
- Links are functional

### ✅ Prev/Next Navigation
- Shows previous and next pages within the same version
- Respects version context (v2.0 links stay in v2.0)
- Uses weight-based ordering

### ✅ Sidebar/TOC
- Shows version-specific navigation tree
- Hierarchical structure with sections
- Active page highlighting

### ✅ Fallback System
- Pages not in v1.0 folder use latest version (v3.0)
- Example: v1.0/docs-test.html doesn't exist, so it falls back to latest
- Seamless navigation between versions

## How to Test

1. **Start the server:**
   ```bash
   cd test-site
   ../kosh.exe serve
   ```

2. **View Latest Version (v3.0):**
   - http://localhost:2604/getting-started.html
   - Shows original content
   - Version selector shows "v3.0 (latest)"
   - NO outdated banner

3. **View v2.0:**
   - http://localhost:2604/v2.0/getting-started.html
   - Shows v2.0 specific content with "What's New in v2.0"
   - Version selector shows "v2.0" selected
   - Shows yellow outdated banner with link to latest
   - Breadcrumbs: Home / Getting Started
   - Prev/Next links work within v2.0

4. **View v1.0:**
   - http://localhost:2604/v1.0/getting-started.html
   - Shows v1.0 content with "What's in v1.0"
   - Version selector shows "v1.0" selected
   - Shows outdated banner

5. **Test Fallback:**
   - http://localhost:2604/v1.0/docs-test.html (doesn't exist in v1.0)
   - Should redirect/fallback to latest version automatically
   - Or shows 404 if fallback not configured

6. **Switch Versions:**
   - Use dropdown in header
   - Try switching between v1.0, v2.0, and latest
   - Page content changes based on version

## Key Behaviors

1. **Sparse Versioning:** Only modified files need to be in version folders
2. **Automatic Fallback:** Missing files use latest version
3. **URL Structure:** 
   - Latest: `/page.html`
   - Versioned: `/v2.0/page.html`
4. **Search:** Will index all versions (cross-version search)
5. **Navigation:** Prev/Next and breadcrumbs respect version context

## Files Created/Modified

### New Content Files:
- `content/v1.0/getting-started.md`
- `content/v2.0/getting-started.md`
- `content/v2.0/new-in-v2.md`
- `content/v2.0/advanced/configuration.md`

### Modified Configuration:
- `kosh.yaml` - Added versions section

### Code Changes (already done):
- `builder/config/config.go` - Version struct
- `builder/models/models.go` - PageData extensions
- `builder/utils/version.go` - Version utilities
- `builder/utils/breadcrumbs.go` - Breadcrumb generation
- `builder/utils/navigation.go` - Prev/next logic
- `builder/run/pipeline_posts.go` - Version-aware rendering
- `test-site/themes/docs/templates/layout.html` - Version UI
- `test-site/themes/docs/static/css/layout.css` - Version styles
