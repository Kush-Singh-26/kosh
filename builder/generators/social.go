package generators

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"sync"

	"my-ssg/builder/assets"

	"github.com/chai2010/webp"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/spf13/afero"
)

var (
	fontCache     = make(map[string]*truetype.Font)
	fontMu        sync.RWMutex
	baseCardImage image.Image
	baseCardOnce  sync.Once
	faviconImage  image.Image
	faviconOnce   sync.Once
)

func getFaviconImage(fs afero.Fs, path string) image.Image {
	faviconOnce.Do(func() {
		f, err := fs.Open(path)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		img, _, err := image.Decode(f)
		if err == nil {
			faviconImage = img
		}
	})
	return faviconImage
}

func loadFont(name string) (*truetype.Font, error) {
	fontMu.RLock()
	if f, ok := fontCache[name]; ok {
		fontMu.RUnlock()
		return f, nil
	}
	fontMu.RUnlock()

	fontMu.Lock()
	defer fontMu.Unlock()

	if f, ok := fontCache[name]; ok {
		return f, nil
	}

	data, err := assets.GetFont(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load font %s: %w", name, err)
	}
	f, err := truetype.Parse(data)
	if err != nil {
		return nil, err
	}
	fontCache[name] = f
	return f, nil
}

func setFontFace(dc *gg.Context, fontPath string, points float64) error {
	f, err := loadFont(fontPath)
	if err != nil {
		return err
	}
	face := truetype.NewFace(f, &truetype.Options{Size: points, DPI: 72})
	dc.SetFontFace(face)
	return nil
}

// GenerateSocialCardToDisk writes directly to a file path on disk
func GenerateSocialCardToDisk(srcFs afero.Fs, title, description, dateStr, destPath, faviconPath string) error {
	img, err := generateSocialCardImage(srcFs, title, description, dateStr, faviconPath)
	if err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return webp.Encode(f, img, &webp.Options{Lossless: false, Quality: 85})
}

// GenerateSocialCard creates an Apple Dark Mode aesthetic Open Graph image.
func GenerateSocialCard(destFs afero.Fs, srcFs afero.Fs, title, description, dateStr, destPath, faviconPath string) error {
	img, err := generateSocialCardImage(srcFs, title, description, dateStr, faviconPath)
	if err != nil {
		return err
	}

	f, err := destFs.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return webp.Encode(f, img, &webp.Options{Lossless: false, Quality: 85})
}

func getBaseCardImage(w, h int) image.Image {
	baseCardOnce.Do(func() {
		dc := gg.NewContext(w, h)
		// --- 1. Canvas & Background (Dark Mode) ---
		dc.SetColor(color.RGBA{10, 10, 10, 255})
		dc.Clear()

		// --- 2. Ambient Lighting (The "Aurora" Effect) ---
		drawDiffuseOrb(dc, 0, 0, 700, color.RGBA{94, 92, 230, 15})
		drawDiffuseOrb(dc, float64(w), float64(h), 800, color.RGBA{48, 176, 199, 12})
		baseCardImage = dc.Image()
	})
	return baseCardImage
}

func generateSocialCardImage(srcFs afero.Fs, title, description, dateStr, faviconPath string) (image.Image, error) {
	const (
		W = 1200
		H = 630
	)

	dc := gg.NewContext(W, H)

	// Draw pre-rendered background
	dc.DrawImage(getBaseCardImage(W, H), 0, 0)

	// --- 3. Typography Setup ---
	boldFont := "Inter-Bold.ttf"
	mediumFont := "Inter-Medium.ttf"
	regFont := "Inter-Regular.ttf"

	marginX := 80.0
	headerY := 90.0
	maxWidth := float64(W) - (marginX * 2)

	// --- 4. Header: Logo + Brand (Top Left) ---
	currentX := marginX

	if faviconPath != "" {
		// Use cached favicon if available
		im := getFaviconImage(srcFs, faviconPath)
		if im != nil {
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

	if err := setFontFace(dc, boldFont, 28); err == nil {
		dc.SetColor(color.RGBA{255, 255, 255, 255})
		dc.DrawString("Kush Blogs", currentX, headerY)
	}

	// --- 5. Header: Date (Top Right) ---
	if err := setFontFace(dc, mediumFont, 24); err == nil {
		dc.SetColor(color.RGBA{245, 245, 247, 255})
		w, _ := dc.MeasureString(dateStr)
		dc.DrawString(dateStr, float64(W)-marginX-w, headerY)
	}

	// --- 6. The Title (Center-Left) ---
	titleFontSize := 80.0
	titleLineSpacing := 1.1

	if err := setFontFace(dc, boldFont, titleFontSize); err != nil {
		return nil, fmt.Errorf("failed to load bold font: %w", err)
	}

	dc.SetColor(color.RGBA{245, 245, 247, 255})
	titleY := 280.0
	dc.DrawStringWrapped(title, marginX, titleY, 0, 0, maxWidth, titleLineSpacing, gg.AlignLeft)

	titleLines := dc.WordWrap(title, maxWidth)
	titleHeight := float64(len(titleLines)) * titleFontSize * titleLineSpacing

	// --- 7. The Description ---
	if err := setFontFace(dc, regFont, 40); err == nil {
		dc.SetColor(color.RGBA{174, 174, 178, 255})
		descY := titleY + titleHeight + 25
		dc.DrawStringWrapped(description, marginX, descY, 0, 0, maxWidth, 1.4, gg.AlignLeft)
	}

	return dc.Image(), nil
}

// drawDiffuseOrb simulates a gradient mesh/blur
// kept for initial generation of base image
func drawDiffuseOrb(dc *gg.Context, x, y, maxRadius float64, baseColor color.RGBA) {
	dc.Push()

	// Optimized: Reduced steps from 100 to 30
	steps := 30
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
