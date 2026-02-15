---
title: "Installation"
description: "Install Kosh on your system"
weight: 95
---

# Installation

This guide covers installing Kosh on macOS, Linux, and Windows.

## Requirements

- **Go 1.23+** (for building from source)
- **Git** (for version control)

## Install from Source

The recommended way to install Kosh is from source:

```bash
go install github.com/kosh/kosh@latest
```

This will install the `kosh` binary to your `$GOPATH/bin` directory.

## Verify Installation

Check that Kosh is installed correctly:

```bash
kosh version
```

You should see output like:

```
Kosh v4.0.0
Built with Go 1.23
Features: BLAKE3, Generics, Memory Pools
```

## Platform-Specific Notes

### macOS

```bash
brew install go
go install github.com/kosh/kosh@latest
```

### Linux

```bash
sudo apt install golang-go
go install github.com/kosh/kosh@latest
```

### Windows

```powershell
winget install GoLang.Go
go install github.com/kosh/kosh@latest
```

## Next Steps

- [Configuration](./configuration.md) - Configure your site
- [Getting Started](./getting-started.md) - Build your first site
- [Tutorial](./tutorial.md) - Step-by-step guide
