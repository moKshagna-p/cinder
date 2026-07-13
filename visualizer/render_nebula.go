package visualizer

import (
	"math"

	"cinder/config"
)

func (s *System) renderNebula() string {
	b := make([]pixel, s.width*s.height)
	bassPulse := s.kick
	midDrive := s.snare
	trebleShimmer := s.hat
	onsetFlash := 0.0
	brightness := 0.0
	centroidTint := 0.0
	if s.audio.Active {
		audioWeight := clamp01(0.35 + 0.65*s.audioPresence)
		bassPulse = mix(bassPulse, s.audio.Bass, audioWeight)
		midDrive = mix(midDrive, s.audio.MidRange, audioWeight)
		trebleShimmer = mix(trebleShimmer, s.audio.Treble, audioWeight)
		onsetFlash = s.audio.Onset
		brightness = s.audio.Level
		centroidTint = s.audio.Centroid
	} else {
		onsetFlash = s.kick
		brightness = 0.55*s.energy + 0.45*s.kick
		centroidTint = s.hat
	}
	if len(s.trail) == len(b) {
		for i := range s.trail {
			colorFade := 0.80 + 0.10*s.profile.trail + 0.04*brightness
			alphaFade := 0.60 + 0.16*s.profile.trail + 0.10*brightness + 0.08*trebleShimmer
			s.trail[i].r *= colorFade
			s.trail[i].g *= colorFade
			s.trail[i].b *= colorFade
			s.trail[i].a *= alphaFade
			b[i] = s.trail[i]
		}
	}
	addNebulaCloud(&b, s.width, s.height, s.cx, s.cy, s.palette, s.phase, s.energy, s.sectionMorph, bassPulse, midDrive, s.profile)
	applyVoid(&b, s.width, s.height, s.cx, s.cy, s.voidRadius*(0.82+0.55*bassPulse))
	drawOrbiters(&b, s.width, s.height, s.palette, s.orbiters, 0.35+0.55*s.energy+0.25*midDrive, s.profile)

	for _, p := range s.particles {
		age := p.Life / p.MaxLife
		if age < 0 {
			age = 0
		}
		if age > 1 {
			age = 1
		}

		dx := p.X - s.cx
		dy := p.Y - s.cy
		radiusNorm := math.Hypot(dx, dy) / math.Max(4, math.Min(float64(s.width), float64(s.height))*0.5)
		if radiusNorm > 1 {
			radiusNorm = 1
		}

		c := config.Mix(s.palette.Core, s.palette.Mid, radiusNorm*0.8)
		if radiusNorm > 0.55 {
			c = config.Mix(c, s.palette.Outer, (radiusNorm-0.55)/0.45)
		}
		if centroidTint > 0 {
			c = config.Mix(c, s.palette.Highlight, clamp01(centroidTint*(0.18+0.42*radiusNorm)))
		}
		if age > 0.85 {
			c = config.Mix(c, s.palette.Highlight, (age-0.85)/0.15)
		}
		if onsetFlash > 0.12 {
			c = config.Mix(c, s.palette.Highlight, clamp01(0.10+0.55*onsetFlash))
		}

		alpha := (0.22 + 0.78*age) * p.Brightness
		alpha *= 0.28 + 0.48*s.energy + 0.18*s.profile.glow + 0.16*brightness
		alpha *= 0.76 + 0.42*bassPulse + 0.22*onsetFlash + 0.10*trebleShimmer
		splat(&b, s.width, s.height, p.X, p.Y, c, alpha)
	}

	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, bassPulse, onsetFlash, s.profile)
	if len(s.trail) == len(b) {
		copy(s.trail, b)
	}
	return pixelBufToString(b, s.width, s.height, s.audioLow, s.audioMid, s.audioHigh, s.audio.Flux)
}

func addNebulaCloud(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, phase, energy, sectionMorph, kick, snare float64, profile motionProfile) {
	if w < 4 || h < 4 {
		return
	}
	rx := math.Max(8, float64(w)*(0.28+0.08*sectionMorph+0.03*profile.trippy))
	ry := math.Max(4, float64(h)*(0.18+0.08*(1-sectionMorph)+0.04*profile.drift))

	for y := 0; y < h; y++ {
		fy := (float64(y) - cy) / ry
		for x := 0; x < w; x++ {
			fx := (float64(x) - cx) / rx
			r2 := fx*fx + fy*fy
			if r2 > 2.4 {
				continue
			}

			theta := math.Atan2(fy, fx)
			ribbon := 0.5 + 0.5*math.Sin(theta*(1.1+2.8*profile.trippy)+phase*(0.5+0.9*profile.pace)+r2*(4.0+3.4*profile.chaos))
			wave := 0.5 + 0.5*math.Sin((fx-fy)*(4.2+4.8*profile.trippy)-phase*(0.3+0.7*profile.pace)+theta*(0.8+1.6*sectionMorph))
			field := 0.58*ribbon + 0.42*wave
			falloff := math.Exp(-r2 * (1.8 + 0.5*(1-energy) + 0.2*(1-profile.trail)))
			a := falloff * (0.04 + 0.10*field) * (0.18 + 0.58*energy) * (0.88 + 0.18*snare + 0.12*profile.glow)
			if a < 0.01 {
				continue
			}

			c := config.Mix(p.Outer, p.Mid, 0.20+0.60*field)
			if profile.trippy > 0.5 {
				c = config.Mix(c, p.Highlight, (profile.trippy-0.5)*0.35+0.20*wave)
			}
			idx := y*w + x
			(*buf)[idx] = blend((*buf)[idx], c, a)
		}
	}
}

func applyVoid(buf *[]pixel, w, h int, cx, cy, radius float64) {
	if radius < 1 {
		return
	}
	x0 := int(cx - radius - 2)
	x1 := int(cx + radius + 2)
	y0 := int(cy - radius - 2)
	y1 := int(cy + radius + 2)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > w-1 {
		x1 = w - 1
	}
	if y1 > h-1 {
		y1 = h - 1
	}

	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			d := math.Hypot(dx, dy*1.25)
			if d > radius {
				continue
			}
			t := 1 - d/radius
			dim := 1 - 0.96*t*t
			idx := y*w + x
			(*buf)[idx].r *= dim
			(*buf)[idx].g *= dim
			(*buf)[idx].b *= dim
			(*buf)[idx].a *= dim
		}
	}
}

func drawOrbiters(buf *[]pixel, w, h int, p config.Palette, orbiters []Orbiter, energy float64, profile motionProfile) {
	for i := range orbiters {
		o := orbiters[i]
		trace := config.Mix(p.Highlight, p.Core, 0.18+0.18*float64(i%3)+0.18*profile.trippy)
		traceA := (0.04 + 0.08*o.bright) * (0.28 + 0.56*energy + 0.18*profile.glow)
		drawLineGlow(buf, w, h, o.prevX, o.prevY, o.x, o.y, trace, traceA)
		head := config.Mix(p.Highlight, p.Mid, 0.25+0.25*profile.trippy)
		splat(buf, w, h, o.x, o.y, head, (0.16+0.30*o.bright)*(0.30+0.44*energy+0.16*profile.glow))
	}
}
