package generators

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/assets"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
)

var (
	fontCache      *lru.Cache[string, *truetype.Font]
	fontCacheOnce  sync.Once
	faviconCache   sync.Map
	baseImageCache sync.Map
)

func getFontCache() *lru.Cache[string, *truetype.Font] {
	fontCacheOnce.Do(func() {
		var err error
		fontCache, err = lru.New[string, *truetype.Font](20)
		if err != nil {
			panic("failed to create font cache: " + err.Error())
		}
	})
	return fontCache
}

func getFaviconImage(fs afero.Fs, path string) image.Image {
	if cached, ok := faviconCache.Load(path); ok {
		if img, ok := cached.(image.Image); ok {
			return img
		}
	}

	f, err := fs.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}

	faviconCache.Store(path, img)
	return img
}

func loadFont(name string) (*truetype.Font, error) {
	cache := getFontCache()
	if f, ok := cache.Get(name); ok {
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
	fontCache.Add(name, f)
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
