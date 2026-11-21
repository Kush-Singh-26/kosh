# Changelog

## [v3] - 2026-11-22

### Changed
- Render LaTeX as native MathML, with an embedded subset math font and browser-specific alphabet, accent, and spacing adjustments.
- Reduce the Brotli-compressed search WASM from approximately 2.9 MiB to 565 KiB by keeping server-only dependencies and MessagePack encoding machinery out of the browser runtime.
- Use binary-search term lookup and avoid redundant posting-list decoding during search filtering.
- Track every bundled asset in a versioned cache manifest, including fonts and shared chunks, for complete warm-build restoration.
- Track actual template dependencies to avoid unnecessary reparsing of unchanged templates and partials.
- Modernize benchmarks to use `testing.B.Loop` and remove obsolete generated WASM copies and KaTeX browser styles/fonts.
- Select the CI and release Go toolchain from `go.mod`.

### Fixed
- Prevent config hot reload from acquiring the build mutex twice.
- Reject late asset-processing failures before publishing a full build.
- Process image cache misses even when existing image outputs are eligible for reuse.
- Close every initialized native renderer worker during shutdown.

### Release Notes
- Regenerate the embedded search runtime and rebuild the site together when upgrading, so the runtime and search index remain in sync.
- Math rendering uses native browser MathML; KaTeX remains embedded for server-side LaTeX parsing.
- Source builds require the Go version specified in `go.mod` and CGO support.

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
