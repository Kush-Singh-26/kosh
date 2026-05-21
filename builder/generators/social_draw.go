package generators

import (
	"image/color"

	"github.com/fogleman/gg"
)

const (
	dotRadius     = 2.0
	dotGridOffset = dotSpacing / 2

	dotColorA  = 70
	dotSpacing = 32

	grainOpacity = 0.05
	accentWidth  = 12.0
	borderWidth  = 1.0
)

// drawClassyBackground draws a solid background with a subtle radial highlight for depth.
func drawClassyBackground(dc *gg.Context, w, h int, baseColor string) {
	dc.Push()
	defer dc.Pop()

	// 1. Solid Base
	dc.SetColor(hexToRGBA(baseColor))
	dc.Clear()

	// 2. Subtle Radial Glow (Top Left)
	// This adds "material" depth without looking like a traditional gradient
	grad := gg.NewRadialGradient(0, 0, 0, 0, 0, float64(w)*0.8)
	c := hexToRGBA(baseColor)
	// Slightly lighter version for the center of the glow
	highlight := color.RGBA{
		R: uint8(min(255, int(c.R)+15)),
		G: uint8(min(255, int(c.G)+15)),
		B: uint8(min(255, int(c.B)+15)),
		A: 255,
	}
	grad.AddColorStop(0, highlight)
	grad.AddColorStop(1, c)

	dc.SetFillStyle(grad)
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()
}

// drawElegantBorder adds a thin, sophisticated border.
func drawElegantBorder(dc *gg.Context, w, h int, color color.RGBA) {
	dc.Push()
	defer dc.Pop()

	// Subtle border - 15% opacity of the accent color
	dc.SetRGBA255(int(color.R), int(color.G), int(color.B), 40)
	dc.SetLineWidth(borderWidth)
	dc.DrawRectangle(borderWidth/2, borderWidth/2, float64(w)-borderWidth, float64(h)-borderWidth)
	dc.Stroke()
}

// drawGrain adds a subtle noise texture to the social card.
func drawGrain(dc *gg.Context, w, h int) {
	dc.Push()
	defer dc.Pop()

	// Very subtle noise - 5% opacity
	dc.SetRGBA(0, 0, 0, grainOpacity)

	// Simple noise simulation using many small dots
	for i := 0; i < 20000; i++ {
		x := float64(i%w) + (float64(i) * 0.7)
		y := float64(i/w) + (float64(i) * 0.3)
		dc.SetPixel(int(x)%w, int(y)%h)
	}
}

// drawAccentLine draws a vertical accent stripe on the left edge.
func drawAccentLine(dc *gg.Context, h int, color color.RGBA) {
	dc.Push()
	defer dc.Pop()

	dc.SetColor(color)
	dc.DrawRectangle(0, 0, accentWidth, float64(h))
	dc.Fill()
}

// drawDotPattern adds a visible dot pattern overlay
func drawDotPattern(dc *gg.Context, w, h int, baseColor color.RGBA) {
	// Use a 10% opacity version of the provided color (usually text color)
	dc.SetRGBA255(int(baseColor.R), int(baseColor.G), int(baseColor.B), dotColorA)

	// Grid spacing
	spacing := dotSpacing
	radius := dotRadius

	for x := dotGridOffset; x < w; x += spacing {
		for y := dotGridOffset; y < h; y += spacing {
			dc.DrawCircle(float64(x), float64(y), radius)
			dc.Fill()
		}
	}
}
