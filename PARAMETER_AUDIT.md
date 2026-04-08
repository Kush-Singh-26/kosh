# Parameter Audit: Functions with Too Many Parameters

## Summary

This document lists all standalone functions (not methods) in the Kosh codebase that have **more than 4 parameters** and should be converted to use the struct/options pattern.

**Total Functions Found:** 24

---

## High Priority (>7 parameters)

### 1. CopyFromDiskCache - 12 parameters

**File:** `builder/assets/image_cache.go:193`

```go
func CopyFromDiskCache(srcFs afero.Fs, sink fspkg.ArtifactSink, relPath, srcPath, dstPath, cacheDir string, srcInfo fs.FileInfo, metrics ImageMetrics, onWrite func(string), keepOriginal bool, muteMetrics bool) error
```

**Recommended:** Create `CopyFromDiskCacheOptions` struct

---

### 2. BuildAssetsEsbuild - 10 parameters

**File:** `builder/fs/assets.go:71`

```go
func BuildAssetsEsbuild(srcFs afero.Fs, sink ArtifactSink, srcDir, destDir string, minify bool, onWrite func(string), cacheDir string, force bool, sched scheduler.BuildScheduler, onAssetProcessed func()) (map[string]string, error)
```

**Recommended:** Create `BuildAssetsOptions` struct

---

### 3. GetFrontmatterHashFromValues - 8 parameters

**File:** `builder/hashing/hash.go:81`

```go
func GetFrontmatterHashFromValues(title, description, date string, tags []string, pinned, draft bool, weight int, other map[string]any) string
```

**Recommended:** Create `FrontmatterHashOptions` struct

---

### 4. hashStandardFields - 8 parameters

**File:** `builder/hashing/hash.go:113`

```go
func hashStandardFields(h *xxh3.Hasher, title, description, date string, tags []string, pinned, draft bool, weight int)
```

**Recommended:** Create `HashStandardFieldsOptions` struct

---

### 5. maybeCopyOriginal - 7 parameters

**File:** `builder/assets/image_processing.go:55`

```go
func maybeCopyOriginal(srcFs afero.Fs, sink fspkg.ArtifactSink, srcPath, dstWebp string, srcInfo fs.FileInfo, onWrite func(string), keepOriginal bool) error
```

**Recommended:** Refactor to use embedded struct

---

## Medium Priority (6-7 parameters)

### 6. GenerateSW - 7 parameters

**File:** `builder/generators/pwa.go:20`

```go
func GenerateSW(sink fspkg.ArtifactSink, destDir string, buildVersion int64, forceRebuild bool, baseURL string, assets map[string]string, isTesting bool) error
```

**Recommended:** Create `SWOptions` struct

---

### 7. GenerateManifest - 7 parameters

**File:** `builder/generators/pwa.go:80`

```go
func GenerateManifest(sink fspkg.ArtifactSink, destDir string, baseURL string, siteTitle string, siteDescription string, forceRebuild bool, isTesting bool) error
```

**Recommended:** Create `ManifestOptions` struct

---

### 8. NewWithFs - 6 parameters

**File:** `builder/renderer/renderer.go:50`

```go
func NewWithFs(sourceFs afero.Fs, compress bool, sink fspkg.ArtifactSink, templateDir string, devMode bool, logger *slog.Logger) *Renderer
```

**Recommended:** Create `RendererOptions` struct (merge with New)

---

### 9. GenerateRSS - 6 parameters

**File:** `builder/generators/rss.go:13`

```go
func GenerateRSS(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, title, description string, outputPath string) (string, error)
```

**Recommended:** Create `RSSOptions` struct

---

### 10. GenerateGraph - 6 parameters

**File:** `builder/generators/graph.go:45`

```go
func GenerateGraph(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, outputPath string, cfg models.GraphConfig, siteTitle string) (string, string, error)
```

**Recommended:** Use existing `GraphConfig` as options (already done)

---

### 11. Run (Server) - 6 parameters

**File:** `internal/server/server.go:62`

```go
func Run(ctx context.Context, args []string, outputDir string, baseURL string, buildCfg *config.BuildConfig, reporter ui.Reporter)
```

**Recommended:** Create `ServerOptions` struct

---

## Low Priority (5 parameters)

### 12. GenerateSitemap - 5 parameters

**File:** `builder/generators/sitemap.go:16`

```go
func GenerateSitemap(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, tags map[string][]models.PostMetadata, outputPath string) (string, error)
```

