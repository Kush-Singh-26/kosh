---
title: "Why Go is Great for Tooling"
date: "2025-10-02"
slug: "go-for-tooling"
---

I chose Go for this project for several reasons, which make it an excellent language for building command-line tools like a static site generator.

1.  **Fast Compilation**: Go compiles incredibly fast, which makes the development cycle quick and enjoyable.
2.  **Single Binary**: It compiles down to a single, dependency-free binary. This makes deployment trivial. You just copy one file!
3.  **Strong Standard Library**: The `html/template`, `os`, and `path/filepath` packages provide everything needed for this project without reaching for many third-party libraries.
4.  **Concurrency**: While not used heavily here, Go's built-in support for concurrency would make it easy to extend this generator to process files in parallel, speeding it up significantly for larger sites.