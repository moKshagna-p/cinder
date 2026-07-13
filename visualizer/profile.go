package visualizer

import (
	"math"
	"strings"
)

type motionProfile struct {
	pace     float64
	chaos    float64
	trippy   float64
	drift    float64
	punch    float64
	glow     float64
	trail    float64
	orbiters int
	voidSize float64
}

func buildMotionProfile(seed uint64, track, artist string) motionProfile {
	text := strings.ToLower(strings.TrimSpace(track + " " + artist))

	fastScore := keywordWeight(text, []string{
		"remix", "mix", "club", "dance", "rave", "beat", "bass", "rush", "riot", "speed",
		"fire", "hard", "hyper", "turbo", "run", "party", "drop", "electric", "pop",
	})
	slowScore := keywordWeight(text, []string{
		"slow", "acoustic", "piano", "ambient", "lullaby", "sleep", "calm", "soft", "rain",
		"interlude", "outro", "dream", "blue", "ocean", "alone", "quiet", "moon",
	})
	trippyScore := keywordWeight(text, []string{
		"dream", "neon", "space", "cosmic", "moon", "night", "echo", "ghost", "haze",
		"wave", "mirror", "prism", "velvet", "star", "electric", "glass", "shadow",
	})

	basePace := 0.20 + 0.60*hashUnit(seed, 0)
	pace := clamp01(basePace + 0.14*fastScore - 0.14*slowScore)
	chaos := clamp01(0.22 + 0.58*hashUnit(seed, 12) + 0.08*fastScore - 0.05*slowScore)
	trippy := clamp01(0.26 + 0.50*hashUnit(seed, 24) + 0.18*trippyScore + 0.08*slowScore)
	drift := clamp01(0.20 + 0.42*hashUnit(seed, 36) + 0.10*slowScore + 0.10*trippy)
	punch := clamp01(0.30 + 0.38*hashUnit(seed, 48) + 0.16*fastScore + 0.10*pace)
	glow := clamp01(0.28 + 0.36*hashUnit(seed, 18) + 0.18*trippy + 0.08*slowScore)
	trail := clamp01(0.18 + 0.40*hashUnit(seed, 30) + 0.20*trippy + 0.14*(1.0-pace))
	orbiters := 3 + int(math.Round(2.0*trippy+2.0*pace))
	if orbiters > 8 {
		orbiters = 8
	}

	return motionProfile{
		pace:     pace,
		chaos:    chaos,
		trippy:   trippy,
		drift:    drift,
		punch:    punch,
		glow:     glow,
		trail:    trail,
		orbiters: orbiters,
		voidSize: 3.8 + 2.0*trippy + 1.6*(1.0-pace),
	}
}

func keywordWeight(text string, words []string) float64 {
	if text == "" {
		return 0
	}
	weight := 0.0
	for _, word := range words {
		if strings.Contains(text, word) {
			weight += 0.18
		}
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func hashUnit(seed uint64, shift uint) float64 {
	return float64((seed>>shift)%1000) / 999.0
}
