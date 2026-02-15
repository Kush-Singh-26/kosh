---
title: "Changelog"
description: "Kosh version history and release notes"
weight: 75
---

# Changelog

All notable changes to Kosh are documented here.

## [v4.0.0] - 2026-02-01

### Added
- BLAKE3 cryptographic hashing for all operations
- Generic worker pools with type safety
- Pre-computed search indexes for faster queries
- Memory pooling for reduced GC pressure

### Changed
- Upgraded to Go 1.23
- Refactored service layer architecture
- Improved version navigation system

### Fixed
- Path normalization on Windows
- Race conditions in cache operations

---

## [v3.0.0] - 2025-06-15

### Added
- Service layer pattern with dependency injection
- Context-based graceful shutdown
- Structured logging with slog

### Changed
- Improved error handling throughout
- Better cache durability settings

See [v3.0 Documentation](./v3.0/index.md) for details.

---

## [v2.0.0] - 2025-01-10

### Added
- Version-aware search engine
- Documentation hub template
- WASM-based client-side search

### Changed
- Sidebar now uses recursive tree structure
- Version selector with client-side navigation

See [v2.0 Documentation](./v2.0/index.md) for details.

---

## [v1.0.0] - 2024-06-01

### Added
- Initial release
- Static site generation
- Markdown support with Goldmark
- Themes: blog and docs
- BoltDB-based caching

See [v1.0 Documentation](./v1.0/index.md) for details.
