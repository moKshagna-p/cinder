package visualizer

import (
	"math"

	"cinder/config"
)

// renderPulse draws concentric rings expanding on every beat — always animated.
func (s *System) renderPulse() string {
	b := s.acquireFrame()

	const aY = 2.0 // aspect correction

	// --- background: slow rotating nebula rings (always visible context) ---
	// Use 6 concentric "standing" rings that breathe with the beat
	maxW := float64(s.width) * 0.49
	maxH := float64(s.height) / aY * 0.92
	for i := 1; i <= 8; i++ {
		fi := float64(i)
		t := fi / 8.0 // 0..1 from inner to outer

		// radius breathes: inner rings pulse more with bass, outer with treble
		bassBreathe := s.kick*(1-t) + s.snare*t*0.5
		rX := maxW * (t*0.95 + 0.04*bassBreathe + 0.02*math.Sin(s.phase*0.5+fi))
		rY := maxH * (t*0.95 + 0.04*bassBreathe + 0.02*math.Cos(s.phase*0.4+fi*1.3))

		opacity := (0.06 + 0.14*bassBreathe) * math.Exp(-t*1.2) * s.energy
		c := config.Mix(s.palette.Outer, s.palette.Mid, t)
		if bassBreathe > 0.3 {
			c = config.Mix(c, s.palette.Core, clamp01((bassBreathe-0.3)*1.5))
		}

		angStep := 0.025 - 0.01*t // finer sampling on outer rings
		for angle := 0.0; angle < 2*math.Pi; angle += angStep {
			px := s.cx + math.Cos(angle)*rX
			py := s.cy + math.Sin(angle)*rY
			splat(b, s.width, s.height, px, py, c, opacity)
		}
	}

	// --- expanding beat rings ---
	trebleMod := s.hat
	if s.audio.Active {
		trebleMod = s.audio.Treble
	}

	for _, ring := range s.pulseRings {
		alpha := ring.life * ring.life * 0.90
		if alpha < 0.01 {
			continue
		}
		rX := ring.radius
		rY := ring.radius / aY

		angStep := math.Max(0.008, 0.025-ring.radius*0.0002)
		for angle := 0.0; angle < 2*math.Pi; angle += angStep {
			// Treble causes the rings to vibrate/become jagged
			jitterX := math.Cos(angle*12.0+s.phase*15.0) * trebleMod * 2.5
			jitterY := math.Sin(angle*12.0+s.phase*15.0) * trebleMod * 2.5

			px := s.cx + math.Cos(angle)*rX + jitterX
			py := s.cy + math.Sin(angle)*rY + jitterY
			splat(b, s.width, s.height, px, py, ring.color, alpha)
		}
		// slightly thicker ring (second pass at slight offset)
		for angle := 0.0; angle < 2*math.Pi; angle += angStep {
			jitterX := math.Cos(angle*12.0+s.phase*15.0) * trebleMod * 2.5
			jitterY := math.Sin(angle*12.0+s.phase*15.0) * trebleMod * 2.5

			px := s.cx + math.Cos(angle)*(rX+1.5) + jitterX
			py := s.cy + math.Sin(angle)*(rY+0.75) + jitterY
			splat(b, s.width, s.height, px, py, ring.color, alpha*0.5)
		}
	}

	// --- core glow ---
	addCoreGlow(b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, s.kick, s.snare, s.profile)
	return s.frameToString(b)
}
