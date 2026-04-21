# Kosh Build Tuning & Performance

Kosh is designed for high-performance builds, but advanced users can further tune its behavior using the `kosh.build.yaml` file.

## 1. The `kosh.build.yaml` File
Create this file in your project root (alongside `kosh.yaml`) to override internal build parameters.

### Default Tuning Parameters
```yaml
# Worker Pools
maxWorkers: 32       # Global maximum worker pool size
defaultWorkers: 12   # Default workers for non-intensive tasks

# Buffer & File Limits
maxBufferSize: 65536         # Max buffer size for memory pools (bytes)
maxFileSize: 52428800        # Max file size to load in memory (default: 50MB)
inlineHTMLThreshold: 32768   # Size threshold for inline fragment storage

# Timeouts & Watcher
debounceDuration: 500ms      # Debounce duration for file changes
cacheDBTimeout: 10s          # Timeout for BoltDB cache operations

# Search Scoring (BM25)
scoreTitleMatch: 10.0
scoreTagMatch: 5.0
scorePhraseMatch: 15.0
maxSearchResults: 100
```

---

## 2. Performance Optimizations

### Fragment Caching
Kosh caches shared UI components (Navbars, Footers) as pre-rendered HTML fragments in BoltDB. These are re-used across builds unless a "Clean Build" (`kosh clean --cache`) is triggered.

### Image Priority Queue
During full builds, Kosh sorts images by file size and processes larger images first. This ensures that the most time-consuming tasks start early, minimizing the "fat-tail" wait time at the end of a build.

### Atomic Staging
By default, Kosh builds the site into a temporary `staging` directory and performs an atomic rename once the build succeeds. This prevents users from seeing a broken or partial site during the build process.
