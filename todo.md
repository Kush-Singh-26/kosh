# Kosh SSG Restoration - Priority TODOs

This file lists the critical regressions and bugs that need to be resolved to restore the Kosh SSG to its stable, production-ready state.

## 1. Sorting & Feed Integrity (CRITICAL)
*   **Issue:** Posts appear in random order on the homepage and in documentation sidebars.
*   **Fix:** In `builder/run/pipeline_posts.go`, call `utils.SortPosts(allPosts)` and `utils.SortPosts(postsByVersion[version])` before the rendering phase.

## 2. Documentation Versioning & Sidebar Isolation (BROKEN)
*   **Issue:** Version leakage in sidebars and incorrect linking (versioned pages link to root version).
*   **Fix:**
    *   Group posts by version in a map during the collection phase.
    *   Build a unique `SiteTree` for every version folder.
    *   Ensure `PostMetadata` uses `utils.BuildPostLink` to correctly prefix links with the version (e.g., `/v1.0/post.html`).

## 3. Documentation Navigation (MISSING)
*   **Issue:** "Previous" and "Next" buttons are gone from the docs theme.
*   **Fix:** Re-implement the neighbor-detection pass in `pipeline_posts.go` using `utils.FindPrevNext` within the version-isolated slices.

## 4. Raw Markdown Deployment (BROKEN)
*   **Issue:** `.md` files are not being copied to `public/`, breaking the "View Source" feature.
*   **Fix:** Restore the logic in `processPosts` that copies source Markdown files to their corresponding output paths in the `public` directory.

## 5. Incremental Build Regressions (STALE)
*   **Issue:** Saving a single file breaks its sidebar and navigation.
*   **Fix:** Update `builder/run/incremental.go` to re-fetch sibling metadata from the BoltDB cache and rebuild the sidebar/navigation context for the specific version being edited.

## 6. Concurrency & Safety (CRITICAL)
*   **Issue:** Fatal panic: `concurrent map read and map write`.
*   **Fix:** Protect all shared map and slice accesses in `pipeline_posts.go` (specifically `allMetadataMap`, `postsByVersion`, and `tagMap`) using the `mu` Mutex.

## 7. URL Protocol Corruption
*   **Issue:** Links starting with `http:/` instead of `http://`.
*   **Fix:** Ensure `BuildPostLink` in `builder/utils/version.go` does not strip the second slash from the protocol.

---

### Instructions for Implementation
1.  **DO NOT use git checkout** on source files as it reverts stabilized logic.
2.  Follow the **Two-Pass Build Architecture**:
    *   **Pass 1 (Collection):** Gather all metadata, rehydrate from cache, and group by version.
    *   **Pass 2 (Rendering):** Sort, find neighbors, build trees, and render using the calculated context.
3.  Always build the binary with memory-optimized flags on Windows: `go build -ldflags="-s -w" -o kosh.exe ./cmd/kosh`.
