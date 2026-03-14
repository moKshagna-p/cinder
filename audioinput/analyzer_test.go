package audioinput

import (
	"math"
	"testing"
)

func TestAnalyzeFrameDetectsLowMidHighEnergy(t *testing.T) {
	tests := []struct {
		name   string
		freq   float64
		assert func(t *testing.T, f Features)
	}{
		{
			name: "bass",
			freq: 60,
			assert: func(t *testing.T, f Features) {
				if f.Bass <= f.MidRange || f.Bass <= f.Treble {
					t.Fatalf("expected bass dominant, got bass=%.3f mid=%.3f treble=%.3f", f.Bass, f.MidRange, f.Treble)
				}
			},
		},
		{
			name: "mid",
			freq: 1000,
			assert: func(t *testing.T, f Features) {
				if f.MidRange <= f.Bass || f.MidRange <= f.Treble {
					t.Fatalf("expected mid dominant, got bass=%.3f mid=%.3f treble=%.3f", f.Bass, f.MidRange, f.Treble)
				}
			},
		},
		{
			name: "treble",
			freq: 8000,
			assert: func(t *testing.T, f Features) {
				if f.Treble <= f.MidRange || f.Treble <= f.Bass {
					t.Fatalf("expected treble dominant, got bass=%.3f mid=%.3f treble=%.3f", f.Bass, f.MidRange, f.Treble)
				}
				if f.Centroid < 0.55 {
					t.Fatalf("expected high centroid for treble, got %.3f", f.Centroid)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newDetectorState(Config{SampleRate: 22050, FrameSize: 1024})
			features := feedSignal(&state, sineWave(tt.freq, 22050, 1024*8, 0.85))
			tt.assert(t, features)
		})
	}
}

func TestAnalyzeFrameSilenceFallsInactive(t *testing.T) {
	state := newDetectorState(Config{SampleRate: 22050, FrameSize: 1024})
	features := feedSignal(&state, make([]float64, 1024*8))
	if features.Active {
		t.Fatalf("expected silence to be inactive")
	}
	if features.Level > 0.05 || features.Flux > 0.05 {
		t.Fatalf("expected silence near zero, got level=%.3f flux=%.3f", features.Level, features.Flux)
	}
}

func TestAnalyzeFrameDetectsOnsetsAndTempo(t *testing.T) {
	state := newDetectorState(Config{SampleRate: 22050, FrameSize: 1024})
	samples := impulseTrain(22050, 120, 12)
	features, peakOnset := feedSignalPeakOnset(&state, samples)
	if peakOnset < 0.12 {
		t.Fatalf("expected onset activity, got %.3f", peakOnset)
	}
	if features.BPM < 110 || features.BPM > 130 {
		t.Fatalf("expected bpm near 120, got %.3f", features.BPM)
	}
}

func feedSignal(state *detectorState, samples []float64) Features {
	hop := state.hopSize
	var f Features
	for start := 0; start+hop <= len(samples); start += hop {
		next, ok := state.push(samples[start : start+hop])
		if ok {
			f = next
		}
	}
	return f
}

func feedSignalPeakOnset(state *detectorState, samples []float64) (Features, float64) {
	hop := state.hopSize
	var f Features
	peakOnset := 0.0
	for start := 0; start+hop <= len(samples); start += hop {
		next, ok := state.push(samples[start : start+hop])
		if ok {
			f = next
			if next.Onset > peakOnset {
				peakOnset = next.Onset
			}
		}
	}
	return f, peakOnset
}

func sineWave(freq float64, sampleRate, samples int, amp float64) []float64 {
	out := make([]float64, samples)
	for i := range out {
		out[i] = math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)) * amp
	}
	return out
}

func impulseTrain(sampleRate, bpm, beats int) []float64 {
	interval := int(math.Round(float64(sampleRate) * 60.0 / float64(bpm)))
	total := interval*beats + sampleRate/2
	out := make([]float64, total)
	for beat := 0; beat < beats; beat++ {
		start := beat * interval
		for i := 0; i < sampleRate/200 && start+i < len(out); i++ {
			out[start+i] = 0.95 * math.Exp(-float64(i)/18.0)
		}
	}
	return out
}
