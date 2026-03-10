// Templ renderer spike TODO

# Summary
- Implement a scoped hybrid templ renderer for the built-in blog theme so we can measure reflection-free rendering without touching docs/custom themes.
- Keep the existing `html/template` pipeline as the default; the experimental path is opt-in via a new build flag and only activates when the `TemplateDir` points at `themes/blog/templates`.
- Preserve compatibility with current `RenderService`, `PageData`, and asset-processing expectations, using typed adapters and shared helper functions inside the renderer layer.

# Phase 1: Config + gating
1. Define `Build.ExperimentalRenderer` (string/enum) in `builder/config/build_config.go` and propagate it into `config.Build`. Provide defaults in `LoadBuildConfig` so the field exists even when the YAML is missing.
   - Accept values such as `""` (off) and `"templ-blog"` (experimental gated path), and add helper `IsTemplBlog()` to make discovery easy.
   - Update `kosh.build.yaml` examples and docs (e.g., `docs/kosh.build.md`) so reviewers know how to enable the spike.
2. Within `builder/run/setup.go` and `builder/run/builder.go`, detect when `cfg.TemplateDir` points to the built-in `themes/blog/templates` directory and the experimental flag is set; set a boolean (e.g., `useTemplBlogRenderer`) and log both the flag value and the template directory.
   - Handle absolute vs relative paths consistently (use `filepath.Clean` + `utils.NormalizePath` or existing helpers) so the gate cannot be bypassed by symlinks.
   - Default the flag to `false` unless both the template directory matches and the experimental value is `"templ-blog"`.
3. Update builder wiring where `renderer.NewWithFs` and `services.NewRenderService` are called so they can instantiate either the legacy renderer (existing logic) or a new `templ.NewGenerator` implementation without changing `RenderService`.
   - Keep `RenderService` interface intact; the builder simply stores whichever implementation satisfies it.
   - Pass down the same sink, assets, and `cfg` values so there is no behavioral difference in incremental builds or asset registration.

# Phase 2: Typed adapter layer
1. Create internal view-model structs (e.g., `type blogLayoutView struct {...}`) in a new `builder/renderer/types.go`.
   - Provide separate structs for the layout shell (`blogLayoutView`) and the page/index data (`blogPageView`, `blogIndexView`), each listing only the fields templ templates use: trusted `template.HTML` for `Content`, `map[string]string` for `Assets`, slices for `Posts`/`PinnedPosts`, paginator, TOC, menu entries, version list, etc.
   - Add derived helpers such as `type MenuEntryView struct { Name, URL, Target, ID, Class string }` so templ components don’t rely on the full `config.MenuEntry`.
2. Add constructor helpers (e.g., `func newBlogLayoutView(data models.PageData) blogLayoutView`) alongside pure Go helpers for asset relativization, slugification, canonical URLs, and version metadata.
   - Reuse existing logic from `renderer.RenderPage` to avoid drifting results: e.g., reuse the `relativePrefix`/`BaseURL` adjustments already implemented there.
   - Keep these helpers in the renderer package (maybe a `helpers.go`) so they can be referenced by both templ components and the legacy renderer if needed.
3. Keep `models.PageData` untouched; tests for the adapter layer should live under `builder/renderer/types_test.go`/`helpers_test.go` and assert that the view models preserve all values (assets, menu, versions, etc.) before handing data to templ.

# Phase 3: Experimental templ renderer
1. Add `builder/renderer/templ_renderer.go` and `builder/renderer/templ_cache.go` implementing a caching templ renderer:
   - Create a templ component loader that reads generated `.templ` files for `layout` and `index`, compiles them to `templ.Component`, and caches them with the same hot-reload TTL logic we already have (reuse `templateCache` ideas but for templ).
   - Provide `RenderPage`/`RenderIndex` that accept the adapter view models, render to a `bytes.Buffer`, process the HTML (legacy regex step), optionally minify, and write via the same `Sink.WriteStream` as the legacy renderer.
   - Only permit the templ renderer to render blog posts/index pages; Graph/404/Sidebar remain on the legacy renderer, but the templ renderer still needs access to the shared `Sink`, asset map, and register-file bookkeeping.
