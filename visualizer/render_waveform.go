package visualizer

import (
	"math"

	"cinder/audioinput"
	"cinder/config"
)

// renderWaveform draws a live waveform that always moves with the beat.
// When audio is active it shows real amplitude; otherwise synthetic oscillators drive it.
func (s *System) renderWaveform() string {
	b := s.acquireFrame()
	mid := s.cy

	// --- Background: radial glow that pulses with kick/snare ---
	glowStrength := 0.06 + 0.10*s.kick + 0.04*s.snare
	for y := 0; y < s.height; y++ {
		fy := (float64(y) - mid) / (mid + 1)
		bg := math.Exp(-fy*fy*3.5) * glowStrength
		c := config.Mix(s.palette.Core, s.palette.Mid, math.Abs(fy))
		for x := 0; x < s.width; x++ {
			b[y*s.width+x] = blend(b[y*s.width+x], c, bg)
		}
	}

	waveLen := audioinput.WaveformLen
	colStep := float64(waveLen) / float64(s.width)

	// amplitude scale: always at least 35% of half-height so it's visible
	audioLevel := s.audio.Level
	if !s.audio.Active {
		audioLevel = 0.5 + 0.5*s.kick // synthetic "level" from beat
	}
	bassDrive := s.kick
	if s.audio.Active {
		bassDrive = s.audio.Bass
	}
	trebleDrive := s.hat
	if s.audio.Active {
		trebleDrive = s.audio.Treble
	}
	scale := mid * (0.22 + 0.42*audioLevel + 0.12*bassDrive)

	// Make the entire wave baseline heave up and down with the bass (Lows)
	midOffset := math.Sin(s.phase*4.0) * bassDrive * 12.0
	mid += midOffset

	for x := 0; x < s.width; x++ {
		idx := int(float64(x) * colStep)
		if idx >= waveLen {
			idx = waveLen - 1
		}
		t := float64(x)*colStep - float64(idx)
		idxNext := idx + 1
		if idxNext >= waveLen {
			idxNext = waveLen - 1
		}
		sample := s.waveSmooth[idx]*(1-t) + s.waveSmooth[idxNext]*t

		// Add high-frequency jitter to the wave surface based on treble (Highs)
		jitter := math.Sin(float64(x)*1.5+s.phase*20.0) * trebleDrive * 6.0

		// primary wave position
		ys := mid - sample*scale + jitter

		// --- draw filled area between midline and wave (oscilloscope fill) ---
		y0 := int(math.Round(math.Min(mid, ys)))
		y1 := int(math.Round(math.Max(mid, ys)))
		if y0 == y1 {
			if sample > 0 {
				y0 = y1 - 1
			} else {
				y1 = y0 + 1
			}
		}
		for y := y0; y <= y1; y++ {
			if y < 0 || y >= s.height {
				continue
			}
			// distance from the wave surface (bright edge) to midline (dim)
			distFromEdge := math.Abs(float64(y) - ys)
			distFromMid := math.Abs(float64(y) - mid)
			totalH := math.Abs(ys - mid)
			if totalH < 1 {
				totalH = 1
			}
			// gradient: full bright at wave edge, fades toward midline
			edgeAlpha := math.Exp(-distFromEdge * 1.2)
			fillAlpha := (1 - distFromMid/totalH) * 0.35

			// colour varies: low freq (bottom half) = core→mid, high freq (top) = mid→highlight
			normY := 1 - float64(y)/float64(s.height) // 0=bottom, 1=top
			c := config.Mix(s.palette.Core, s.palette.Mid, normY*1.5)
			if normY > 0.5 {
				c = config.Mix(c, s.palette.Highlight, (normY-0.5)*2.0)
			}
			// bass tints the fill warm, treble makes it colder
			bassVal := s.kick
			if s.audio.Active {
				bassVal = s.audio.Bass
			}
			trebleVal := s.hat
			if s.audio.Active {
				trebleVal = s.audio.Treble
			}
			c = config.Mix(c, s.palette.Highlight, clamp01(trebleVal*0.4))
			c = config.Mix(c, s.palette.Core, clamp01(bassVal*0.3))

			alpha := math.Max(edgeAlpha, fillAlpha) * (0.42 + 0.42*audioLevel)
			b[y*s.width+x] = blend(b[y*s.width+x], c, clamp01(alpha))
		}

		// bright dot on the wave surface
		dotY := int(math.Round(ys))
		if dotY >= 0 && dotY < s.height {
			dotAlpha := 0.72 + 0.12*s.kick
			if s.audio.Active {
				dotAlpha = 0.64 + 0.18*s.audio.Onset
			}
			b[dotY*s.width+x] = blend(b[dotY*s.width+x], s.palette.Highlight, dotAlpha)
		}

		// mirror image (inverted, dimmer) — gives symmetric oscilloscope look
		ysMirror := mid + sample*scale*0.55
		dotYm := int(math.Round(ysMirror))
		if dotYm >= 0 && dotYm < s.height {
			b[dotYm*s.width+x] = blend(b[dotYm*s.width+x], s.palette.Mid, 0.45+0.25*s.snare)
		}
	}

	// --- centre line ---
	for x := 0; x < s.width; x++ {
		y := int(mid)
		if y >= 0 && y < s.height {
			b[y*s.width+x] = blend(b[y*s.width+x], s.palette.Mid, 0.12+0.18*s.kick)
		}
	}

	// --- beat flash: vertical bright line on kick ---
	flashDrive := s.kick
	if s.audio.Active {
		flashDrive = s.audio.Onset
	}
	if flashDrive > 0.72 {
		flashAlpha := (flashDrive - 0.72) * 1.1
		for y := 0; y < s.height; y++ {
			x := s.width / 2
			b[y*s.width+x] = blend(b[y*s.width+x], s.palette.Highlight, flashAlpha*0.4)
		}
	}

	return s.frameToString(b)
}
