package visualizer

import (
	"math"

	"cinder/audioinput"
	"cinder/config"
)

// renderSpectrum draws animated frequency bars, always moving with the beat.
func (s *System) renderSpectrum() string {
	b := s.acquireFrame()
	bands := audioinput.SpectrumBands

	// distribute bars across the full width with 1-col gaps
	barW := (s.width - bands + 1) / bands
	if barW < 2 {
		barW = 2
	}

	// subtle dark background grid
	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			if y%5 == 0 || x%(barW+1) == barW {
				b[y*s.width+x] = blend(b[y*s.width+x], s.palette.Outer, 0.03)
			}
		}
	}

	bassDrive := s.kick
	if s.audio.Active {
		bassDrive = s.audio.Bass
	}
	trebleDrive := s.hat
	if s.audio.Active {
		trebleDrive = s.audio.Treble
	}

	for band := 0; band < bands; band++ {
		energy := s.specSmooth[band]
		bandNorm := float64(band) / float64(bands-1) // 0=bass, 1=treble

		// bar height scales with energy and overall kick boost
		boost := 0.82 + 0.10*s.kick
		if s.audio.Active {
			boost = 0.74 + 0.08*s.audio.Level
		}

		// React strongly to highs and lows explicitly
		if bandNorm < 0.3 { // Bass bands
			boost += 1.4 * bassDrive
		} else if bandNorm > 0.7 { // Treble bands
			boost += 1.6 * trebleDrive
		}

		barH := int(math.Round(energy * float64(s.height) * boost))
		if barH < 1 {
			barH = 0
		}

		xStart := band * (barW + 1)
		xEnd := xStart + barW - 1
		if xEnd >= s.width {
			xEnd = s.width - 1
		}

		for x := xStart; x <= xEnd; x++ {
			for row := 0; row < barH; row++ {
				y := s.height - 1 - row
				if y < 0 {
					continue
				}
				norm := float64(row) / math.Max(1, float64(barH)) // 0=bottom, 1=top

				// gradient: bottom = bass colour, top = treble colour
				c := config.Mix(s.palette.Core, s.palette.Mid, bandNorm)
				c = config.Mix(c, s.palette.Highlight, norm*norm) // quadratic brightening

				// peak cap: top row is full highlight
				alpha := 0.50 + 0.50*norm
				if row == barH-1 && barH > 2 {
					alpha = 1.0
					c = s.palette.Highlight
				}
				alpha *= (0.55 + 0.45*s.energy)
				b[y*s.width+x] = blend(b[y*s.width+x], c, clamp01(alpha))
			}

			// bar base glow (bottom 2 rows always slightly lit)
			for row := 0; row < 2 && row < s.height; row++ {
				y := s.height - 1 - row
				c := config.Mix(s.palette.Core, s.palette.Outer, bandNorm)
				b[y*s.width+x] = blend(b[y*s.width+x], c, 0.08+0.06*s.kick)
			}
		}

		// reflection: dim mirror below (inverted bars at bottom)
		reflH := barH / 3
		for x := xStart; x <= xEnd; x++ {
			for row := 0; row < reflH; row++ {
				y := s.height - barH - 1 - row
				if y < 0 || y >= s.height {
					continue
				}
				norm := 1 - float64(row)/math.Max(1, float64(reflH))
				c := config.Mix(s.palette.Outer, s.palette.Core, bandNorm)
				b[y*s.width+x] = blend(b[y*s.width+x], c, 0.15*norm*s.energy)
			}
		}
	}

	// --- beat flash: full-width highlight row at top on kick ---
	flashDrive := s.kick
	if s.audio.Active {
		flashDrive = s.audio.Onset
	}
	if flashDrive > 0.68 {
		for x := 0; x < s.width; x++ {
			b[0*s.width+x] = blend(b[0*s.width+x], s.palette.Highlight, (flashDrive-0.68)*0.45)
		}
	}

	return s.frameToString(b)
}