2. Update `services/render_service.go` so it can hold either the legacy `renderer.Renderer` or the new templ renderer as implementation details.
   - Add a wrapping struct (e.g., `type templRenderAdapter struct { templ *templRenderer; legacy *renderer.Renderer }`) and implement `RenderPage`/`RenderIndex` by picking the appropriate backend.
   - Ensure methods like `SetAssets`, `RegisterFile`, and `ClearRenderedFiles` still delegate to the underlying renderer and keep snapshots consistent.
3. Propagate `ReloadTemplates` to both renderers.
   - When templ mode is enabled, reload templ components first, then fall back to the legacy reload for graph/404 templates.
   - Add thorough logging so the build log clearly shows which renderer was reloaded.

# Phase 4: Templ components
1. Create `.templ` representations of `themes/blog/templates/layout.html` and `index.html` in `builder/renderer/templ_components/`.
   - Keep the same structure (doctype, head, header, nav, hero, posts, footer) but use templ syntax: `{ data.Property }`, `@if`, `@for range`, etc.
   - Replace `{{ range }}` loops with templ `@for _, post := range data.Posts { ... }`.
   - Inline files like `themes/blog/static/js` should now be referred to via helpers (e.g., `@helpers.AssetLink(data.Assets, "/static/js/main.js")`).
2. Create helper packages under `builder/renderer/helpers` for Menu/Version iterators, paginator rendering, slug construction, and asset relativization.
   - Each helper should be exported as a Go function returning string/`template.HTML`, and templ files import them via `import "github.com/Kush-Singh-26/kosh/builder/renderer/helpers"`.
   - Expose CLI-friendly functions for `helpers.Relativize`, `helpers.Slugify`, and any `Config` property access needed in templates to avoid overloading templ with logic.
3. Keep the `.html` files untouched; the templ versions exist solely for the experimental renderer. Document that the HTML files remain the source of truth for production until the spike proves templ correct.

# Phase 5: Testing & benchmarking
1. Under `builder/renderer`, add parallel tests that render the same `models.PageData` using both templ and html/template renderers, then diff the normalized outputs (strip whitespace, canonicalize asset links). Cover:
   - Blog post layout: assert `<title>`, `<meta description>`, post list, metadata (date, reading time), TOC, `Content`, and hashed asset URLs are identical.
   - Blog index page: assert hero section, featured/pinned posts, tag cloud, paginator links, `Menu`, and `Versions` behave identically.
2. Extend `renderer/security_test.go` to run the same data through the templ renderer’s sanitization path.
3. Add integration tests (maybe under `builder/run`) that run a sample build/serve path with the experimental flag on and ensure:
   - The docs theme still uses the legacy renderer (`kosh.yaml` scenario).
   - `RenderGraph`/`Render404` still resolve via html/template even when templ is enabled.
   - `ReloadTemplates` works for both renderer types without panics.
4. Run benchmarks twice: once with the legacy renderer and once with templ enabled (`go test ./builder/benchmarks -run BenchmarkFullBuild` or similar). Capture the rendering phase durations and attach to the spike report so we can decide if templ is worth scaling to other templates.
5. Verify the entire test matrix (`go test ./builder/renderer/...`, `./builder/services`, `./builder/run`, `./builder/utils`) passes after re-wiring render service factories; document any failures tied to asset registration or template hot-reload so future agents can debug quickly.

# Acceptance
- The experimental flag defaults to off; legacy behavior must be unchanged when the flag is inactive.
- Template reloads (watch mode) still happen via `ReloadTemplates`, and the templ renderer should have an equivalent cache invalidation strategy (watch the same template files or recompile components on demand).
- Document the flag and how to run with it (e.g., `KOSH_BUILD_EXPERIMENTAL=templ-blog` or new YAML entry) so future reviewers can reproduce the spike.
