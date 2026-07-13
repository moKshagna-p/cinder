package visualizer

import (
	"math"
	"strconv"

	"cinder/config"
)

type pixel struct {
	r float64
	g float64
	b float64
	a float64
}

// Glyph ladder: density/luma drive maps to increasingly heavy glyphs.
// ladderThresholds[k] is the boundary between band k and band k+1.
var (
	ladderGlyphs     = [7]byte{'.', ':', '-', '*', 'o', 'O', '@'}
	ladderThresholds = [6]float64{0.07, 0.14, 0.24, 0.35, 0.50, 0.69}
)

const (
	blankBand       = 0xFF // sentinel: cell rendered as space last frame
	glyphHysteresis = 0.02 // drive margin required to cross into an adjacent band
	blankOffCut     = 0.028
	blankOnCut      = 0.045
)

// acquireFrame returns the reusable zeroed frame buffer for the current size.
func (s *System) acquireFrame() []pixel {
	n := s.width * s.height
	if cap(s.frameBuf) < n {
		s.frameBuf = make([]pixel, n)
	}
	b := s.frameBuf[:n]
	for i := range b {
		b[i] = pixel{}
	}
	return b
}

// frameToString converts a pixel buffer into an ANSI truecolor string.
// Color escapes are emitted only when the color changes from the previous
// cell, and glyph selection is hysteretic per cell so densities hovering at
// a band boundary don't shimmer between glyphs every frame.
func (s *System) frameToString(b []pixel) string {
	w, h := s.width, s.height
	if len(s.glyphBand) != w*h {
		s.glyphBand = make([]byte, w*h)
		for i := range s.glyphBand {
			s.glyphBand[i] = blankBand
		}
	}
	if cap(s.outBuf) < (w+1)*h {
		s.outBuf = make([]byte, 0, (w+1)*h*6)
	}
	out := s.outBuf[:0]
	out = append(out, "\x1b[48;2;0;0;0m"...)

	low := clamp01(s.audioLow)
	mid := clamp01(s.audioMid)
	high := clamp01(s.audioHigh)
	flux := clamp01(s.audio.Flux)
	sparkleOn := high > 0.62 && flux > 0.12

	lastR, lastG, lastB := -1, -1, -1
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			i := row + x
			p := b[i]
			density := toneMapDensity(p.a)

			prev := s.glyphBand[i]
			blankCut := blankOffCut
			if prev == blankBand {
				blankCut = blankOnCut
			}
			if density <= blankCut {
				s.glyphBand[i] = blankBand
				out = append(out, ' ')
				continue
			}

			ir := int(clamp01(toneMapChannel(p.r)) * 255)
			ig := int(clamp01(toneMapChannel(p.g)) * 255)
			ib := int(clamp01(toneMapChannel(p.b)) * 255)
			luma := clamp01((0.2126*float64(ir) + 0.7152*float64(ig) + 0.0722*float64(ib)) / 255.0)

			drive := clamp01(clamp01(density*0.70+luma*0.30) + 0.18*low + 0.05*mid - 0.10*high)
			band := ladderBand(drive)
			if prev != blankBand && int(prev) != band {
				diff := band - int(prev)
				if diff == 1 || diff == -1 {
					boundary := band
					if int(prev) < boundary {
						boundary = int(prev)
					}
					if math.Abs(drive-ladderThresholds[boundary]) < glyphHysteresis {
						band = int(prev)
					}
				}
			}
			s.glyphBand[i] = byte(band)
			glyph := ladderGlyphs[band]

			// treble sparkle: deliberate glitter on hot high-end, no hysteresis
			if sparkleOn && drive > 0.18 && drive < 0.66 {
				switch {
				case drive < 0.30:
					glyph = ':'
				case drive < 0.42:
					glyph = '-'
				case drive < 0.54:
					glyph = '*'
				default:
					glyph = 'x'
				}
			}

			if ir != lastR || ig != lastG || ib != lastB {
				out = append(out, "\x1b[38;2;"...)
				out = strconv.AppendInt(out, int64(ir), 10)
				out = append(out, ';')
				out = strconv.AppendInt(out, int64(ig), 10)
				out = append(out, ';')
				out = strconv.AppendInt(out, int64(ib), 10)
				out = append(out, 'm')
				lastR, lastG, lastB = ir, ig, ib
			}
			out = append(out, glyph)
		}
		if y < h-1 {
			out = append(out, '\n')
		}
	}
	out = append(out, "\x1b[0m"...)
	s.outBuf = out
	return string(out)
}

func ladderBand(drive float64) int {
	for k, t := range ladderThresholds {
		if drive < t {
			return k
		}
	}
	return len(ladderThresholds)
}

func addCoreGlow(buf []pixel, w, h int, cx, cy float64, p config.Palette, energy, kick, snare float64, profile motionProfile) {
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
			buf[idx] = blend(buf[idx], c, a)
		}
	}
}

func splat(buf []pixel, w, h int, x, y float64, c config.RGB, alpha float64) {
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
			buf[idx] = blend(buf[idx], c, alpha*wgt)
		}
	}
}

func drawLineGlow(buf []pixel, w, h int, x0, y0, x1, y1 float64, c config.RGB, alpha float64) {
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
