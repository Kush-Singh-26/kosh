package generators

import (
	"image/color"

	"github.com/fogleman/gg"
)

const (
	gradientMinColors        = 2
	gradientDefaultBG        = "#faf8f5"
	gradientFullTurnDegrees  = 360
	gradientHorizontalStart  = 45
	gradientHorizontalEnd    = 135
	gradientHorizontalStart2 = 225
	gradientHorizontalEnd2   = 315
	gradientAlpha            = 1.0
	gradientRectThickness    = 1.0
	colorMaxFloat            = 255.0
	dotColorR                = 120
	dotColorG                = 100
	dotColorB                = 80
	dotColorA                = 70
	dotSpacing               = 32
	dotRadius                = 2.0
	dotGridOffset            = dotSpacing / 2
)

// GradientOptions configures gradient drawing.
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

	if len(colors) < gradientMinColors {
		// If only one color or no colors, use solid background
		bg := gradientDefaultBG
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
	angle %= gradientFullTurnDegrees
	if angle < 0 {
		angle += gradientFullTurnDegrees
	}

	// Draw gradient as a series of rectangles
	steps := h
	isHorizontal := angle >= gradientHorizontalStart && angle < gradientHorizontalEnd ||
		angle >= gradientHorizontalStart2 && angle < gradientHorizontalEnd2
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

		dc.SetRGBA(float64(r)/colorMaxFloat, float64(g)/colorMaxFloat, float64(b)/colorMaxFloat, gradientAlpha)

		if isHorizontal {
			// Draw horizontal strip
			dc.DrawRectangle(0, float64(i), float64(w), gradientRectThickness)
		} else {
			// Draw vertical strip
			dc.DrawRectangle(float64(i), 0, gradientRectThickness, float64(h))
		}
		dc.Fill()
	}
}

// drawDotPattern adds a visible dot pattern overlay
func drawDotPattern(dc *gg.Context, w, h int) {
	// More visible warm brown dots
	dc.SetRGBA255(dotColorR, dotColorG, dotColorB, dotColorA) // Warm brown with ~27% opacity

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
