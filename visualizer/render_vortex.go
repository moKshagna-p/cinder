package visualizer

import (
	"math"

	"cinder/config"
)

// renderVortex draws a spinning vortex fully driven by the beat clock and audio.
func (s *System) renderVortex() string {
	b := s.acquireFrame()
	// terminal cells are roughly 2× taller than wide — correct for circle
	const aX = 1.0
	const aY = 2.0

	maxR := math.Min(float64(s.width)/aX, float64(s.height)/aY) * 0.49

	arms := 3 + int(math.Round(s.profile.trippy*3+s.profile.pace*2))

	// beat-reactive modulation values (always driven from synthetic clock)
	kickMod := s.kick
	snareMod := s.snare
	hatMod := s.hat
	bassMod := s.kick*0.7 + s.snare*0.3
	trebleMod := s.hat
	if s.audio.Active {
		bassMod = s.audio.Bass*0.82 + kickMod*0.18
		trebleMod = s.audio.Treble*0.82 + hatMod*0.18
	}

	// Squeeze and stretch the entire vortex space based on Bass (Lows)
	dynAX := aX * (1.0 - 0.25*math.Sin(s.phase*8.0)*bassMod)
	dynAY := aY * (1.0 + 0.30*math.Cos(s.phase*8.0)*bassMod)

	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			fx := (float64(x) - s.cx) / dynAX
			fy := (float64(y) - s.cy) / dynAY
			r := math.Hypot(fx, fy)
			if r > maxR || r < 0.5 {
				continue
			}

			theta := math.Atan2(fy, fx)
			rNorm := r / maxR

			// primary spiral: phase-driven rotation + radial twist
			rot := s.vortexPhase + s.vortexBassAngle*bassMod
			spiral := theta + rot + rNorm*math.Pi*(2.5+3.5*s.profile.chaos+2.0*bassMod)

			// Jagged chaotic disruption of the spiral based on Treble (Highs)
			spiral += math.Sin(rNorm*30.0-s.phase*25.0) * trebleMod * 0.25

			// arm brightness: multiple harmonics of the spiral angle
			arm1 := math.Cos(float64(arms) * spiral)
			arm2 := math.Cos(float64(arms)*spiral*1.5 + trebleMod*math.Pi)
			arm3 := math.Sin(float64(arms+1)*spiral + snareMod*math.Pi*0.7)
			v := 0.50*arm1 + 0.30*arm2 + 0.20*arm3
			if v < 0 {
				v = 0
			}
			v = math.Pow(v, 0.5) // soften the arms so fill is visible

			// outer edge shimmer (hat/treble makes outer ring glitter)
			if rNorm > 0.7 {
				outer := math.Sin(float64(arms*2)*spiral + s.phase*2.0 + trebleMod*4.0)
				v += clamp01(outer) * (rNorm - 0.7) * trebleMod * 0.6
			}

			// radial falloff: tighter when energy is low, expands on beat
			fallExp := 1.5 - 0.6*s.energy - 0.5*kickMod
			falloff := math.Exp(-rNorm * fallExp)
			a := v * falloff * (0.12 + 0.60*s.energy) * (1.0 + 0.5*kickMod + 0.2*snareMod)

			// colour: inner warm → outer cool, with beat tinting
			c := config.Mix(s.palette.Core, s.palette.Mid, rNorm)
			if rNorm > 0.45 {
				c = config.Mix(c, s.palette.Highlight, (rNorm-0.45)/0.55)
			}
			// bass paints the inner glow warm, treble adds cool outer shimmer
			c = config.Mix(c, s.palette.Outer, clamp01(trebleMod*rNorm*0.6))
			c = config.Mix(c, s.palette.Core, clamp01(bassMod*(1-rNorm)*0.5))

			b[y*s.width+x] = blend(b[y*s.width+x], c, clamp01(a))
		}
	}

	// core glow pulsing on kick
	addCoreGlow(b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, kickMod, snareMod, s.profile)
	return s.frameToString(b)
}
