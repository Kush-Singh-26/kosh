# Performance Baseline

Use this document to store pre-refactor cold-build measurements and parity checks.

## Environment
- OS: Windows
- CPU: 12th Gen Intel(R) Core(TM) i7-1255U
- RAM: Not captured
- Go version: installed `kosh` binary benchmarked after local rebuild
- libvips version: runtime initialized with `concurrency=8`
- Repository commit: not recorded in benchmark output
- Site working directory: `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`

## Datasets
- Small: `100` posts
- Medium: `1k` posts
- Large: `10k` posts
- Variants: image-heavy, math-heavy, mixed

## Commands
```powershell
go test -bench=. -benchmem ./builder/benchmarks/
./scripts/benchmark_cold_build.ps1 -Runs 3 -WorkingDir 'C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src'
./scripts/benchmark_cold_build.ps1 -Runs 3 -WorkingDir 'C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src' -CleanCache
```

## Current Results (Historical Claim From Earlier Run)
| Scenario | Run | Total ms | Notes |
| --- | --- | ---: | --- |
| warm-cache cold-output | 1 | ~725 | `39/39` cache hits, hardlinks, parallel search |
| clean-cache cold build | 1 | ~9200 | `0/39` cache hits |

Warm-cache average: `~725ms` (stable from Phase 5 - search generation parallelized but site is small)

Clean-cache average: `~9200ms` (stable)

## Phase Timing Snapshot (Post Phase 6)
- Warm-cache representative run:
- Asset building: `280ms`
- Parse: `200ms`
- Render: `174ms`
- Search index generation: `132ms` (parallelized)
- Graph and metadata: `140ms`
- Pagination: `47ms`
- Tags rendering: `50ms`
- PWA generation: `28ms`
- Publish: `75ms`

- Clean-cache representative run:
- Asset building: `3.15s`
- Parse: `3.91s`
- Render: `118ms`
- Search index generation: `203ms`
- Graph and metadata: `208ms`
- Pagination: `388ms`
- Tags rendering: `996ms`
- PWA generation: `229ms`
- Publish: `92ms`

## Latest Verification Run (2026-03-06)
These are the latest installed-binary checks run against `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`.

| Scenario | Run | Total ms | Notes |
| --- | --- | ---: | --- |
| warm-cache cold-output | 1 | ~1957 | `39/39` cache hits |
| clean-cache cold build | 1 | ~13697 | `0/39` cache hits |

Warm-cache phase snapshot:
- Asset building: `368.07ms`
- Parse 39 posts: `247.11ms`
- Render 39 pages: `473.72ms`
- Search index generation: `318.02ms`
- Graph and metadata: `380.65ms`
- Pagination: `209.41ms`
- Tags rendering: `179.27ms`
- PWA generation: `139.03ms`
- Publish: `317.62ms`

Clean-cache phase snapshot:
- Asset building: `3.90s`
- Parse 39 posts: `6.69s`
- Render 39 pages: `602.98ms`
- Search index generation: `522.78ms`
- Graph and metadata: `597.20ms`
- Pagination: `882.92ms`
- Tags rendering: `1.45s`
- PWA generation: `457.36ms`
- Publish: `186.74ms`

Conclusion from latest verification:
- Publish is materially improved versus the original baseline, so the staging/sink work appears real.
- Parse and asset phases still dominate clean builds.
- BoltDB is not separately timed, so no DB migration conclusion should be made from these numbers alone.

## Latest Verification Run With Direct DB Timing (2026-03-06)
These runs include explicit timing around `BatchCommit(...)`.

| Scenario | Run | Total ms | Notes |
| --- | --- | ---: | --- |
| warm-cache cold-output | 1 | ~968 | `39/39` cache hits |
| clean-cache cold build | 1 | ~11629 | `0/39` cache hits |

Warm-cache phase snapshot:
- Asset building: `478.74ms`
- Parse 39 posts: `24.98ms`
- Render 39 pages: `551.20us`
- Search index generation: `275.52ms`
- Graph and metadata: `279.50ms`
- Tags rendering: `126.93ms`
- Pagination: `57.27ms`
- PWA generation: `6.62ms`
- Publish: `112.42ms`
- Cache commit: not present because no cache writes were needed

Clean-cache phase snapshot:
- Asset building: `3.39s`
- Parse 39 posts: `3.96s`
- Render 39 pages: `611.24ms`
- Cache commit: `55.71ms`
- Search index generation: `795.21ms`
- Graph and metadata: `879.20ms`
- Pagination: `1.20s`
- Tags rendering: `1.90s`
- PWA generation: `556.69ms`
- Publish: `263.37ms`

Conclusion from DB-timed verification:
- On the current real site, BoltDB cache commit time is small (`55.71ms`) relative to parse/assets/tags.
- DB backend migration is not justified by this dataset.
- Recheck this conclusion on larger synthetic datasets before permanently closing the migration option.

## Output Parity
```powershell
./scripts/compare_build_outputs.ps1 -Left .\baseline-output -Right .\candidate-output
```

## Acceptance Notes
- No missing files.
- No hash mismatches.
- No broken URLs or asset regressions observed.
