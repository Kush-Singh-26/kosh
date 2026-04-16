# Changelog

## [2.0.0] - 2026-04-16

### Added
- Generalized taxonomy support: Use `Taxonomies` map in config and `ContentMetadata`.
- Dynamic content prefix support via `ContentPrefix` configuration.
- Custom article type configuration with `ArticleType`.
- Configurable PWA icons via `Icon192` and `Icon512`.

### Changed
- **Breaking:** Renamed core terminology from "Post" to "Content" or "Item".
  - `PostMetadata` -> `ContentMetadata`
  - `PostMeta` -> `ContentMeta`
  - `PostRecord` -> `ContentRecord`
  - `IndexedPost` -> `IndexedContent`
- **Breaking:** Configuration field renaming:
  - `PostsPerPage` -> `ItemsPerPage`
- Refactored `builder/services/post` package into `builder/services/content`.
- Updated themes to use dynamic taxonomies instead of hardcoded `"tags"`.

### Migration Guide
1. Update `config.yaml`:
   - Rename `postsPerPage` to `itemsPerPage`.
   - Add `articleType: "BlogPosting"` (optional, default).
   - Configure `taxonomies` as a map of your choice (e.g., `tags: "tags"`, `categories: "categories"`).
2. Update templates:
   - Use `$.Config.Taxonomies` to iterate over available taxonomies.
   - Replace any hardcoded `/tags/` paths with dynamic lookups.
   - Use `$.Config.ContentPrefix` for link generation.
