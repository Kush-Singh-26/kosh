# Build Benchmark Results

This file stores the detailed `kosh clean --cache` benchmark matrix used to choose image concurrency defaults.

## Environment

- Date: 2026-03-08
- OS: Windows
- CPU: 12th Gen Intel(R) Core(TM) i7-1255U
- Site repo: `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`
- Kosh repo: `C:\Users\KIIT0001\blogs`

## Settings Matrix

| vipsConcurrency | imageWorkers | Run | Total Build | Asset copy root/static | Asset building | Parse 39 items | Notes |
|---|---:|---:|---:|---:|---:|---:|---|
| 4 | 8 | 1 | 9.7275337s | 8.21s | 8.38s | 7.55s | bench-v4-w8-run1-20260308-154509.log |
| 4 | 8 | 2 | 9.6776511s | 8.31s | 8.36s | 7.86s | bench-v4-w8-run2-20260308-154519.log |
| 4 | 8 | 3 | 10.578407s | 8.08s | 8.10s | 8.12s | bench-v4-w8-run3-20260308-154530.log |
| 4 | 12 | 1 | 9.7940335s | 6.17s | 6.45s | 8.06s | bench-v4-w12-run1-20260308-154541.log |
| 4 | 12 | 2 | 9.9299249s | 7.45s | 7.53s | 8.39s | bench-v4-w12-run2-20260308-154553.log |
| 4 | 12 | 3 | 10.4678317s | 6.73s | 6.74s | 7.65s | bench-v4-w12-run3-20260308-154604.log |
| 4 | 16 | 1 | 13.901247s | 7.91s | 8.01s | 8.25s | bench-v4-w16-run1-20260308-154615.log |
| 4 | 16 | 2 | 10.4443903s | 8.52s | 8.72s | 8.96s | bench-v4-w16-run2-20260308-154630.log |
| 4 | 16 | 3 | 11.3008552s | 8.03s | 8.65s | 8.97s | bench-v4-w16-run3-20260308-154641.log |
| 6 | 8 | 1 | 9.8290734s | 9.03s | 9.04s | 8.17s | bench-v6-w8-run1-20260308-154654.log |
| 6 | 8 | 2 | 10.2394096s | 7.64s | 7.66s | 8.32s | bench-v6-w8-run2-20260308-154705.log |
| 6 | 8 | 3 | 9.9735086s | 8.05s | 8.07s | 7.87s | bench-v6-w8-run3-20260308-154716.log |
| 6 | 12 | 1 | 11.3369628s | 8.46s | 8.49s | 7.75s | bench-v6-w12-run1-20260308-154727.log |
| 6 | 12 | 2 | 9.5266789s | 7.67s | 7.71s | 7.79s | bench-v6-w12-run2-20260308-154740.log |
| 6 | 12 | 3 | 10.1971584s | 8.75s | 8.95s | 8.47s | bench-v6-w12-run3-20260308-154750.log |
| 6 | 16 | 1 | 9.6173253s | 7.63s | 7.74s | 8.25s | bench-v6-w16-run1-20260308-154801.log |
| 6 | 16 | 2 | 10.8831476s | 9.34s | 9.39s | 9.00s | bench-v6-w16-run2-20260308-154811.log |
| 6 | 16 | 3 | 10.1841744s | 7.85s | 7.97s | 8.40s | bench-v6-w16-run3-20260308-154823.log |
| 8 | 8 | 1 | 10.733472s | 8.92s | 8.92s | 8.01s | bench-v8-w8-run1-20260308-154835.log |
| 8 | 8 | 2 | 9.7703572s | 9.10s | 9.10s | 7.78s | bench-v8-w8-run2-20260308-154847.log |
| 8 | 8 | 3 | 10.001348s | 9.53s | 9.56s | 7.73s | bench-v8-w8-run3-20260308-154858.log |
| 8 | 12 | 1 | 10.8135273s | 9.68s | 9.69s | 7.48s | bench-v8-w12-run1-20260308-154909.log |
| 8 | 12 | 2 | 11.1296144s | 8.17s | 8.24s | 8.71s | bench-v8-w12-run2-20260308-154922.log |
| 8 | 12 | 3 | 10.8584512s | 9.85s | 9.87s | 7.49s | bench-v8-w12-run3-20260308-154934.log |
| 8 | 16 | 1 | 10.6615406s | 9.53s | 9.54s | 7.48s | bench-v8-w16-run1-20260308-154946.log |
| 8 | 16 | 2 | 10.7833888s | 9.81s | 9.83s | 7.67s | bench-v8-w16-run2-20260308-154957.log |
| 8 | 16 | 3 | 10.9116547s | 8.88s | 8.90s | 8.36s | bench-v8-w16-run3-20260308-155009.log |

## Summary

- Best total build: `vipsConcurrency: 6`, `imageWorkers: 12` with `9.5266789s` on the single fastest run, but this combo was less stable across repeats.
- Best root/static time: `vipsConcurrency: 4`, `imageWorkers: 12` with root/static results of `6.17s`, `7.45s`, and `6.73s`.
- Most stable combo: `vipsConcurrency: 4`, `imageWorkers: 8` with total builds of `9.73s`, `9.68s`, and `10.58s`.
- Recommended final config: `imageWorkers: 8`, `vipsConcurrency: 4`
