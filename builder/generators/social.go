package generators

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"

	"github.com/chai2010/webp"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/spf13/afero"
)

const (
	socialCardWidth  = 1200
	socialCardHeight = 630

	marginX       = 80.0
	headerY       = 90.0
	titleStartY   = 180.0
	titleFontSize = 80.0
	descFontSize  = 40.0
	iconSize      = 48.0
	brandFontSize = 28.0
	dateFontSize  = 24.0
)

var (
	fontCache    = make(map[string]*truetype.Font)
	fontMu       sync.RWMutex
	faviconImage image.Image
	faviconOnce  sync.Once
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

// hexToRGBA converts a hex color string to color.RGBA
func hexToRGBA(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.RGBA{0, 0, 0, 255}
	}

	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)

	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

// drawGradient draws a linear gradient on the context
func drawGradient(dc *gg.Context, w, h int, colors []string, angle int) {
	if len(colors) < 2 {
		// If only one color or no colors, use solid background
		bg := "#faf8f5"
		if len(colors) == 1 {
			bg = colors[0]
		}
		dc.SetColor(hexToRGBA(bg))
		dc.Clear()
		return
	}

	// Convert colors
	parsedColors := make([]color.RGBA, len(colors))
	for i, c := range colors {
		parsedColors[i] = hexToRGBA(c)
	}

	// Normalize angle to 0-360
	angle = angle % 360
	if angle < 0 {
		angle += 360
	}

	// Draw gradient as a series of rectangles
	steps := h
	isHorizontal := angle >= 45 && angle < 135 || angle >= 225 && angle < 315
	if !isHorizontal {
		steps = w
	}

	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps-1)

		// Interpolate color
		colorIdx := t * float64(len(parsedColors)-1)
		idx1 := int(colorIdx)
		idx2 := idx1 + 1
		if idx2 >= len(parsedColors) {
			idx2 = len(parsedColors) - 1
		}

		localT := colorIdx - float64(idx1)
		c1 := parsedColors[idx1]
		c2 := parsedColors[idx2]

		r := uint8(float64(c1.R)*(1-localT) + float64(c2.R)*localT)
		g := uint8(float64(c1.G)*(1-localT) + float64(c2.G)*localT)
		b := uint8(float64(c1.B)*(1-localT) + float64(c2.B)*localT)

		dc.SetRGBA(float64(r)/255, float64(g)/255, float64(b)/255, 1)

		if isHorizontal {
			// Draw horizontal strip
			dc.DrawRectangle(0, float64(i), float64(w), 1)
		} else {
			// Draw vertical strip
			dc.DrawRectangle(float64(i), 0, 1, float64(h))
		}
		dc.Fill()
	}
}

// drawDotPattern adds a visible dot pattern overlay
func drawDotPattern(dc *gg.Context, w, h int) {
	// More visible warm brown dots
	dc.SetRGBA255(120, 100, 80, 70) // Warm brown with ~27% opacity

	// Grid spacing
	spacing := 32
	dotRadius := 2.0

	for x := spacing / 2; x < w; x += spacing {
		for y := spacing / 2; y < h; y += spacing {
			dc.DrawCircle(float64(x), float64(y), dotRadius)
			dc.Fill()
		}
	}
}

// GenerateSocialCardToDisk writes directly to a file path on disk
func GenerateSocialCardToDisk(srcFs afero.Fs, cfg *config.SocialCardsConfig, siteTitle, title, description, dateStr, destPath, faviconPath string) error {
	img, err := generateSocialCardImage(srcFs, cfg, siteTitle, title, description, dateStr, faviconPath)
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

// GenerateSocialCard creates a configurable gradient social card.
func GenerateSocialCard(destFs afero.Fs, srcFs afero.Fs, cfg *config.SocialCardsConfig, siteTitle, title, description, dateStr, destPath, faviconPath string) error {
	img, err := generateSocialCardImage(srcFs, cfg, siteTitle, title, description, dateStr, faviconPath)
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

func generateSocialCardImage(srcFs afero.Fs, cfg *config.SocialCardsConfig, siteTitle, title, description, dateStr, faviconPath string) (image.Image, error) {
	dc := gg.NewContext(socialCardWidth, socialCardHeight)

	// --- 1. Draw Gradient Background ---
	allColors := append([]string{cfg.Background}, cfg.Gradient...)
	drawGradient(dc, socialCardWidth, socialCardHeight, allColors, cfg.Angle)

	// --- 2. Draw Dot Pattern Overlay ---
	drawDotPattern(dc, socialCardWidth, socialCardHeight)

	// --- 3. Typography Setup ---
	boldFont := "Inter-Bold.ttf"
	mediumFont := "Inter-Medium.ttf"
	regFont := "Inter-Regular.ttf"

	maxWidth := float64(socialCardWidth) - (marginX * 2)

	textColor := hexToRGBA(cfg.TextColor)
	textColorSecondary := textColor
	// Make secondary text 75% opacity (slightly darker)
	textColorSecondary.A = uint8(float64(textColor.A) * 0.75)

	// --- 4. Header: Logo + Brand (Top Left) ---
	currentX := marginX

	if faviconPath != "" {
		// Use cached favicon if available
		im := getFaviconImage(srcFs, faviconPath)
		if im != nil {
			w := im.Bounds().Dx()
			scale := iconSize / float64(w)

			dc.Push()
			dc.Scale(scale, scale)
			dc.DrawImage(im, int(currentX/scale), int((headerY-35)/scale))
			dc.Pop()

			currentX += iconSize + 20
		}
	}

	if err := setFontFace(dc, boldFont, brandFontSize); err == nil {
		dc.SetColor(textColor)
		dc.DrawString(siteTitle, currentX, headerY)
	}

	// --- 5. Header: Date (Top Right) ---
	if err := setFontFace(dc, mediumFont, dateFontSize); err == nil {
		dc.SetColor(textColor)
		w, _ := dc.MeasureString(dateStr)
		dc.DrawString(dateStr, float64(socialCardWidth)-marginX-w, headerY)
	}

	// --- 6. The Title (Center-Left) ---
	titleLineSpacing := 1.1

	if err := setFontFace(dc, boldFont, titleFontSize); err != nil {
		return nil, fmt.Errorf("failed to load bold font: %w", err)
	}

	dc.SetColor(textColor)
	dc.DrawStringWrapped(title, marginX, titleStartY, 0, 0, maxWidth, titleLineSpacing, gg.AlignLeft)

	titleLines := dc.WordWrap(title, maxWidth)
	titleHeight := float64(len(titleLines)) * titleFontSize * titleLineSpacing

	// --- 7. The Description ---
	if err := setFontFace(dc, regFont, descFontSize); err == nil {
		dc.SetColor(textColorSecondary)
		descY := titleStartY + titleHeight + 25
		dc.DrawStringWrapped(description, marginX, descY, 0, 0, maxWidth, 1.4, gg.AlignLeft)
	}

	return dc.Image(), nil
}
