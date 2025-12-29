# GO SSG

A static site generator (ssg) in golang.

- To build the binaries :

```bash
# Create a 'bin' directory to keep things organized 
mkdir bin 

# Compile the builder
go build -o bin/builder.exe ./builder

# Compile the server
go build -o bin/server.exe ./server/main.go

# Create new file
go build -o bin/new.exe new.go
```

- To build the site without compression

```bash
.\bin\builder.exe
```

- To build the site with compression (Minifies HTML/CSS/JS and converts images to WebP)

```bash
.\bin\builder.exe -compress
```

- To run the local server

```bash
.\bin\server.exe
```

## For continuous reloading :

**Terminal 1** :

```bash
air
```

**Terminal 2** :

```bash
.\bin\server.exe
```

---

## Content Management

- To create a new file

```bash
.\bin\new.exe "<title>"
```

- Draft System

```yaml
title: "draft post"
date: "2025-04-26"
draft: true
```

Drafts are automatically skipped during the build process.

---

## Features

- **Image Optimization** : Automatically converts local images (PNG/JPG) to WebP and resizes images larger than 1200px when using the -compress flag.

- **Minification** : Minifies HTML, CSS, and JavaScript files during production builds.

- **Incremental Builds** : intelligently skips processing files that haven't changed to speed up build times.

- **Math Support** : Renders LaTeX equations using KaTeX.

- **Knowledge Graph** : Generates an interactive graph of connected tags and posts.

## SSG Structure

```txt
.
├── .air.toml              # Live-reloading configuration
├── .github/
│   └── workflows/
│       └── deploy.yml     # GitHub Actions CI/CD pipeline
├── .gitignore             # Git ignore rules
├── README.md              
├── builder/               # SSG Core Logic
│   ├── config/            # Configuration & Flags
│   ├── generators/        # RSS, Sitemap, & Graph generation
│   ├── models/            # Shared Data Structures
│   ├── parser/            # Markdown parsing & URL transforming
│   ├── renderer/          # HTML Templates & Rendering
│   ├── utils/             # Minification, Image Processing, & File Ops
│   └── main.go            # Main Entry Point
├── content/               # Markdown Source Files
├── go.mod                 # Go module definition
├── new.go                 # Helper script to create posts
├── server/                # Local Development Server
├── static/                # Static Assets
│   ├── css/               # Stylesheets (layout, theme variables)
│   ├── images/            # Source images
│   ├── js/                # Scripts (graph, math)
│   └── wasm/              # WebAssembly binaries
└── templates/             # HTML Templates
    ├── graph.html         # Knowledge Graph page template
    └── layout.html        # Master layout template
```