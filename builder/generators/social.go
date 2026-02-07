package generators

import (
	"fmt"
	"image"
	"image/color"

	"github.com/chai2010/webp"
	"github.com/fogleman/gg"
	"github.com/spf13/afero"
)

// GenerateSocialCard creates an Apple Dark Mode aesthetic Open Graph image.
func GenerateSocialCard(destFs afero.Fs, srcFs afero.Fs, title, description, dateStr, destPath, faviconPath, fontsDir string) error {
	const (
		W = 1200
		H = 630
	)

	dc := gg.NewContext(W, H)

	// --- 1. Canvas & Background (Dark Mode) ---
	// Apple "Midnight" Black
	dc.SetColor(color.RGBA{10, 10, 10, 255})
	dc.Clear()

	// --- 2. Ambient Lighting (The "Aurora" Effect) ---
	// Electric Indigo Glow (Top Left)
	drawDiffuseOrb(dc, 0, 0, 700, color.RGBA{94, 92, 230, 15})

	// Cyan/Teal Glow (Bottom Right)
	drawDiffuseOrb(dc, W, H, 800, color.RGBA{48, 176, 199, 12})

	// --- 3. Typography Setup ---
	// Fonts are read from OS filesystem directly as gg/freetype requires paths usually,
	// or we can assume fontsDir is accessible via OS.
	// If VFS is used for source, we might need to read fonts into bytes if they are virtual.
	// But in this setup, source fonts are real files.
	boldFont := fontsDir + "/Inter-Bold.ttf"
	mediumFont := fontsDir + "/Inter-Medium.ttf"
	regFont := fontsDir + "/Inter-Regular.ttf"

	// Layout Grid
	marginX := 80.0
	headerY := 90.0
	maxWidth := float64(W) - (marginX * 2)

	// --- 4. Header: Logo + Brand (Top Left) ---
	currentX := marginX

	if faviconPath != "" {
		// Load image from SourceFs
		f, err := srcFs.Open(faviconPath)
		if err == nil {
			defer func() { _ = f.Close() }()
			im, _, err := image.Decode(f)
			if err == nil {
				iconSize := 48.0
				w := im.Bounds().Dx()
				scale := iconSize / float64(w)

				dc.Push()
				dc.Scale(scale, scale)
				dc.DrawImage(im, int(currentX/scale), int((headerY-35)/scale))
				dc.Pop()

				currentX += iconSize + 20
			}
		}
	}

	// Brand Name "Kush Blogs"
	if err := dc.LoadFontFace(boldFont, 28); err == nil {
		dc.SetColor(color.RGBA{255, 255, 255, 255})
		dc.DrawString("Kush Blogs", currentX, headerY)
	}

	// --- 5. Header: Date (Top Right) ---
	if err := dc.LoadFontFace(mediumFont, 24); err == nil {
		dc.SetColor(color.RGBA{245, 245, 247, 255})

		// Measure width to align right
		w, _ := dc.MeasureString(dateStr)
		dc.DrawString(dateStr, float64(W)-marginX-w, headerY)
	}

	// --- 6. The Title (Center-Left) ---
	titleFontSize := 80.0
	titleLineSpacing := 1.1

	if err := dc.LoadFontFace(boldFont, titleFontSize); err != nil {
		return fmt.Errorf("failed to load bold font: %w", err)
	}

	// High Contrast White for Title
	dc.SetColor(color.RGBA{245, 245, 247, 255})

	titleY := 280.0
	dc.DrawStringWrapped(title, marginX, titleY, 0, 0, maxWidth, titleLineSpacing, gg.AlignLeft)

	// Calculate title height for description positioning
	titleLines := dc.WordWrap(title, maxWidth)
	titleHeight := float64(len(titleLines)) * titleFontSize * titleLineSpacing

	// --- 7. The Description ---
	if err := dc.LoadFontFace(regFont, 40); err == nil {
		// Secondary Text Color (Light Gray)
		dc.SetColor(color.RGBA{174, 174, 178, 255})

		descY := titleY + titleHeight + 25
		dc.DrawStringWrapped(description, marginX, descY, 0, 0, maxWidth, 1.4, gg.AlignLeft)
	}

	// --- 8. Save to DestFs ---
	// Ensure directory exists
	// filepath.Dir might be "." or "public/..."
	// We need to use destFs.MkdirAll
	// destPath is passed as relative path usually.

	// Create parent dir
	// We assume destPath is clean.
	// But filepath.Dir on "foo.webp" is "."
	// MkdirAll on "." is fine.

	// Issue: "public/static/..." - we need to make sure we strip public/ if destFs is rooted at public,
	// BUT DestFs is likely MemMapFs mounted at root. The path passed in run.go includes "public/".
	// If SyncVFS syncs root of MemFs to current dir, then "public" in MemFs maps to "public" on disk.
	// So we should keep "public/" in the path.

	// filepath.Dir(destPath)

	// We can't use os.MkdirAll. Use destFs.MkdirAll.
	// But gg doesn't have a "SaveToWriter".
	// Wait, gg doesn't have SaveToWriter??
	// dc.SavePNG(path) uses EncodePNG.
	// We can manually encode.

	// Create file in VFS
	// Need to import path/filepath if not imported? (It is not imported in original file)
	// I'll add it.

	// Wait, I can just use webp.Encode as before, passing the writer.

	f, err := destFs.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return webp.Encode(f, dc.Image(), &webp.Options{Lossless: true})

}

// drawDiffuseOrb simulates a gradient mesh/blur
func drawDiffuseOrb(dc *gg.Context, x, y, maxRadius float64, baseColor color.RGBA) {
	dc.Push()

	steps := 100
	r, g, b := int(baseColor.R), int(baseColor.G), int(baseColor.B)

	for i := 0; i < steps; i++ {
		progress := float64(i) / float64(steps)
		radius := maxRadius * (1.0 - progress)

		alpha := float64(baseColor.A) * (1.0 - progress)

		dc.SetRGBA255(r, g, b, int(alpha))
		dc.DrawCircle(x, y, radius)
		dc.Fill()
	}

	dc.Pop()
}
