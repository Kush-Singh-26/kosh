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
	logoCache      sync.Map
	baseImageCache sync.Map
)

const (
	fontCacheSize = 20
	fontDPI       = 72

	hexColorLen  = 6
	hexByteLen   = 2
	hexByteCount = 3
	hexRadix     = 16
	hexByteSize  = 8
	colorZero    = 0
	colorMaxByte = 255
)

func getFontCache() *lru.Cache[string, *truetype.Font] {
	fontCacheOnce.Do(func() {
		var err error
		fontCache, err = lru.New[string, *truetype.Font](fontCacheSize)
		if err != nil {
			panic("failed to create font cache: " + err.Error())
		}
	})
	return fontCache
}

func getLogoImage(fs afero.Fs, path string) image.Image {
	if cached, ok := logoCache.Load(path); ok {
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

	logoCache.Store(path, img)
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
	face := truetype.NewFace(f, &truetype.Options{Size: points, DPI: fontDPI})
	dc.SetFontFace(face)
	return nil
}

// hexToRGBA converts a hex color string to color.RGBA
func hexToRGBA(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != hexColorLen {
		return color.RGBA{colorZero, colorZero, colorZero, colorMaxByte}
	}

	r, _ := strconv.ParseUint(hex[0:hexByteLen], hexRadix, hexByteSize)
	g, _ := strconv.ParseUint(hex[hexByteLen:hexByteLen*2], hexRadix, hexByteSize)
	b, _ := strconv.ParseUint(hex[hexByteLen*2:hexByteLen*hexByteCount], hexRadix, hexByteSize)

	return color.RGBA{uint8(r), uint8(g), uint8(b), colorMaxByte}
}
