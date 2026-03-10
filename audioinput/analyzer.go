package audioinput

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Enabled    bool
	Device     string
	SampleRate int
	FrameSize  int
}

// WaveformLen is the number of samples kept in the rolling waveform buffer.
const WaveformLen = 256

// SpectrumBands is the number of frequency bands in the spectrum snapshot.
const SpectrumBands = 16

type Features struct {
	Active   bool
	Level    float64
	Bass     float64
	Treble   float64
	MidRange float64 // mid-frequency energy (new)
	Flux     float64
	BPM      float64
	Err      string
	Device   string

	// WaveformBuf holds the last WaveformLen normalised peak values [-1..1]
	// representing the amplitude envelope over time (highs and lows).
	WaveformBuf [WaveformLen]float64

	// SpectrumBands holds SpectrumBands normalised band energies [0..1].
	// Band 0 = sub-bass, Band 15 = high-treble.
	Spectrum [SpectrumBands]float64
}

type Analyzer struct {
	cfg Config

	mu       sync.RWMutex
	features Features

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type detectorState struct {
	frameDur     float64
	lowEnv       float64
	midEnv       float64 // mid-frequency envelope follower
	prevMix      float64
	noiseFloor   float64
	level        float64
	bass         float64
	treble       float64
	midRange     float64
	flux         float64
	bpm          float64
	history      []float64
	historyIdx   int
	historyCount int
	framesToBPM  int

	// rolling waveform: last WaveformLen frame-level amplitude values
	waveform    [WaveformLen]float64
	waveformIdx int

	// per-band envelope followers for spectrum
	bandEnv [SpectrumBands]float64
}

func ConfigFromEnv() Config {
	enabled := envTrue("CINDER_AUDIO_REACTIVE")
	device := strings.TrimSpace(os.Getenv("CINDER_AUDIO_DEVICE"))
	if device != "" {
		enabled = true
	}
	if device == "" {
		device = "default"
	}

	sampleRate := envInt("CINDER_AUDIO_SAMPLE_RATE", 22050)
	if sampleRate < 8000 {
		sampleRate = 22050
	}

	frameSize := envInt("CINDER_AUDIO_FRAME_SIZE", 1024)
	if frameSize < 256 {
		frameSize = 1024
	}

	return Config{
		Enabled:    enabled,
		Device:     device,
		SampleRate: sampleRate,
		FrameSize:  frameSize,
	}
}

func NewAnalyzer(cfg Config) *Analyzer {
	a := &Analyzer{cfg: cfg}
	if !cfg.Enabled {
		return a
	}

	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.setFeatures(Features{Device: cfg.Device})
	a.wg.Add(1)
	go a.run()
	return a
}

func (a *Analyzer) Snapshot() Features {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.features
}

func (a *Analyzer) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
}