**Recommended:** Create `SitemapOptions` struct

---

### 13. ShouldGenerateSocialCard - 5 parameters

**File:** `builder/generators/social.go:42`

```go
func ShouldGenerateSocialCard(cache models.SocialCardCache, cacheKey, currentHash, cachedCardPath string, force bool) bool
```

**Recommended:** Use existing `SocialCardOptions` struct (expand it)

---

### 14. GeneratePWAIcons - 5 parameters

**File:** `builder/generators/pwa.go:234`

```go
func GeneratePWAIcons(srcFs afero.Fs, sink fspkg.ArtifactSink, srcPath, destDir string, logger *slog.Logger) error
```

**Recommended:** Create `PWAIconsOptions` struct

---

### 15. CheckWASMFsWithSource - 5 parameters

**File:** `builder/assets/wasm.go:54`

```go
func CheckWASMFsWithSource(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir string, sourceWasm []byte, compressionLevel int) bool
```

**Recommended:** Create `CheckWASMOptions` struct

---

### 16. DeployWASMFromFileWithLevel - 5 parameters

**File:** `builder/assets/wasm.go:158`

```go
func DeployWASMFromFileWithLevel(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir, sourcePath string, level int) bool
```

**Recommended:** Create `DeployWASMOptions` struct

---

### 17. RenameWithRetry - 5 parameters

**File:** `builder/retry/retry.go:12`

```go
func RenameWithRetry(ctx context.Context, oldPath, newPath string, maxRetries int, baseDelay time.Duration) error
```

**Recommended:** Create `RenameOptions` struct

---

### 18. ParallelWalk - 5 parameters

**File:** `builder/fs/walk.go:19`

```go
func ParallelWalk(ctx context.Context, sourceFs afero.Fs, root string, concurrency int, walkFn WalkFunc) error
```

**Recommended:** Create `WalkOptions` struct

---

### 19. SyncVFS - 5 parameters

**File:** `builder/async/sync.go:78`

```go
func SyncVFS(ctx context.Context, srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool, isCleanBuild bool) error
```

**Recommended:** Create `VFSOptions` struct

---

### 20. syncSingleFileTask - 5 parameters

**File:** `builder/async/sync.go:174`

```go
func syncSingleFileTask(ctx context.Context, srcFs afero.Fs, task syncTask, isCleanBuild bool, tx *fs.TxSync) error
```

**Recommended:** Create `SyncTaskOptions` struct

---

### 21. drawGradient - 5 parameters

**File:** `builder/generators/social_draw.go:10`

```go
func drawGradient(dc *gg.Context, w, h int, colors []string, angle int)
```

**Recommended:** Create `GradientOptions` struct

---

### 22. New (Renderer) - 5 parameters

**File:** `builder/renderer/renderer.go:46`

```go
func New(compress bool, sink fspkg.ArtifactSink, templateDir string, devMode bool, logger *slog.Logger) *Renderer
```

**Recommended:** Use `RendererOptions` (merged with NewWithFs)

---

### 23. RenderParsedMarkdown - 5 parameters

**File:** `builder/services/post/post_parser.go:142`

```go
func RenderParsedMarkdown(source []byte, res *ParsedMarkdownResult, mdPool *sync.Pool, nativeRenderer *native.Renderer, diagramAdapter *cache.DiagramCacheAdapter) error
```

**Recommended:** Use existing `ParseOptions` struct (expand it to 17 fields)

---

### 24. NewBuildContext - 5 parameters

**File:** `builder/context/context.go:19`

```go
func NewBuildContext(isTesting, isDev, isClean bool, s scheduler.BuildScheduler, l *slog.Logger) *BuildContext
```

**Recommended:** Already creates struct - could accept struct as parameter

---

## Already Using Struct Pattern (No Change Needed)

| File | Function | Status |
|------|----------|--------|
| `assets/pipeline.go:48` | `CopyDirVFS` | Uses `CopyOptions` ✓ |
| `generators/social.go:113` | `SocialCardOptions` | Already struct with 9 fields ✓ |

---

## Recommended Struct Definitions

### CopyFromDiskCacheOptions (12 fields)

```go
type CopyFromDiskCacheOptions struct {
    SrcFs         afero.Fs
    Sink          fspkg.ArtifactSink
    RelPath       string
    SrcPath       string
    DstPath       string
    CacheDir      string
    SrcInfo       fs.FileInfo
    Metrics       ImageMetrics
    OnWrite       func(string)
    KeepOriginal  bool
    MuteMetrics   bool
}
```

