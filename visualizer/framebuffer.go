package visualizer

import (
	"fmt"
	"math"
	"strings"

	"cinder/config"
)

type pixel struct {
	r float64
	g float64
	b float64
	a float64
}

// pixelBufToString converts a pixel buffer into an ANSI-colored string.
func pixelBufToString(b []pixel, w, h int, low, mid, high, flux float64) string {
	var out strings.Builder
	out.Grow((w + 1) * h * 4)
	out.WriteString("\x1b[48;2;0;0;0m")
	low = clamp01(low)
	mid = clamp01(mid)
	high = clamp01(high)
	flux = clamp01(flux)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := b[y*w+x]
			density := toneMapDensity(p.a)
			if density <= 0.035 {
				out.WriteByte(' ')
				continue
			}
			ir := int(clamp01(toneMapChannel(p.r)) * 255)
			ig := int(clamp01(toneMapChannel(p.g)) * 255)
			ib := int(clamp01(toneMapChannel(p.b)) * 255)
			luma := clamp01((0.2126*float64(ir) + 0.7152*float64(ig) + 0.0722*float64(ib)) / 255.0)
			glyph := glyphFor(density, luma, low, mid, high, flux)
			out.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm%c", ir, ig, ib, glyph))
		}
		if y < h-1 {
			out.WriteByte('\n')
		}
	}
	out.WriteString("\x1b[0m")
	return out.String()
}

func addCoreGlow(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, energy, kick, snare float64, profile motionProfile) {
	radius := 4.0 + energy*2.4 + kick*(1.2+1.8*profile.punch) + profile.glow*2.2
	for oy := -7; oy <= 7; oy++ {
		for ox := -14; ox <= 14; ox++ {
			x := int(cx) + ox
			y := int(cy) + oy
			if x < 0 || x >= w || y < 0 || y >= h {
				continue
			}
			d := math.Hypot(float64(ox)*0.6, float64(oy))
			if d > radius {
				continue
			}
			falloff := 1 - d/radius
			c := config.Mix(p.Core, p.Highlight, clamp01(0.18+0.28*energy+0.30*snare+0.18*profile.trippy))
			a := 0.28 * falloff * (0.42 + 0.28*energy + 0.26*kick + 0.28*profile.glow)
			idx := y*w + x
			(*buf)[idx] = blend((*buf)[idx], c, a)
		}
	}
}

func splat(buf *[]pixel, w, h int, x, y float64, c config.RGB, alpha float64) {
	ix := int(math.Round(x))
	iy := int(math.Round(y))

	for oy := -2; oy <= 2; oy++ {
		for ox := -2; ox <= 2; ox++ {
			tx := ix + ox
			ty := iy + oy
			if tx < 0 || tx >= w || ty < 0 || ty >= h {
				continue
			}

			d := math.Hypot(float64(ox), float64(oy))
			if d > 2.2 {
				continue
			}
			wgt := 1 - d/2.2
			idx := ty*w + tx
			(*buf)[idx] = blend((*buf)[idx], c, alpha*wgt)
		}
	}
}

func drawLineGlow(buf *[]pixel, w, h int, x0, y0, x1, y1 float64, c config.RGB, alpha float64) {
	dx := x1 - x0
	dy := y1 - y0
	steps := int(math.Hypot(dx, dy) * 1.4)
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + dx*t
		y := y0 + dy*t
		fade := 1 - t*0.7
		splat(buf, w, h, x, y, c, alpha*fade)
	}
}

func blend(dst pixel, c config.RGB, a float64) pixel {
	a = clamp01(a)
	dstAlpha := clamp01(dst.a)
	gain := 1.0 - 0.34*dstAlpha
	dst.r += c.R * a * gain
	dst.g += c.G * a * gain
	dst.b += c.B * a * gain
	dst.a += a * (1.0 - 0.55*dstAlpha)
	if dst.a > 2.8 {
		dst.a = 2.8
	}
	return dst
}

func glyphFor(density, luma, low, mid, high, flux float64) byte {
	density = clamp01(density)
	luma = clamp01(luma)
	drive := clamp01(density*0.70 + luma*0.30)
	drive = clamp01(drive + 0.18*low + 0.05*mid - 0.10*high)

	if high > 0.62 && flux > 0.12 && drive > 0.18 && drive < 0.66 {
		switch {
		case drive < 0.30:
			return ':'
		case drive < 0.42:
			return '-'
		case drive < 0.54:
			return '*'
		default:
			return 'x'
		}
	}

	switch {
	case drive < 0.07:
		return '.'
	case drive < 0.14:
		return ':'
	case drive < 0.24:
		return '-'
	case drive < 0.35:
		return '*'
	case drive < 0.50:
		return 'o'
	case drive < 0.69:
		return 'O'
	default:
		return '@'
	}
}

func toneMapChannel(v float64) float64 {
	if v <= 0 {
		return 0
	}
	s := v * 1.25
	return math.Pow(s/(1+s), 0.92)
}

func toneMapDensity(alpha float64) float64 {
	if alpha <= 0 {
		return 0
	}
	return math.Pow(1-math.Exp(-0.55*alpha), 0.90)
}