func (a *Analyzer) run() {
	defer a.wg.Done()

	backoff := time.Second
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		if err := a.captureOnce(); err != nil {
			a.setError(err.Error())
		}

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (a *Analyzer) captureOnce() error {
	cmd := exec.CommandContext(a.ctx,
		"ffmpeg",
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "nobuffer",
		"-f", "avfoundation",
		"-i", "none:"+a.cfg.Device,
		"-vn",
		"-ac", "1",
		"-ar", strconv.Itoa(a.cfg.SampleRate),
		"-f", "f32le",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var stderrWG sync.WaitGroup
	var stderrLast string
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		buf, _ := io.ReadAll(stderr)
		stderrLast = strings.TrimSpace(string(buf))
	}()

	state := detectorState{
		frameDur: float64(a.cfg.FrameSize) / float64(a.cfg.SampleRate),
		history:  make([]float64, int(math.Round(8.0/(float64(a.cfg.FrameSize)/float64(a.cfg.SampleRate))))),
	}
	if len(state.history) < 64 {
		state.history = make([]float64, 64)
	}

	frameBytes := a.cfg.FrameSize * 4
	buf := make([]byte, frameBytes)
	samples := make([]float64, a.cfg.FrameSize)

	for {
		if _, err := io.ReadFull(stdout, buf); err != nil {
			_ = cmd.Wait()
			stderrWG.Wait()
			if a.ctx.Err() != nil {
				return nil
			}
			if stderrLast != "" {
				return errors.New(stderrLast)
			}
			return err
		}

		for i := 0; i < a.cfg.FrameSize; i++ {
			bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
			samples[i] = float64(math.Float32frombits(bits))
		}

		features := state.analyze(samples)
		features.Device = a.cfg.Device
		a.setFeatures(features)
	}
}

func (d *detectorState) analyze(samples []float64) Features {
	if len(samples) == 0 {
		return Features{}
	}

	var sumSq float64
	var bassSum float64
	var trebleSum float64
	var midSum float64
	var peak float64

	n := len(samples)
	for _, sample := range samples {
		absSample := math.Abs(sample)
		if absSample > peak {
			peak = absSample
		}

		sumSq += sample * sample

		// Low envelope (bass ~20-250 Hz proxy via slow EMA)
		d.lowEnv += 0.055 * (absSample - d.lowEnv)
		bassSum += d.lowEnv

		// Mid envelope (~250-4000 Hz proxy) – faster than bass, slower than treble
		d.midEnv += 0.18 * (absSample - d.midEnv)
		midVal := d.midEnv - d.lowEnv
		if midVal < 0 {
			midVal = 0
		}
		midSum += midVal

		treble := absSample - d.midEnv
		if treble < 0 {
			treble = 0
		}
		trebleSum += treble
	}

	rms := math.Sqrt(sumSq / float64(n))
	bass := bassSum / float64(n)
	mid := midSum / float64(n)
	treble := trebleSum / float64(n)
	mix := 0.65*rms + 0.95*bass + 0.45*treble
	rawFlux := mix - d.prevMix*0.93
	d.prevMix = mix
	if rawFlux < 0 {
		rawFlux = 0
	}

	d.noiseFloor += 0.05 * (rawFlux - d.noiseFloor)
	flux := rawFlux - d.noiseFloor*0.85
	if flux < 0 {
		flux = 0
	}

	d.level += 0.30 * (clamp01(rms*4.6+peak*0.7) - d.level)
	d.bass += 0.30 * (clamp01(bass*7.5) - d.bass)
	d.treble += 0.30 * (clamp01(treble*11.0) - d.treble)
	d.midRange += 0.30 * (clamp01(mid*9.0) - d.midRange)
	d.flux += 0.42 * (clamp01(flux*18.0) - d.flux)

	// --- waveform ring buffer: store signed peak (captures highs & lows) ---
	framePeak := 0.0
	for _, s := range samples {
		if math.Abs(s) > math.Abs(framePeak) {
			framePeak = s
		}
	}
	d.waveform[d.waveformIdx] = framePeak
	d.waveformIdx = (d.waveformIdx + 1) % WaveformLen

	// --- pseudo spectrum: divide samples into bands by decimation ratio ---
	// We simulate SpectrumBands frequency bands by partitioning samples and
	// using different EMA speeds (slow EMA = low freq, fast EMA = high freq).
	bandAlphas := [SpectrumBands]float64{}
	for b := 0; b < SpectrumBands; b++ {
		// alpha goes from 0.02 (sub-bass) to 0.50 (high treble)
		bandAlphas[b] = 0.02 + 0.48*float64(b)/float64(SpectrumBands-1)
	}
	// Run per-band envelope followers over the frame
	for _, sample := range samples {
		absSample := math.Abs(sample)
		for b := 0; b < SpectrumBands; b++ {
			d.bandEnv[b] += bandAlphas[b] * (absSample - d.bandEnv[b])
		}
	}
	// To make bands look like a spectrum (low bands = bass, high bands = treble)
	// we compute each band's *relative* contribution by differencing adjacent slow envelopes.
	var spec [SpectrumBands]float64
	prev := 0.0
	for b := 0; b < SpectrumBands; b++ {
		v := d.bandEnv[b] - prev
		if v < 0 {
			v = 0
		}
		spec[b] = clamp01(v * (8.0 + float64(b)*1.5))
		prev = d.bandEnv[b]
	}

	d.pushOnset(clamp01(flux * 22.0))
	d.framesToBPM++
	if d.historyCount >= len(d.history)/2 && d.framesToBPM >= 8 {
		d.framesToBPM = 0
		if bpm, ok := d.estimateBPM(); ok {
			if d.bpm == 0 {
				d.bpm = bpm
			} else {
				d.bpm += (bpm - d.bpm) * 0.18
			}
		}
	}

	f := Features{
		Active:   d.level > 0.03 || d.flux > 0.03,
		Level:    d.level,
		Bass:     d.bass,
		Treble:   d.treble,
		MidRange: d.midRange,
		Flux:     d.flux,
		BPM:      d.bpm,
	}
	// copy waveform ordered oldest→newest
	for i := 0; i < WaveformLen; i++ {
		f.WaveformBuf[i] = d.waveform[(d.waveformIdx+i)%WaveformLen]
	}
	f.Spectrum = spec
	return f
}

func (d *detectorState) pushOnset(v float64) {
	if len(d.history) == 0 {
		return
	}
	d.history[d.historyIdx] = v
	d.historyIdx = (d.historyIdx + 1) % len(d.history)
	if d.historyCount < len(d.history) {
		d.historyCount++
	}
}

func (d *detectorState) estimateBPM() (float64, bool) {
	history := d.orderedHistory()
	if len(history) < 32 {
		return 0, false
	}

	bestBPM := 0.0
	bestScore := 0.0
	minLag := int(math.Round(60.0 / 180.0 / d.frameDur))
	maxLag := int(math.Round(60.0 / 72.0 / d.frameDur))
	if minLag < 1 {
		minLag = 1
	}

	for lag := minLag; lag <= maxLag; lag++ {
		score := 0.0
		for i := lag; i < len(history); i++ {
			score += history[i] * history[i-lag]
		}
		if score > bestScore {
			bestScore = score
			bestBPM = 60.0 / (float64(lag) * d.frameDur)
		}
	}

	if bestScore < 0.08 {
		return 0, false
	}
	return bestBPM, true
}

func (d *detectorState) orderedHistory() []float64 {
	if d.historyCount == 0 {
		return nil
	}

	out := make([]float64, d.historyCount)
	start := d.historyIdx - d.historyCount
	if start < 0 {
		start += len(d.history)
	}
	for i := 0; i < d.historyCount; i++ {
		out[i] = d.history[(start+i)%len(d.history)]
	}
	return out
}

func (a *Analyzer) setFeatures(features Features) {
	a.mu.Lock()
	defer a.mu.Unlock()
	features.Err = ""
	a.features = features
}

func (a *Analyzer) setError(errText string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.features.Err = strings.TrimSpace(errText)
	a.features.Active = false
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func envTrue(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
