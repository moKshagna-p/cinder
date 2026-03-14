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

const WaveformLen = 256
const SpectrumBands = 16

type Features struct {
	Active   bool
	Level    float64
	Bass     float64
	Treble   float64
	MidRange float64
	Flux     float64
	Onset    float64
	Centroid float64
	BPM      float64
	Err      string
	Device   string

	WaveformBuf [WaveformLen]float64
	Spectrum    [SpectrumBands]float64
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
	sampleRate int
	frameSize  int
	hopSize    int
	frameDur   float64

	window     []float64
	frame      []float64
	frameFill  int
	fftReal    []float64
	fftImag    []float64
	prevBins   []float64
	bandEdges  [SpectrumBands + 1]int
	bandEnv    [SpectrumBands]float64
	bandCenter [SpectrumBands]float64

	level       float64
	bass        float64
	midRange    float64
	treble      float64
	flux        float64
	onset       float64
	centroid    float64
	bpm         float64
	fluxFloor   float64
	onsetFloor  float64
	waveform    [WaveformLen]float64
	waveformIdx int

	history      []float64
	historyIdx   int
	historyCount int
	framesToBPM  int
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

	frameSize := normalizeFrameSize(envInt("CINDER_AUDIO_FRAME_SIZE", 1024))

	if device == "default" {
		if preferred, ok, err := DetectPreferredInput(); err == nil && ok {
			device = preferred
			enabled = true
		}
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

	state := newDetectorState(a.cfg)
	frameBytes := state.hopSize * 4
	buf := make([]byte, frameBytes)
	samples := make([]float64, state.hopSize)

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

		for i := 0; i < state.hopSize; i++ {
			bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
			samples[i] = float64(math.Float32frombits(bits))
		}

		if features, ok := state.push(samples); ok {
			features.Device = a.cfg.Device
			a.setFeatures(features)
		}
	}
}

func newDetectorState(cfg Config) detectorState {
	hopSize := cfg.FrameSize / 2
	if hopSize < 128 {
		hopSize = cfg.FrameSize
	}
	frameDur := float64(hopSize) / float64(cfg.SampleRate)
	historyLen := int(math.Round(8.0 / frameDur))
	if historyLen < 64 {
		historyLen = 64
	}

	edges := logBandEdges(cfg.SampleRate, cfg.FrameSize)
	state := detectorState{
		sampleRate: cfg.SampleRate,
		frameSize:  cfg.FrameSize,
		hopSize:    hopSize,
		frameDur:   frameDur,
		window:     hannWindow(cfg.FrameSize),
		frame:      make([]float64, cfg.FrameSize),
		fftReal:    make([]float64, cfg.FrameSize),
		fftImag:    make([]float64, cfg.FrameSize),
		prevBins:   make([]float64, cfg.FrameSize/2),
		bandEdges:  edges,
		history:    make([]float64, historyLen),
	}

	for i := 0; i < SpectrumBands; i++ {
		startHz := float64(edges[i]*cfg.SampleRate) / float64(cfg.FrameSize)
		endHz := float64(edges[i+1]*cfg.SampleRate) / float64(cfg.FrameSize)
		state.bandCenter[i] = (startHz + endHz) * 0.5
	}
	return state
}

func (d *detectorState) push(samples []float64) (Features, bool) {
	if len(samples) == 0 {
		return Features{}, false
	}

	if d.frameFill < d.frameSize {
		n := copy(d.frame[d.frameFill:], samples)
		d.frameFill += n
		if d.frameFill < d.frameSize {
			return Features{}, false
		}
		return d.analyzeFrame(), true
	}

	copy(d.frame, d.frame[d.hopSize:])
	copy(d.frame[d.frameSize-d.hopSize:], samples)
	return d.analyzeFrame(), true
}

