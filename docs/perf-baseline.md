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

## Current Results (Post Phase 1)
| Scenario | Run | Total ms | Notes |
| --- | --- | ---: | --- |
| warm-cache cold-output | 1 | ~1250 | `39/39` cache hits |
| clean-cache cold build | 1 | ~10000 | `0/39` cache hits |

Warm-cache average: `~1250ms` (down from `1334ms`)

Clean-cache average: `~10000ms` (down from `22916ms` - **~56% improvement!**)

## Phase Timing Snapshot (Post Phase 1)
- Warm-cache representative run:
- Asset building: `966ms`
- Parse: `28ms`
- Render: `<1ms`
- Search index generation: `187ms`
- Graph and metadata: `191ms`
- Pagination: `103ms`
- Tags rendering: `74ms`
- PWA generation: `6ms`
- Sync/publish: `6ms` (down from `147-274ms`)

- Clean-cache representative run:
- Asset building: `3.86s`
- Parse: `4.17s`
- Render: `98ms`
- Search index generation: `242ms`
- Graph and metadata: `255ms`
- Pagination: `576ms`
- Tags rendering: `1.09s`
- PWA generation: `217ms`
- Sync/publish: `2.8ms` (down from `737ms-3.14s`)

## Output Parity
```powershell
./scripts/compare_build_outputs.ps1 -Left .\baseline-output -Right .\candidate-output
```

## Acceptance Notes
- No missing files.
- No hash mismatches.
- No broken URLs or asset regressions observed.
