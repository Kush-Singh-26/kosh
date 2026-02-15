---
title: "SDK"
description: "Kosh SDK documentation"
weight: 40
---

# SDK

Extend Kosh with the Go SDK.

## Installation

```bash
go get github.com/kosh/kosh
```

## Basic Usage

```go
package main

import (
    "github.com/kosh/kosh/builder/config"
    "github.com/kosh/kosh/builder/run"
)

func main() {
    cfg, err := config.Load("kosh.yaml")
    if err != nil {
        panic(err)
    }
    
    builder := run.NewBuilder(cfg)
    if err := builder.Build(); err != nil {
        panic(err)
    }
}
```

## Service Layer

Kosh uses a service layer pattern:

```go
type Builder struct {
    cacheService  services.CacheService
    postService   services.PostService
    assetService  services.AssetService
    renderService services.RenderService
}
```

### Cache Service

```go
// Get cached item
item, err := cacheService.Get("posts", "my-post")

// Set cached item
err := cacheService.Set("posts", "my-post", data)

// Clear cache
err := cacheService.Clear()
```

### Post Service

```go
// Parse markdown
post, err := postService.Parse("content/my-post.md")

// Render HTML
html, err := postService.Render(post)
```

## Worker Pools

Use generic worker pools:

```go
pool := utils.NewWorkerPool(ctx, numWorkers, func(task MyTask) {
    // Process task
})
pool.Start()
pool.Submit(task)
pool.Stop()
```

## Memory Pools

Reuse buffers:

```go
buf := utils.SharedBufferPool.Get()
defer utils.SharedBufferPool.Put(buf)

buf.WriteString("content")
result := buf.String()
```

## Custom Extensions

Create Goldmark extensions:

```go
import "github.com/yuin/goldmark"

md := goldmark.New(
    goldmark.WithExtensions(
        MyExtension{},
    ),
)
```

## Related

- [API Reference](./reference.md) - Full API docs
- [API Examples](./examples.md) - Code examples
- [Changelog](../changelog.md) - Version history