func (d *detectorState) analyzeFrame() Features {
	var sumSq float64
	peak := 0.0
	framePeak := 0.0
	for i, sample := range d.frame {
		if math.Abs(sample) > peak {
			peak = math.Abs(sample)
		}
		if math.Abs(sample) > math.Abs(framePeak) {
			framePeak = sample
		}
		sumSq += sample * sample
		d.fftReal[i] = sample * d.window[i]
		d.fftImag[i] = 0
	}

	rms := math.Sqrt(sumSq / float64(len(d.frame)))
	fft(d.fftReal, d.fftImag)

	var bandRaw [SpectrumBands]float64
	var bandWeight [SpectrumBands]float64
	var fluxSum float64
	var totalMag float64
	var centroidSum float64
	half := d.frameSize / 2

	for bin := 1; bin < half; bin++ {
		real := d.fftReal[bin]
		imag := d.fftImag[bin]
		mag := math.Hypot(real, imag)
		delta := mag - d.prevBins[bin]
		if delta > 0 {
			fluxSum += delta
		}
		d.prevBins[bin] = mag

		totalMag += mag
		freq := float64(bin*d.sampleRate) / float64(d.frameSize)
		centroidSum += freq * mag

		band := d.bandIndexForBin(bin)
		if band >= 0 {
			bandRaw[band] += mag
			bandWeight[band]++
		}
	}

	centroidHz := 0.0
	if totalMag > 1e-9 {
		centroidHz = centroidSum / totalMag
	}
	centroidNorm := normalizeCentroid(centroidHz, float64(d.sampleRate)/2)

	var spectrum [SpectrumBands]float64
	var bassRaw float64
	var midRaw float64
	var trebleRaw float64
	for i := 0; i < SpectrumBands; i++ {
		raw := 0.0
		if totalMag > 1e-9 && bandWeight[i] > 0 {
			raw = bandRaw[i] / totalMag * float64(SpectrumBands)
		}
		raw = clamp01(math.Sqrt(raw) * 1.7)

		attack := 0.42
		release := 0.14
		if raw < d.bandEnv[i] {
			attack = release
		}
		d.bandEnv[i] += (raw - d.bandEnv[i]) * attack
		spectrum[i] = d.bandEnv[i]

		center := d.bandCenter[i]
		switch {
		case center < 250:
			bassRaw += raw
		case center < 4000:
			midRaw += raw
		default:
			trebleRaw += raw
		}
	}

	bassRaw = clamp01(bassRaw / 3.5)
	midRaw = clamp01(midRaw / 5.5)
	trebleRaw = clamp01(trebleRaw / 5.0)

	fluxRaw := 0.0
	if totalMag > 1e-9 {
		fluxRaw = fluxSum / totalMag
	}
	fluxRaw = clamp01(fluxRaw * 3.8)
	d.fluxFloor += 0.05 * (fluxRaw - d.fluxFloor)
	fluxSignal := fluxRaw - d.fluxFloor*0.85
	if fluxSignal < 0 {
		fluxSignal = 0
	}

	onsetRaw := clamp01(fluxSignal * (1.3 + 0.8*bassRaw + 0.35*trebleRaw))
	d.onsetFloor += 0.04 * (onsetRaw - d.onsetFloor)
	onsetSignal := onsetRaw - d.onsetFloor*0.92
	if onsetSignal < 0 {
		onsetSignal = 0
	}

	levelTarget := clamp01(rms*3.4 + peak*0.45)
	d.level = smoothAttackRelease(d.level, levelTarget, 0.34, 0.12)
	d.bass = smoothAttackRelease(d.bass, bassRaw, 0.38, 0.16)
	d.midRange = smoothAttackRelease(d.midRange, midRaw, 0.34, 0.15)
	d.treble = smoothAttackRelease(d.treble, trebleRaw, 0.40, 0.17)
	d.flux = smoothAttackRelease(d.flux, clamp01(fluxSignal*2.2), 0.44, 0.18)
	d.onset = smoothAttackRelease(d.onset, clamp01(onsetSignal*3.0), 0.62, 0.20)
	d.centroid = smoothAttackRelease(d.centroid, centroidNorm, 0.28, 0.12)

	d.waveform[d.waveformIdx] = framePeak
	d.waveformIdx = (d.waveformIdx + 1) % WaveformLen

	d.pushOnset(d.onset)
	d.framesToBPM++
	if d.historyCount >= len(d.history)/2 && d.framesToBPM >= 8 {
		d.framesToBPM = 0
		if bpm, ok := d.estimateBPM(); ok {
			d.bpm = smoothAttackRelease(d.bpm, bpm, 0.20, 0.08)
		}
	}

	f := Features{
		Active:   d.level > 0.03 || d.flux > 0.04 || d.onset > 0.05,
		Level:    d.level,
		Bass:     d.bass,
		Treble:   d.treble,
		MidRange: d.midRange,
		Flux:     d.flux,
		Onset:    d.onset,
		Centroid: d.centroid,
		BPM:      d.bpm,
		Spectrum: spectrum,
	}
	for i := 0; i < WaveformLen; i++ {
		f.WaveformBuf[i] = d.waveform[(d.waveformIdx+i)%WaveformLen]
	}
	return f
}

func (d *detectorState) bandIndexForBin(bin int) int {
	for i := 0; i < SpectrumBands; i++ {
		if bin >= d.bandEdges[i] && bin < d.bandEdges[i+1] {
			return i
		}
	}
	return SpectrumBands - 1
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

	if bestScore < 0.05 {
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

func smoothAttackRelease(current, target, attack, release float64) float64 {
	rate := release
	if target > current {
		rate = attack
	}
	return current + (target-current)*rate
}

func normalizeCentroid(hz, maxHz float64) float64 {
	if hz <= 0 || maxHz <= 0 {
		return 0
	}
	minHz := 80.0
	if hz < minHz {
		hz = minHz
	}
	if hz > maxHz {
		hz = maxHz
	}
	return clamp01(math.Log(hz/minHz+1) / math.Log(maxHz/minHz+1))
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
