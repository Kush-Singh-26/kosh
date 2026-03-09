# Performance Baseline

This document tracks current full-build performance on the reference Windows site repo and records the benchmark-backed recommended image settings.

## Environment

- OS: Windows
- CPU: 12th Gen Intel(R) Core(TM) i7-1255U
- RAM: Not captured
- Site repo: `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`
- Kosh repo: `C:\Users\KIIT0001\blogs`
- Go version: installed `kosh` binary benchmarked after local rebuild

## Commands

```powershell
go test -bench=. -benchmem ./builder/benchmarks/
.
scripts\benchmark_cold_build.ps1 -Runs 3 -WorkingDir 'C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src'
.
scripts\benchmark_cold_build.ps1 -Runs 3 -WorkingDir 'C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src' -CleanCache
.
scripts\benchmark-clean-cache.ps1
.
scripts\benchmark-clean-warm.ps1
```

## Current Conclusions

- Warm full builds (`kosh clean`) are in a good place.
- Cold full builds (`kosh clean --cache`) are still dominated by:
  - `Asset copy root/static`
  - `Parse 39 posts`
- The benchmarked best overall stable image settings on the reference machine are:

```yaml
imageWorkers: 8
```

## Full Matrix Reference

See:

- `docs/build-benchmark-results.md`
- `docs/build-benchmark-results-warm.md`

## Benchmark Summary (2026-03-08)

- Best total single run:
  - `vipsConcurrency: 6`, `imageWorkers: 12`
  - `9.5266789s`
  - not chosen as final default because repeat stability was weaker
- Best root/static-only behavior:
  - `vipsConcurrency: 4`, `imageWorkers: 12`
  - root/static: `6.17s`, `7.45s`, `6.73s`
- Most stable overall combination:
  - `vipsConcurrency: 4`, `imageWorkers: 8`
  - total: `9.73s`, `9.68s`, `10.58s`

## Recommended Config

```yaml
imageWorkers: 8
```

## Recent Full-Build State

Representative recent observations after the latest full-build work:

- `kosh clean --cache`
  - total roughly `10.6s` to `11.1s`
  - expected `0/39` cache hits
  - dominant phases remain:
    - `Asset copy root/static`
    - `Parse 39 posts`
- `kosh clean`
  - roughly `1.5s`
  - `39/39` cache hits
  - warm-cache performance is strong

## Interpretation

- further cold-build gains are now mostly constrained by real image work and markdown parsing work
- warm builds benefited significantly from image/cache-path and publish reliability improvements
- Windows publish reliability is materially improved after unique staging/backup dir work

## Acceptance Criteria

Any future performance optimization should preserve:

- no source-tree mutations
- no stale asset hashes in HTML
- no search runtime/index schema mismatch
- no broken `.webp` rewriting for eligible local raster images
- no partial-output publish state on clean builds

## Output Parity

```powershell
.
scripts\compare_build_outputs.ps1 -Left .\baseline-output -Right .\candidate-output
```

## Notes

- Do not change the recommended config in docs unless a newer benchmark matrix proves a better stable combination.
- If benchmark methodology changes, update `docs/build-benchmark-results.md` and this file together.
