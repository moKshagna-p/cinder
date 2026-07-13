package visualizer

import (
	"math"
	"regexp"
	"strings"
	"testing"

	"cinder/audioinput"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func testAudioFeatures() AudioFeatures {
	f := AudioFeatures{
		Active:   true,
		Level:    0.6,
		Bass:     0.7,
		Treble:   0.4,
		MidRange: 0.5,
		Flux:     0.3,
		Onset:    0.7,
		Centroid: 0.5,
		BPM:      128,
	}
	for i := 0; i < audioinput.WaveformLen; i++ {
		f.WaveformBuf[i] = 0.8 * math.Sin(float64(i)*0.2)
	}
	for i := 0; i < audioinput.SpectrumBands; i++ {
		f.Spectrum[i] = 0.5 + 0.4*math.Sin(float64(i))
	}
	return f
}

func newTestSystem(w, h int) *System {
	s := NewSystem(280)
	s.Resize(w, h)
	s.SetSongSignature("test|Track|Artist", "Track", "Artist")
	s.SetPlaying(true)
	return s
}

func runSim(s *System, steps int) {
	const dt = 1.0 / 120
	for i := 0; i < steps; i++ {
		s.Update(dt)
	}
}

func assertFrame(t *testing.T, frame string, w, h int) {
	t.Helper()
	if frame == "" {
		t.Fatal("empty frame")
	}
	lines := strings.Split(frame, "\n")
	if len(lines) != h {
		t.Fatalf("expected %d lines, got %d", h, len(lines))
	}
	for i, line := range lines {
		plain := ansiRE.ReplaceAllString(line, "")
		if len(plain) != w {
			t.Fatalf("line %d: expected width %d, got %d", i, w, len(plain))
		}
	}
}

func TestRenderAllModes(t *testing.T) {
	const w, h = 120, 40
	for withAudio, label := range map[bool]string{false: "synthetic", true: "audio"} {
		t.Run(label, func(t *testing.T) {
			s := newTestSystem(w, h)
			if withAudio {
				s.SetAudioFeatures(testAudioFeatures())
			}
			for mode := 0; mode < int(modeCount); mode++ {
				runSim(s, 60)
				assertFrame(t, s.Render(), w, h)
				s.NextMode()
			}
		})
	}
}

func TestSimulationStaysFinite(t *testing.T) {
	s := newTestSystem(160, 50)
	s.SetAudioFeatures(testAudioFeatures())
	s.Explode()
	runSim(s, 1200) // 10 seconds at 120 Hz

	maxV := 34.0 + 18.0 + 8.0 + 1 // maxV clamp upper bound plus slack
	for i, p := range s.particles {
		for name, v := range map[string]float64{"X": p.X, "Y": p.Y, "VX": p.VX, "VY": p.VY} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("particle %d: %s is %v", i, name, v)
			}
		}
		if math.Abs(p.VX) > maxV || math.Abs(p.VY) > maxV {
			t.Fatalf("particle %d: velocity (%.2f, %.2f) exceeds clamp", i, p.VX, p.VY)
		}
	}
}

func TestPauseFreezesParticles(t *testing.T) {
	s := newTestSystem(120, 40)
	runSim(s, 240)
	s.SetPlaying(false)
	runSim(s, 1200)

	var speedSum float64
	for _, p := range s.particles {
		speedSum += math.Hypot(p.VX, p.VY)
	}
	avg := speedSum / float64(len(s.particles))
	if avg > 1.0 {
		t.Fatalf("expected particles nearly frozen after pause, avg speed %.3f", avg)
	}
}

func TestResizeSmaller(t *testing.T) {
	s := newTestSystem(200, 60)
	runSim(s, 30)
	s.Resize(40, 12)
	runSim(s, 30)
	assertFrame(t, s.Render(), 40, 12)
}

func BenchmarkUpdate(b *testing.B) {
	s := newTestSystem(200, 55)
	s.SetAudioFeatures(testAudioFeatures())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Update(1.0 / 120)
	}
}

func BenchmarkRenderNebula(b *testing.B) {
	s := newTestSystem(200, 55)
	s.SetAudioFeatures(testAudioFeatures())
	runSim(s, 60)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Render()
	}
}
