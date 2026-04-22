# Kosh SSG: Technical Deep-Dive

Kosh is a high-performance, strictly-typed Static Site Generator (SSG) built in Go. It is designed with a focus on source immutability, atomic publishing, and a client-side search engine that rivals server-side solutions.

---

## 1. System Architecture & Orchestration

Kosh operates as a pipelined engine where tasks (Images, Text, Assets) are processed in parallel using a worker-pool model.

### The Build Pipeline
The orchestration layer (`builder/orchestration/build.go`) manages two primary build modes:
- **Clean Build (`kosh build` / `kosh clean`)**: Performs a full site reconstruction in a isolated staging directory, ensuring that the final output is atomic and consistent.
- **Incremental Dev Build (`kosh serve --dev`)**: Monitors the file system and performs targeted rebuilds (e.g., single-post re-renders) to minimize latency during development.

### Scheduler Weights
To maximize CPU utilization, tasks are weighted based on their resource intensity:
- `TaskImage` (Weight: 300): Heavy CPU/IO for WebP conversion.
- `TaskMarkdown` (Weight: 100): Standard parsing.
- `TaskAsset` (Weight: 50): Simple copy or bundle operations.

---

## 2. Transaction & Sink System

Source integrity is a core principle of Kosh. The engine is architected to ensure that the site `content/` and `static/` directories are never mutated by the build process.

### Atomic Publishing
1. **Staging**: All files are written to a unique staging directory (`.kosh-staging-[uuid]`).
2. **Commit**: Once the build succeeds, Kosh performs a directory swap:
   - Current output is moved to a backup directory.
   - Staging is moved to the final output destination.
3. **Rollback**: If the swap fails, the backup is restored, ensuring the live site is never in a partial state.

### The Sink Layer
Outputs are managed by `builder/utils/sink.go`, which enforces boundaries on where the SSG can write. This layer prevents hard-linking from source to output, protecting original assets from accidental deletion or corruption.

---

## 3. Content & SSR Pipeline

Kosh uses a hybrid content processing pipeline designed for speed and rich features.

### Unified Parser
- **Scanner Phase**: Fast extraction of frontmatter and metadata for site-wide indexing.
- **Parser Phase (Goldmark)**: Fully semantic Markdown-to-HTML conversion with extensions for wikilinks, attributes, and more.

### Server-Side Rendering (SSR)
Kosh implements SSR for complex content to ensure zero runtime impact on the client:
- **Math (KaTeX)**: LaTeX is rendered at build-time.
- **Diagrams (D2)**: Diagram source is compiled into SVG and embedded directly into the HTML.
- **SSR Cache**: Both Math and D2 results are stored in a persistent BoltDB cache, avoiding redundant renders for unchanged blocks.

### Metadata Cascading
Kosh supports metadata inheritance via `_index.md` files. Fields defined in a section index "cascade" down to all posts in that directory, enabling powerful directory-based configuration.

---

## 4. Asset Pipeline

The asset pipeline handles the lifecycle of CSS, JS, and multimedia assets.

### Bundling & Hashing
Kosh uses **Esbuild** to bundle theme assets. Every bundled file is injected with a content hash (e.g., `styles.a9b2c.css`).
- **Asset Mapping**: A JSON map tracks the relationship between source files and their hashed production versions.
- **Live Injection**: During `kosh serve`, the engine ensures that HTML files are re-rendered immediately when an asset hash changes, preventing "Stale CSS" issues.

### Multimedia Optimization
- **WebP Transformation**: All eligible raster images (`.png`, `.jpg`, `.jpeg`) are converted to WebP format.
- **Source Sync**: After conversion, Kosh removes the original raster files from the *output* directory (to save space) but keeps them in the *source* tree.
- **Image Priority Queue**: High-priority images (hero sections) are processed first to ensure fast "Time to Visual Completion" during builds.

---

## 5. Search Engine: CSR Architecture

Kosh features a state-of-the-art client-side search engine that uses a **Compressed Sparse Row (CSR)** indexing strategy.

### The CSR Index
Unlike traditional engines that use heavy JSON maps, Kosh uses flat, pointer-free arrays for its index:
- **Lexicon**: A sorted `[]string` of all terms in the site.
- **Posting Tables**: Flat `uint32` arrays storing delta-encoded Document IDs and term positions.
- **Memory Efficiency**: This structure avoids Go GC pressure and results in a 40-70% smaller memory footprint compared to map-based indices.

### Ranking & Relevancy
- **BM25 Scoring**: Implementation of the probabilistic ranking function on the client.
- **Tiered Boosting**: Titles are indexed and scored separately, with significantly higher weights than body content.
- **Real-time Suggestions**: A prefix-based trie (simulated over the sorted lexicon) provides instant autocomplete.

### WASM Runtime
The search engine is written in Go and cross-compiled to WebAssembly. The WASM binary is Brotli-compressed and embedded into the Kosh binary, then deployed to the client as a single, high-performance module.

---

## 6. Performance & Caching

### BoltDB Persistence
Kosh uses BoltDB as its tactical high-speed cache. The persistent store holds:
- Parsed Frontmatter (Metadata)
- Rendered HTML Fragments (Navbar/Footer)
- SSR Results (Math/D2)
- Image Processing Metadata

### Pipelined Workers
The engine uses a "Fat-Tail" optimization strategy. Heavy tasks are piped into channels and picked up by background workers immediately, allowing Markdown parsing to happen while images are still being compressed.

---

## 7. Configuration & Schema

### v2.0 Terminology
As of version 2.0, Kosh has moved to generalized terminology to support non-blog use cases:
- `itemsPerPage` (Old: `postsPerPage`)
- `search: { isEnabled: true }` (Old: `search: true`)
- `ContentMeta` (Old: `PostMeta`)

### Schema Versioning
Both the BoltDB cache and the Search Index are versioned. On startup, Kosh validates the schema version and automatically invalidates/migrates stale caches to prevent runtime panics.
