# Libvips Integration Plan

**Objective:** Replace the current native Go image processing pipeline (which uses `image`, `golang.org/x/image/draw`, and `chai2010/webp`) entirely with `libvips` via the `github.com/h2non/bimg` package to leverage C-level SIMD optimizations and drastically reduce cold-build times.

*Note: Per requirements, this plan removes the pure-Go fallback and makes `libvips` and CGO a strict requirement for compiling and running `kosh`.*

## Step 1: Install Dependencies
1. Ensure the host system has `libvips` installed (e.g., `apt-get install libvips-dev` on Linux, `brew install vips` on macOS).
2. Enable CGO in the Go environment: `export CGO_ENABLED=1`.
3. Add the `bimg` dependency:
   ```bash
   go get github.com/h2non/bimg
   ```

## Step 2: Refactor `builder/utils/fs_copy.go`
Locate the `processImageVFS` function and replace the pure-Go decoding, scaling, and encoding logic with `bimg`.

**Logic changes in `processImageVFS`:**
1. **Remove Old Imports:** Remove `image`, `image/jpeg`, `image/png`, `golang.org/x/image/draw`, and `github.com/chai2010/webp`.
2. **Read File:** Instead of using a streamed `bufio.Reader` for `image.Decode`, read the entire file into memory using `afero.ReadFile(srcFs, srcPath)`.
3. **Initialize `bimg`:** 
   ```go
   imgData, err := afero.ReadFile(srcFs, srcPath)
   // handle err
   img := bimg.NewImage(imgData)
   size, err := img.Size()
   ```
4. **Configure Options:**
   ```go
   opts := bimg.Options{
       Type:    bimg.WEBP,
       Quality: webpQuality,
   }
   ```
5. **Apply Resizing Logic:** Keep the `1200` width constraint.
   ```go
   if size.Width > 1200 && !skipResize {
       opts.Width = 1200
       opts.Height = (size.Height * 1200) / size.Width
   } else if skipResize && !isNil(m) {
       m.RecordImageResizeSkipped()
   }
   ```
6. **Process & Encode:**
   ```go
   encodedData, err := img.Process(opts)
   // handle err
   ```
7. **Cache & Write:** Use the returned `encodedData` byte slice to write directly to the memory cache, disk cache, and the final `ArtifactSink`.
8. **Cleanup Memory Pools:** Remove `SharedLargeBufferPool` and `rgbaPixPool` from `fs_copy.go` as `libvips` handles its own memory allocation via C.

## Step 3: Cleanup `go.mod`
Run `go mod tidy` to remove the newly orphaned native Go image processing packages.

## Step 4: Update Documentation
Update `AGENTS.md` and the project `README.md` to state that `libvips` is now a mandatory system dependency for Kosh and CGO must be enabled. Update CI/CD pipelines to install `libvips-dev` before running `go build`.