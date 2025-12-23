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
```

- To build the site

```bash
.\bin\builder.exe
```

- To start the server

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

- To create a new file

```bash
go run new.go "<title>"
```

---

## `builder` structure

```txt
builder/
├── config/
│   └── config.go          # Configuration & Flags
├── generators/
│   ├── graph.go           # JSON Graph generation
│   ├── rss.go             # RSS Feed generation
│   └── sitemap.go         # XML Sitemap generation
├── models/
│   └── models.go          # Shared Data Structures (Structs)
├── parser/
│   └── parser.go          # Markdown parsing & URL transforming
├── renderer/
│   └── renderer.go        # HTML Templates & Rendering
├── utils/
│   └── utils.go           # File copying & Image processing
└── main.go                # Main Entry Point (Orchestrator)
```

## SSG Structure

```txt
.
├── .air.toml              # Live-reloading configuration
├── .github/
│   └── workflows/
│       └── deploy.yml     # GitHub Actions CI/CD pipeline
├── .gitignore             # Git ignore rules
├── README.md              
├── builder/               # SSG logic
├── content/               # Markdown Source Files
│   ├── _index.md          # Homepage content
│   ├── 404.md             # 404 Error page content
│   ├── CV-*.md            # Computer Vision blog posts
│   ├── NLP-*.md           # NLP blog posts
│   ├── NN-*.md            # Neural Network blog posts
│   └── ...                # Other posts
├── go.mod                 # Go module definition
├── go.sum                 # Go dependencies checksums
├── new.go                 # Helper script to make new posts
├── server/                # Local Development Server
│   └── main.go            # HTTP server for previewing 'public/'
├── static/                # Static Assets (copied to public/)
│   ├── css/               # Stylesheets (layout, theme, fonts)
│   ├── images/            # Blog images and diagrams
│   ├── js/                # Scripts (math rendering, graph, latex)
│   ├── robots.txt         # Search engine instructions
│   └── wasm/              # WebAssembly binaries and glue code
└── templates/             # HTML Templates
    ├── graph.html         # Template for the Knowledge Graph page
    └── layout.html        # Main master template for all pages
```