### BuildAssetsOptions (10 fields)

```go
type BuildAssetsOptions struct {
    SrcFs             afero.Fs
    Sink              ArtifactSink
    SrcDir            string
    DestDir           string
    Minify            bool
    OnWrite           func(string)
    CacheDir          string
    Force            bool
    Sched            scheduler.BuildScheduler
    OnAssetProcessed func()
}
```

### FrontmatterHashOptions (8 fields)

```go
type FrontmatterHashOptions struct {
    Title       string
    Description string
    Date        string
    Tags        []string
    Pinned      bool
    Draft       bool
    Weight      int
    Other       map[string]any
}
```

---

## Priority Matrix

| Priority | Count | Threshold |
|----------|-------|-----------|
| High     | 5     | >7 params |
| Medium   | 6     | 6-7 params |
| Low      | 13    | 5 params  |

---

---

## Detailed Refactoring Plan

This section provides exact changes needed for each function.

---

### 1. CopyFromDiskCache (12 params)

**File:** `builder/assets/image_cache.go:193`

**Current:**
```go
func CopyFromDiskCache(srcFs afero.Fs, sink fspkg.ArtifactSink, relPath, srcPath, dstPath, cacheDir string, srcInfo fs.FileInfo, metrics ImageMetrics, onWrite func(string), keepOriginal bool, muteMetrics bool) error
```

**Change to:**
```go
type CopyFromDiskCacheOptions struct {
    SrcFs         afero.Fs
    Sink          fspkg.ArtifactSink
    RelPath       string
    SrcPath      string
    DstPath      string
    CacheDir     string
    SrcInfo      fs.FileInfo
    Metrics     ImageMetrics
    OnWrite     func(string)
    KeepOriginal bool
    MuteMetrics bool
}

func CopyFromDiskCache(opts CopyFromDiskCacheOptions) error
```

**Call sites to update (1 location):**
- `builder/services/asset/asset_service.go:246`:
  ```go
  // Before:
  err := assets.CopyFromDiskCache(s.sourceFs, s.sink, t.relPath, t.srcPath, dstWebp, ...)
  // After:
  err := assets.CopyFromDiskCache(assets.CopyFromDiskCacheOptions{
      SrcFs:    s.sourceFs,
      Sink:    s.sink,
      RelPath: t.relPath,
      SrcPath: t.srcPath,
      DstPath: dstWebp,
      // ... fill in remaining fields
  })
  ```

---

### 2. BuildAssetsEsbuild (10 params)

**File:** `builder/fs/assets.go:71`

**Current:**
```go
func BuildAssetsEsbuild(srcFs afero.Fs, sink ArtifactSink, srcDir, destDir string, minify bool, onWrite func(string), cacheDir string, force bool, sched scheduler.BuildScheduler, onAssetProcessed func()) (map[string]string, error)
```

**Change to:**
```go
type BuildAssetsOptions struct {
    SrcFs             afero.Fs
    Sink              ArtifactSink
    SrcDir            string
    DestDir           string
    Minify            bool
    OnWrite           func(string)
    CacheDir          string
    Force            bool
    Sched            scheduler.BuildScheduler
    OnAssetProcessed func()
}

func BuildAssetsEsbuild(opts BuildAssetsOptions) (map[string]string, error)
```

**Call sites to update (1 location):**
- `builder/services/asset/asset_service.go:566`:
  ```go
  // Before:
  assets, assetErr := fspkg.BuildAssetsEsbuild(s.sourceFs, s.sink, srcDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force, sched, onAssetProcessed)
  // After:
  assets, assetErr := fspkg.BuildAssetsEsbuild(fspkg.BuildAssetsOptions{
      SrcFs:     s.sourceFs,
      Sink:     s.sink,
      SrcDir:   srcDir,
      DestDir:  destStaticDir,
      Minify:   s.cfg.CompressImages,
      OnWrite:  s.renderer.RegisterFile,
      CacheDir: s.cfg.CacheDir+"/assets",
      Force:   force,
      Sched:   sched,
      OnAssetProcessed: onAssetProcessed,
  })
  ```

---

### 3. GetFrontmatterHashFromValues (8 params)

**File:** `builder/hashing/hash.go:81`

