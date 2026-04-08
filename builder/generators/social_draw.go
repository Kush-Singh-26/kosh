package generators

import (
	"image/color"

	"github.com/fogleman/gg"
)

type GradientOptions struct {
	DC     *gg.Context
	W, H   int
	Colors []string
	Angle  int
}

// drawGradient draws a linear gradient on the context
func drawGradient(opts GradientOptions) {
	dc := opts.DC
	w, h := opts.W, opts.H
	colors := opts.Colors
	angle := opts.Angle

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
