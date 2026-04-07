package generators

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/models"

	"github.com/chai2010/webp"
	"github.com/fogleman/gg"
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

// SocialCardHash generates a stable hash for social card content
func SocialCardHash(title, description string) string {
	cardContent := fmt.Sprintf("%s|%s", title, description)
	return cache.HashString(cardContent)
}

// ShouldGenerateSocialCard determines if a social card needs generation
func ShouldGenerateSocialCard(cache models.SocialCardCache, cacheKey, currentHash, cachedCardPath string, force bool) bool {
	if force {
		return true
	}
	if _, err := os.Stat(cachedCardPath); os.IsNotExist(err) {
		return true
	}
	if cache != nil {
		storedHash, _ := cache.GetSocialCardHash(cacheKey)
		return storedHash != currentHash
	}
	return false
}

// ProvideSocialCardOptions holds parameters for ProvideSocialCard
type ProvideSocialCardOptions struct {
	Sink        models.ArtifactSink
	Cache       models.SocialCardCache
	SourceFs    afero.Fs
	OutputDir   string
	CacheDir    string
	Title       string // Site title
	DestPath    string
	CacheKey    string
	CardTitle   string
	Description string
	Badge       string
	Force       bool
	SocialCfg   *models.SocialCardsConfig
	Render      models.RenderService
	LogoPath    string
}

// ProvideSocialCard ensures a social card exists in the VFS, using cache if possible
func ProvideSocialCard(opts ProvideSocialCardOptions) {
	currentHash := SocialCardHash(opts.CardTitle, opts.Description)
	cachedCardPath := filepath.Join(opts.CacheDir, "social-cards", currentHash+".webp")

	needsGen := ShouldGenerateSocialCard(opts.Cache, opts.CacheKey, currentHash, cachedCardPath, opts.Force)

	buildCtx.IgnoreError(opts.Sink.MkdirAll(filepath.Dir(opts.DestPath)), "ensure social card dir in VFS")

	if needsGen {
		buildCtx.IgnoreError(os.MkdirAll(filepath.Dir(cachedCardPath), 0755), "ensure social card dir in cache")

		err := GenerateSocialCardToDisk(SocialCardOptions{
			SrcFs:       opts.SourceFs,
			Cfg:         opts.SocialCfg,
			SiteTitle:   opts.Title,
			Title:       opts.CardTitle,
			Description: opts.Description,
			DateStr:     opts.Badge,
			DestPath:    cachedCardPath,
			LogoPath:    opts.LogoPath,
		})
		if err != nil {
			return
		}
		if opts.Cache != nil {
			buildCtx.IgnoreError(opts.Cache.SetSocialCardHash(opts.CacheKey, currentHash), "update social card hash")
		}
	}

	data, err := os.ReadFile(cachedCardPath)
	if err == nil {
		buildCtx.IgnoreError(opts.Sink.WriteFile(opts.DestPath, data), "write social card to VFS")
		opts.Render.RegisterFile(opts.DestPath)
	}
}

// SocialCardOptions contains parameters for social card generation.
type SocialCardOptions struct {
	Sink        models.ArtifactSink
	SrcFs       afero.Fs
	Cfg         *models.SocialCardsConfig
	SiteTitle   string
	Title       string
	Description string
	DateStr     string
	DestPath    string
	LogoPath    string
}

// GenerateSocialCardToDisk writes directly to a file path on disk
func GenerateSocialCardToDisk(opts SocialCardOptions) error {
	img, err := generateSocialCardImage(opts)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 85}); err != nil {
		return err
	}

	return os.WriteFile(opts.DestPath, buf.Bytes(), 0644)
}

// GenerateSocialCard creates a configurable gradient social card.
func GenerateSocialCard(opts SocialCardOptions) error {
	img, err := generateSocialCardImage(opts)
	if err != nil {
		return err
	}

	return opts.Sink.WriteStream(opts.DestPath, func(w io.Writer) error {
		return webp.Encode(w, img, &webp.Options{Lossless: false, Quality: 85})
	})
}

func getBaseSocialCardImage(opts SocialCardOptions) *image.RGBA {
	cacheKey := fmt.Sprintf("%s|%s|%s|%d|%s", opts.SiteTitle, opts.LogoPath, opts.Cfg.Background, opts.Cfg.Angle, strings.Join(opts.Cfg.Gradient, ","))
	if cached, ok := baseImageCache.Load(cacheKey); ok {
		return cached.(*image.RGBA)
	}

	dc := gg.NewContext(socialCardWidth, socialCardHeight)

	// --- 1. Draw Gradient Background ---
	allColors := append([]string{opts.Cfg.Background}, opts.Cfg.Gradient...)
	drawGradient(dc, socialCardWidth, socialCardHeight, allColors, opts.Cfg.Angle)

	// --- 2. Draw Dot Pattern Overlay ---
	drawDotPattern(dc, socialCardWidth, socialCardHeight)

	boldFont := "Inter-Bold.ttf"
	textColor := hexToRGBA(opts.Cfg.TextColor)

	// --- 3. Header: Logo + Brand (Top Left) ---
	currentX := marginX

	if opts.LogoPath != "" {
		// Use cached logo if available
		im := getLogoImage(opts.SrcFs, opts.LogoPath)
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
		dc.DrawString(opts.SiteTitle, currentX, headerY)
	}

	// We know Image() from gg context returns an *image.RGBA
	baseImg := dc.Image().(*image.RGBA)
	baseImageCache.Store(cacheKey, baseImg)
	return baseImg
}

func generateSocialCardImage(opts SocialCardOptions) (image.Image, error) {
	baseImg := getBaseSocialCardImage(opts)

	// Clone the base image
	clonedImg := image.NewRGBA(baseImg.Bounds())
	copy(clonedImg.Pix, baseImg.Pix)

	dc := gg.NewContextForRGBA(clonedImg)

	// --- Typography Setup ---
	boldFont := "Inter-Bold.ttf"
	mediumFont := "Inter-Medium.ttf"
	regFont := "Inter-Regular.ttf"

	maxWidth := float64(socialCardWidth) - (marginX * 2)

	textColor := hexToRGBA(opts.Cfg.TextColor)
	textColorSecondary := textColor
	// Make secondary text 75% opacity (slightly darker)
	textColorSecondary.A = uint8(float64(textColor.A) * 0.75)

	// --- Header: Date (Top Right) ---
	if err := setFontFace(dc, mediumFont, dateFontSize); err == nil {
		dc.SetColor(textColor)
		w, _ := dc.MeasureString(opts.DateStr)
		dc.DrawString(opts.DateStr, float64(socialCardWidth)-marginX-w, headerY)
	}

	// --- The Title (Center-Left) ---
	titleLineSpacing := 1.1

	if err := setFontFace(dc, boldFont, titleFontSize); err != nil {
		return nil, fmt.Errorf("failed to load bold font: %w", err)
	}

	dc.SetColor(textColor)
	dc.DrawStringWrapped(opts.Title, marginX, titleStartY, 0, 0, maxWidth, titleLineSpacing, gg.AlignLeft)

	titleLines := dc.WordWrap(opts.Title, maxWidth)
	titleHeight := float64(len(titleLines)) * titleFontSize * titleLineSpacing

	// --- The Description ---
	if err := setFontFace(dc, regFont, descFontSize); err == nil {
		dc.SetColor(textColorSecondary)
		descY := titleStartY + titleHeight + 25
		dc.DrawStringWrapped(opts.Description, marginX, descY, 0, 0, maxWidth, 1.4, gg.AlignLeft)
	}

	return dc.Image(), nil
}