**Current:**
```go
func GetFrontmatterHashFromValues(title, description, date string, tags []string, pinned, draft bool, weight int, other map[string]any) string
```

**Change to:**
```go
type FrontmatterHashOptions struct {
    Title       string
    Description string
    Date        string
    Tags        []string
    Pinned      bool
    Draft       bool
    Weight     int
    Other      map[string]any
}

func GetFrontmatterHashFromValues(opts FrontmatterHashOptions) string
```

**Call sites to update (1 location):**
- `builder/services/scanner/scanner.go:171`:
  ```go
  // Before:
  frontmatterHash := hashing.GetFrontmatterHashFromValues(...)
  // After:
  frontmatterHash := hashing.GetFrontmatterHashFromValues(hashing.FrontmatterHashOptions{
      Title:       /* ... */,
      Description: /* ... */,
      // ... etc
  })
  ```

---

### 4. hashStandardFields (8 params)

**File:** `builder/hashing/hash.go:113`

**Current:**
```go
func hashStandardFields(h *xxh3.Hasher, title, description, date string, tags []string, pinned, draft bool, weight int)
```

**Change to:**
```go
type HashStandardFieldsOptions struct {
    Hasher       *xxh3.Hasher
    Title       string
    Description string
    Date        string
    Tags        []string
    Pinned      bool
    Draft       bool
    Weight     int
}

func hashStandardFields(opts HashStandardFieldsOptions)
```

**Call sites to update (2 locations):**
- `builder/hashing/hash.go:48` - internal call
- `builder/hashing/hash.go:84` - internal call
  ```go
  // Before:
  hashStandardFields(h, title, description, date, tags, pinned, draft, weight)
  // After:
  hashStandardFields(HashStandardFieldsOptions{
      Hasher:       h,
      Title:       title,
      Description: description,
      Date:        date,
      Tags:        tags,
      Pinned:      pinned,
      Draft:       draft,
      Weight:      weight,
  })
  ```

---

### 5. maybeCopyOriginal (7 params)

**File:** `builder/assets/image_processing.go:55`

**Current:**
```go
func maybeCopyOriginal(srcFs afero.Fs, sink fspkg.ArtifactSink, srcPath, dstWebp string, srcInfo fs.FileInfo, onWrite func(string), keepOriginal bool) error
```

**Change to:**
```go
type MaybeCopyOriginalOptions struct {
    SrcFs         afero.Fs
    Sink          fspkg.ArtifactSink
    SrcPath       string
    DstWebp      string
    SrcInfo      fs.FileInfo
    OnWrite      func(string)
    KeepOriginal bool
}

func maybeCopyOriginal(opts MaybeCopyOriginalOptions) error
```

**Call sites to update (3 locations):**
- `builder/assets/image_processing.go:215`
- `builder/assets/image_processing.go:251`
- `builder/assets/image_processing.go:376`
  ```go
  // Before:
  _ = maybeCopyOriginal(opts.SrcFs, opts.Sink, opts.SrcPath, opts.DstPath, opts.SrcInfo, opts.Opts.OnWrite, opts.Opts.KeepOriginal)
  // After:
  _ = maybeCopyOriginal(MaybeCopyOriginalOptions{
      SrcFs:         opts.SrcFs,
      Sink:          opts.Sink,
      SrcPath:       opts.SrcPath,
      DstWebp:      opts.DstPath,
      SrcInfo:      opts.SrcInfo,
      OnWrite:      opts.Opts.OnWrite,
      KeepOriginal: opts.Opts.KeepOriginal,
  })
  ```

---

### 6. GenerateSW (7 params)

**File:** `builder/generators/pwa.go:20`

**Current:**
```go
func GenerateSW(sink fspkg.ArtifactSink, destDir string, buildVersion int64, forceRebuild bool, baseURL string, assets map[string]string, isTesting bool) error
```

**Change to:**
```go
type SWOptions struct {
    Sink         fspkg.ArtifactSink
    DestDir      string
    BuildVersion int64
    ForceRebuild bool
    BaseURL      string
    Assets       map[string]string
    IsTesting    bool
}

func GenerateSW(opts SWOptions) error
```

