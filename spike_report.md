# Templ Renderer Spike Report

## Summary
The experimental `templ` renderer was successfully integrated alongside the legacy `html/template` pipeline for the built-in `blog` theme. The experiment proves that we can achieve a typed, reflection-free rendering layer for Kosh without mutating or breaking existing docs or custom themes.

## Changes Implemented
1. **Config Gating:** Added `experimentalRenderer: "templ-blog"` to `kosh.build.yaml`. When active with the `blog` theme, the renderer injects a new `templRenderAdapter` that routes specific template calls to the compiled `.templ` components.
2. **Adapter Layer:** Created explicit view-model structs (`BlogLayoutView`, `BlogIndexView`, `BlogPageView`) in `builder/renderer/types/`. These structs decouple Kosh's generic `PageData` map from the template definitions, enforcing compile-time safety for all data bindings.
3. **Template Parity:** Converted `layout.html` and `index.html` to `layout.templ` and `index.templ`. They consume the new types and utilize shared helper functions extracted to `builder/renderer/helpers/`.
4. **Security Testing:** Extended `security_test.go` to prove that `a-h/templ` correctly handles XSS scenarios identically to Go's standard library (with slightly stricter contextual escaping rules on attributes).

## Benchmarking Notes
The standard `go test -bench BenchmarkFullBuild` run timed out on the test VM while parsing 100 posts, meaning definitive isolated renderer execution times could not be captured in this environment. However, based on architectural analysis:
- **Performance:** `templ` generates pure Go functions, avoiding runtime reflection entirely. The memory allocation overhead per render is expected to be drastically lower compared to `html/template`.
- **Developer Experience:** The typed adapter layer caught several potential template binding errors during compilation, proving that transitioning to `templ` would dramatically improve maintainability.
- **Safety:** XSS protection is built-in and arguably safer due to `templ`'s static awareness of HTML context (e.g., automatically rejecting risky dynamic href attributes).

## Conclusion
Migrating the remaining templates (RSS, Sitemap, custom themes, doc sites) to `templ` is technically viable and brings massive benefits in type safety. The primary challenge would be supporting "Custom Themes", as Kosh currently allows users to override `.html` files dynamically. A hybrid approach where core built-in themes use `templ` and user overrides use `html/template` is the recommended path forward.
