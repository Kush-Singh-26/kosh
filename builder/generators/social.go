package generators

import (
	"fmt"
	"image/color"
	"os"

	"github.com/chai2010/webp"
	"github.com/fogleman/gg"
)

// GenerateSocialCard creates an Open Graph image for a blog post.
// destPath: where to save the webp (e.g., public/static/images/cards/posts/hello.webp)
// fontsDir: path to directory containing .ttf files (e.g., builder/assets/fonts)
func GenerateSocialCard(title, description, dateStr, destPath, faviconPath, fontsDir string) error {
	const (
		W = 1200
		H = 630
	)

	dc := gg.NewContext(W, H)

	// 1. Background (Very Light Cool Gray)
	dc.SetColor(color.RGBA{248, 249, 250, 255})
	dc.Clear()

	// 2. Add "Interesting" Visual Elements (Background Shapes)
	dc.SetColor(color.RGBA{230, 230, 235, 255}) // Slightly darker than BG
	
	// Circle 1: Top Right (bleeding out)
	dc.DrawCircle(W, 0, 300)
	dc.Fill()

	// Circle 2: Bottom Right (large)
	dc.DrawCircle(W-200, H, 400)
	dc.Fill()

	// Circle 3: Center Left (small accent)
	dc.DrawCircle(100, 300, 50)
	dc.Fill()

	// 3. Accent Strip (Left Side)
	dc.SetColor(color.RGBA{40, 40, 40, 255}) // Almost Black
	dc.DrawRectangle(0, 0, 20, H)
	dc.Fill()

	// 4. Load Fonts
	boldFont := fontsDir + "/Inter-Bold.ttf"
	regFont := fontsDir + "/Inter-Regular.ttf"

	// Margins
	marginLeft := 80.0
	marginRight := 60.0
	maxWidth := float64(W) - marginLeft - marginRight

	// 5. Draw Date (Top Left)
	if err := dc.LoadFontFace(boldFont, 32); err == nil {
		dc.SetColor(color.RGBA{100, 100, 100, 255}) // Dark Gray
		dc.DrawString(dateStr, marginLeft, 80)
	}

	// 6. Draw Brand/Favicon (Top Right)
	if faviconPath != "" {
		im, err := gg.LoadImage(faviconPath)
		if err == nil {
			// Resize favicon to 64x64
			iconSize := 64.0
			w := im.Bounds().Dx()
			scale := iconSize / float64(w)
			
			// Position: Top Right corner
			iconX := float64(W) - marginRight - iconSize
			iconY := 40.0 

			dc.Push()
			dc.Scale(scale, scale)
			dc.DrawImage(im, int(iconX/scale), int(iconY/scale))
			dc.Pop()

			// Draw "Kush Blogs" text
			if err := dc.LoadFontFace(boldFont, 32); err == nil {
				dc.SetColor(color.RGBA{50, 50, 50, 255})
				text := "Kush Blogs"
				w, _ := dc.MeasureString(text)
				dc.DrawString(text, iconX-w-20, 80) 
			}
		}
	}

	// 7. Draw Title (Center-ish)
	if err := dc.LoadFontFace(boldFont, 80); err != nil {
		return fmt.Errorf("failed to load bold font: %w", err)
	}
	dc.SetColor(color.RGBA{20, 20, 20, 255}) 
	
	titleY := 250.0
	dc.DrawStringWrapped(title, marginLeft, titleY, 0, 0, maxWidth, 1.2, gg.AlignLeft)

	// 8. Draw Description
	if err := dc.LoadFontFace(regFont, 40); err == nil {
		dc.SetColor(color.RGBA{80, 80, 85, 255}) 
		descY := 480.0
		dc.DrawStringWrapped(description, marginLeft, descY, 0, 0, maxWidth, 1.5, gg.AlignLeft)
	}

	// 9. Save as WebP
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Use Lossless for sharp text/graphics
	return webp.Encode(f, dc.Image(), &webp.Options{Lossless: true})
}