**Call sites to update (2 locations):**
- `builder/orchestration/pipeline_pwa.go:35`:
  ```go
  // Before:
  return generators.GenerateSW(b.Sink, b.Cfg.OutputDir, b.Cfg.BuildVersion, shouldForce, b.Cfg.BaseURL, b.Deps.Render.GetAssets(), b.Ctx.IsTesting)
  // After:
  return generators.GenerateSW(generators.SWOptions{
      Sink:         b.Sink,
      DestDir:      b.Cfg.OutputDir,
      BuildVersion: b.Cfg.BuildVersion,
      ForceRebuild: shouldForce,
      BaseURL:     b.Cfg.BaseURL,
      Assets:      b.Deps.Render.GetAssets(),
      IsTesting:   b.Ctx.IsTesting,
  })
  ```
- `builder/generators/pwa_test.go:21` - test function

---

### 7. GenerateManifest (7 params)

**File:** `builder/generators/pwa.go:80`

**Current:**
```go
func GenerateManifest(sink fspkg.ArtifactSink, destDir string, baseURL string, siteTitle string, siteDescription string, forceRebuild bool, isTesting bool) error
```

**Change to:**
```go
type ManifestOptions struct {
    Sink           fspkg.ArtifactSink
    DestDir        string
    BaseURL        string
    SiteTitle      string
    SiteDescription string
    ForceRebuild  bool
    IsTesting      bool
}

func GenerateManifest(opts ManifestOptions) error
```

**Call sites to update (2 locations):**
- `builder/orchestration/pipeline_pwa.go:39`:
  ```go
  // Before:
  return generators.GenerateManifest(b.Sink, b.Cfg.OutputDir, b.Cfg.BaseURL, b.Cfg.Title, b.Cfg.Description, shouldForce, b.Ctx.IsTesting)
  // After:
  return generators.GenerateManifest(generators.ManifestOptions{
      Sink:             b.Sink,
      DestDir:          b.Cfg.OutputDir,
      BaseURL:          b.Cfg.BaseURL,
      SiteTitle:         b.Cfg.Title,
      SiteDescription:  b.Cfg.Description,
      ForceRebuild:    shouldForce,
      IsTesting:       b.Ctx.IsTesting,
  })
  ```
- `builder/generators/pwa_test.go:58` - test function

---

### 8. NewWithFs (6 params)

**File:** `builder/renderer/renderer.go:50`

**Current:**
```go
func NewWithFs(sourceFs afero.Fs, compress bool, sink fspkg.ArtifactSink, templateDir string, devMode bool, logger *slog.Logger) *Renderer
```

**Change to:**
```go
type RendererOptions struct {
    SourceFs   afero.Fs
    Compress   bool
    Sink       fspkg.ArtifactSink
    TemplateDir string
    DevMode   bool
    Logger    *slog.Logger
}

func NewWithFs(opts RendererOptions) *Renderer
```

**Note:** Merge with `New` function - use single constructor accepting `RendererOptions`.

---

### 9. GenerateRSS (6 params)

**File:** `builder/generators/rss.go:13`

**Current:**
```go
func GenerateRSS(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, title, description string, outputPath string) (string, error)
```

**Change to:**
```go
type RSSOptions struct {
    Sink       fspkg.ArtifactSink
    BaseURL    string
    Posts     []models.PostMetadata
    Title     string
    Description string
    OutputPath string
}

func GenerateRSS(opts RSSOptions) (string, error)
```

**Call sites to update (3 locations):**
- `builder/orchestration/site_wide.go:119`:
  ```go
  // Before:
  _, err := generators.GenerateRSS(b.Sink, b.Cfg.BaseURL, allPosts, b.Cfg.Title, b.Cfg.Description, filepath.Join(b.Cfg.OutputDir, "rss.xml"))
  // After:
  _, err := generators.GenerateRSS(generators.RSSOptions{
      Sink:       b.Sink,
      BaseURL:    b.Cfg.BaseURL,
      Posts:      allPosts,
      Title:      b.Cfg.Title,
      Description: b.Cfg.Description,
      OutputPath: filepath.Join(b.Cfg.OutputDir, "rss.xml"),
  })
  ```
- `builder/generators/rss_test.go:34` - test function
- `builder/generators/rss_test.go:58` - test function
- `builder/generators/rss_test.go:84` - test function

---

### 10. GenerateGraph (6 params)

**File:** `builder/generators/graph.go:45`

**Current:**
```go
func GenerateGraph(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, outputPath string, cfg models.GraphConfig, siteTitle string) (string, string, error)
```

**Change to:** Already uses `GraphConfig` struct - just rename parameter to make it clearer or wrap in options struct.

```go
type GraphOptions struct {
    Sink       fspkg.ArtifactSink
    BaseURL    string
    Posts     []models.PostMetadata
    OutputPath string
    Config    models.GraphConfig
    SiteTitle string
}

func GenerateGraph(opts GraphOptions) (string, string, error)
```

**Call sites to update (2 locations):**
- `builder/orchestration/site_wide.go:154`:
  ```go
  // Before:
  _, _, err := generators.GenerateGraph(b.Sink, b.Cfg.BaseURL, allPosts, filepath.Join(b.Cfg.OutputDir, "graph.json"), b.Cfg.Features.Generators.Graph, b.Cfg.Title)
  // After:
  _, _, err := generators.GenerateGraph(generators.GraphOptions{
      Sink:       b.Sink,
      BaseURL:    b.Cfg.BaseURL,
      Posts:     allPosts,
      OutputPath: filepath.Join(b.Cfg.OutputDir, "graph.json"),
      Config:     b.Cfg.Features.Generators.Graph,
      SiteTitle:  b.Cfg.Title,
  })
  ```
- `builder/generators/graph_test.go:34,119,144` - test functions

---

### 11. Run (Server) - 6 params

**File:** `internal/server/server.go:62`

**Current:**
```go
func Run(ctx context.Context, args []string, outputDir string, baseURL string, buildCfg *config.BuildConfig, reporter ui.Reporter)
```

**Change to:**
```go
type ServerOptions struct {
    Context   context.Context
    Args      []string
    OutputDir string
    BaseURL   string
    BuildConfig *config.BuildConfig
    Reporter  ui.Reporter
}

func Run(opts ServerOptions)
```

**Note:** Already uses `*config.BuildConfig` - could expand config to include all server options instead.

---

### 12. GenerateSitemap (5 params)

**File:** `builder/generators/sitemap.go:16`

**Current:**
```go
func GenerateSitemap(sink fspkg.ArtifactSink, baseURL string, posts []models.PostMetadata, tags map[string][]models.PostMetadata, outputPath string) (string, error)
```

**Change to:**
```go
type SitemapOptions struct {
    Sink       fspkg.ArtifactSink
    BaseURL    string
    Posts     []models.PostMetadata
    Tags      map[string][]models.PostMetadata
    OutputPath string
}

func GenerateSitemap(opts SitemapOptions) (string, error)
```

**Call sites to update (2 locations):**
- `builder/orchestration/site_wide.go:95`
- `builder/generators/sitemap_test.go` - test functions

---

### 13. GeneratePWAIcons (5 params)

**File:** `builder/generators/pwa.go:234`

**Current:**
```go
func GeneratePWAIcons(srcFs afero.Fs, sink fspkg.ArtifactSink, srcPath, destDir string, logger *slog.Logger) error
```

**Change to:**
```go
type PWAIconsOptions struct {
    SrcFs   afero.Fs
    Sink    fspkg.ArtifactSink
    SrcPath string
    DestDir string
    Logger  *slog.Logger
}

func GeneratePWAIcons(opts PWAIconsOptions) error
```

**Call sites to update:** None found (internal function)

---

### 14. CheckWASMFsWithSource (5 params)

**File:** `builder/assets/wasm.go:54`

**Current:**
```go
func CheckWASMFsWithSource(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir string, sourceWasm []byte, compressionLevel int) bool
```

**Change to:**
```go
type CheckWASMOptions struct {
    Fs               afero.Fs
    Sink             fspkg.ArtifactSink
    CacheDir         string
    SourceWasm      []byte
    CompressionLevel int
}

func CheckWASMFsWithSource(opts CheckWASMOptions) bool
```

**Call sites to update (2 locations):**
- `builder/assets/wasm.go:51` - internal call
- `builder/assets/wasm.go:167` - internal call
- `builder/assets/wasm_test.go` - test functions

---

### 15. DeployWASMFromFileWithLevel (5 params)

**File:** `builder/assets/wasm.go:158`

**Current:**
```go
func DeployWASMFromFileWithLevel(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir, sourcePath string, level int) bool
```

**Change to:**
```go
type DeployWASMOptions struct {
    Fs        afero.Fs
    Sink      fspkg.ArtifactSink
    CacheDir  string
    SourcePath string
    Level     int
}

func DeployWASMFromFileWithLevel(opts DeployWASMOptions) bool
```

**Call sites to update (1 location):**
- `builder/assets/wasm.go:155` - internal call

---

### 16. RenameWithRetry (5 params)

**File:** `builder/retry/retry.go:12`

**Current:**
```go
func RenameWithRetry(ctx context.Context, oldPath, newPath string, maxRetries int, baseDelay time.Duration) error
```

**Change to:**
```go
type RenameOptions struct {
    Context   context.Context
    OldPath   string
    NewPath   string
    MaxRetries int
    BaseDelay time.Duration
}

func RenameWithRetry(opts RenameOptions) error
```

**Call sites to update (8 locations):**
- `builder/cache/store/store.go:227`
- `builder/fs/tx_sync.go:89`
- `builder/fs/tx/transaction.go:117,123,127`
- `builder/async/sync.go:279`
- `builder/cache/store/store_test.go:23,36` - test functions
- `builder/retry/retry_test.go` - multiple test functions

---

### 17. ParallelWalk (5 params)

**File:** `builder/fs/walk.go:19`

**Current:**
```go
func ParallelWalk(ctx context.Context, sourceFs afero.Fs, root string, concurrency int, walkFn WalkFunc) error
```

**Change to:**
```go
type WalkOptions struct {
    Context    context.Context
    SourceFs  afero.Fs
    Root       string
    Concurrency int
    WalkFn     WalkFunc
}

func ParallelWalk(opts WalkOptions) error
```

**Call sites to update (7 locations):**
- `builder/assets/pipeline.go:158`
- `builder/services/asset/asset_service.go:297`
- `builder/cache/store/store.go:297,323,344`
- `builder/fs/assets_scan.go:44`
- `builder/fs/walk_test.go` - multiple test functions

---

### 18. SyncVFS (5 params)

**File:** `builder/async/sync.go:78`

**Current:**
```go
func SyncVFS(ctx context.Context, srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool, isCleanBuild bool) error
```

**Change to:**
```go
type VFSOptions struct {
    Context    context.Context
    SrcFs     afero.Fs
    TargetDir string
    DirtyFiles map[string]bool
    IsCleanBuild bool
}

func SyncVFS(opts VFSOptions) error
```

**Call sites to update (2 locations):**
- `builder/async/rollback_test.go:46`
- `builder/async/sync_test.go:30,65`

---

### 19. drawGradient (5 params)

**File:** `builder/generators/social_draw.go:10`

**Current:**
```go
func drawGradient(dc *gg.Context, w, h int, colors []string, angle int)
```

**Change to:**
```go
type GradientOptions struct {
    DC     *gg.Context
    Width  int
    Height int
    Colors []string
    Angle  int
}

func drawGradient(opts GradientOptions)
```

**Call sites to update:** Internal function only

---

### 20. New (Renderer) - 5 params

**File:** `builder/renderer/renderer.go:46`

**Current:**
```go
func New(compress bool, sink fspkg.ArtifactSink, templateDir string, devMode bool, logger *slog.Logger) *Renderer
```

**Change to:** Merge with `NewWithFs` - use `RendererOptions` struct.

---

### 21. NewBuildContext (5 params)

**File:** `builder/context/context.go:19`

**Current:**
```go
func NewBuildContext(isTesting, isDev, isClean bool, s scheduler.BuildScheduler, l *slog.Logger) *BuildContext
```

**Change to:**
```go
type BuildContextOptions struct {
    IsTesting bool
    IsDev     bool
    IsClean   bool
    Scheduler scheduler.BuildScheduler
    Logger   *slog.Logger
}

func NewBuildContext(opts BuildContextOptions) *BuildContext
```

**Call sites to update:** Find all callers and update.

---

## Implementation Order

1. **High Priority (>7 params):**
   1. CopyFromDiskCache
   2. BuildAssetsEsbuild
   3. GetFrontmatterHashFromValues
   4. hashStandardFields
   5. maybeCopyOriginal

2. **Medium Priority (6-7 params):**
   6. GenerateSW
   7. GenerateManifest
   8. NewWithFs + New (renderer)
   9. GenerateRSS
   10. GenerateGraph
   11. Run (server)

3. **Low Priority (5 params):**
   12. GenerateSitemap
   13. GeneratePWAIcons
   14. CheckWASMFsWithSource
   15. DeployWASMFromFileWithLevel
   16. RenameWithRetry
   17. ParallelWalk
   18. SyncVFS
   19. drawGradient
   20. NewBuildContext

---

## Build Verification Command

After implementing all changes, run:
```bash
go build ./...
go test ./...
```

To verify no call sites were missed.