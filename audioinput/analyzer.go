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

type captureInput struct {
	Arg   string
	Label string
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
	noiseFloor  float64
	peakFloor   float64
	waveform    [WaveformLen]float64
	waveformIdx int

	history      []float64
	historyIdx   int
	historyCount int
	framesToBPM  int

	waveformStride int
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
	inputs, err := buildCaptureInputs(a.cfg.Device)
	if err != nil {
		return err
	}

	var lastErr error
	for _, input := range inputs {
		for _, sampleRate := range captureSampleRates(a.cfg.SampleRate) {
			if err := a.captureFromInput(input, sampleRate); err != nil {
				if a.ctx.Err() != nil {
					return nil
				}
				lastErr = err
				if looksLikeInputSelectionError(err.Error()) || looksLikeSampleRateError(err.Error()) {
					continue
				}
				return err
			}
			return nil
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("no AVFoundation input candidates available")
}

func (a *Analyzer) captureFromInput(input captureInput, sampleRate int) error {
	cmd := exec.CommandContext(a.ctx,
		"ffmpeg",
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "nobuffer",
		"-f", "avfoundation",
		"-i", input.Arg,
		"-vn",
		"-ac", "1",
		"-ar", strconv.Itoa(sampleRate),
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
			features.Device = input.Label
			a.setFeatures(features)
		}
	}
}

func buildCaptureInputs(device string) ([]captureInput, error) {
	trimmed := strings.TrimSpace(device)
	if trimmed == "" {
		trimmed = "default"
	}

	seen := map[string]bool{}
	out := make([]captureInput, 0, 4)
	add := func(arg, label string) {
		arg = strings.TrimSpace(arg)
		if arg == "" || seen[arg] {
			return
		}
		seen[arg] = true
		out = append(out, captureInput{Arg: arg, Label: strings.TrimSpace(label)})
	}

	if idx, err := strconv.Atoi(trimmed); err == nil && idx >= 0 {
		add(":"+strconv.Itoa(idx), "index:"+strconv.Itoa(idx))
		add("none:"+strconv.Itoa(idx), "index:"+strconv.Itoa(idx))
		return out, nil
	}

	if idx, ok, err := lookupInputDeviceIndex(trimmed); err == nil && ok {
		add(":"+strconv.Itoa(idx), trimmed)
	}

	if strings.EqualFold(trimmed, "default") {
		add(":0", "default")
	}

	add("none:"+trimmed, trimmed)
	add(":"+trimmed, trimmed)

	if len(out) == 0 {
		return nil, errors.New("unable to build AVFoundation input candidates")
	}
	return out, nil
}

func captureSampleRates(primary int) []int {
	if primary <= 0 {
		primary = 22050
	}
	candidates := []int{primary, 44100, 48000, 22050}
	out := make([]int, 0, len(candidates))
	seen := map[int]bool{}
	for _, sr := range candidates {
		if sr < 8000 || sr > 192000 || seen[sr] {
			continue
		}
		seen[sr] = true
		out = append(out, sr)
	}
	return out
}

func looksLikeInputSelectionError(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "error opening input") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "no such device") ||
		strings.Contains(msg, "invalid data found") ||
		strings.Contains(msg, "device not found") ||
		strings.Contains(msg, "input/output error") ||
		strings.Contains(msg, "device busy")
}

func looksLikeSampleRateError(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "sample rate") ||
		strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "could not set")
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
	state.waveformStride = maxInt(1, hopSize/32)

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

	d.appendWaveform(samples)

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
	for i, sample := range d.frame {
		if math.Abs(sample) > peak {
			peak = math.Abs(sample)
		}
		sumSq += sample * sample
		d.fftReal[i] = sample * d.window[i]
		d.fftImag[i] = 0
	}

	rms := math.Sqrt(sumSq / float64(len(d.frame)))
	if d.noiseFloor == 0 || rms < d.noiseFloor {
		d.noiseFloor += 0.06 * (rms - d.noiseFloor)
	} else {
		d.noiseFloor += 0.002 * (rms - d.noiseFloor)
	}
	if d.peakFloor == 0 || peak < d.peakFloor {
		d.peakFloor += 0.10 * (peak - d.peakFloor)
	} else {
		d.peakFloor += 0.002 * (peak - d.peakFloor)
	}
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
		mag := math.Log1p(math.Hypot(real, imag))
		delta := mag - d.prevBins[bin]
		if delta > 0 {
			freqWeight := 0.85 + 0.30*float64(bin)/float64(half)
			fluxSum += delta * freqWeight
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
		raw = clamp01(1 - math.Exp(-1.55*raw))

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
	fluxRaw = clamp01(fluxRaw * 1.25)
	if fluxRaw < d.fluxFloor {
		d.fluxFloor += 0.08 * (fluxRaw - d.fluxFloor)
	} else {
		d.fluxFloor += 0.01 * (fluxRaw - d.fluxFloor)
	}
	fluxSignal := adaptiveExcess(fluxRaw, d.fluxFloor, 0.12)

	rmsExcess := math.Max(0, rms-(d.noiseFloor*1.30+0.0012))
	peakExcess := math.Max(0, peak-(d.peakFloor*1.20+0.0025))
	levelTarget := clamp01(rmsExcess*5.4 + peakExcess*0.9)
	bassRise := math.Max(0, bassRaw-d.bass*0.92)
	midRise := math.Max(0, midRaw-d.midRange*0.92)
	transientDrive := clamp01(0.58*fluxSignal + 0.24*bassRise + 0.14*midRise)
	onsetRaw := transientDrive * clamp01(0.30+0.70*levelTarget)
	if onsetRaw < d.onsetFloor {
		d.onsetFloor += 0.10 * (onsetRaw - d.onsetFloor)
	} else {
		d.onsetFloor += 0.012 * (onsetRaw - d.onsetFloor)
	}
	onsetSignal := adaptiveExcess(onsetRaw, d.onsetFloor, 0.22)

	d.level = smoothAttackRelease(d.level, levelTarget, 0.18, 0.10)
	d.bass = smoothAttackRelease(d.bass, bassRaw, 0.18, 0.11)
	d.midRange = smoothAttackRelease(d.midRange, midRaw, 0.17, 0.11)
	d.treble = smoothAttackRelease(d.treble, trebleRaw, 0.19, 0.12)
	d.flux = smoothAttackRelease(d.flux, clamp01(fluxSignal*1.20), 0.20, 0.13)
	d.onset = smoothAttackRelease(d.onset, clamp01(onsetSignal*1.30), 0.20, 0.13)
	d.centroid = smoothAttackRelease(d.centroid, centroidNorm, 0.28, 0.12)

	d.pushOnset(d.onset)
	d.framesToBPM++
	if d.historyCount >= len(d.history)/2 && d.framesToBPM >= 8 {
		d.framesToBPM = 0
		if bpm, ok := d.estimateBPM(); ok {
			d.bpm = smoothAttackRelease(d.bpm, bpm, 0.20, 0.08)
		}
	}

	f := Features{
		Active:   d.level > 0.05 || d.flux > 0.08 || d.onset > 0.10,
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

func (d *detectorState) appendWaveform(samples []float64) {
	if len(samples) == 0 {
		return
	}

	stride := d.waveformStride
	if stride <= 0 {
		stride = maxInt(1, len(samples)/8)
	}
	for start := 0; start < len(samples); start += stride {
		end := start + stride
		if end > len(samples) {
			end = len(samples)
		}
		if end <= start {
			continue
		}

		mean := 0.0
		peak := 0.0
		for _, sample := range samples[start:end] {
			mean += sample
			if math.Abs(sample) > math.Abs(peak) {
				peak = sample
			}
		}
		mean /= float64(end - start)
		value := math.Tanh((0.75*mean + 0.25*peak) * 1.25)
		d.waveform[d.waveformIdx] = value
		d.waveformIdx = (d.waveformIdx + 1) % WaveformLen
	}
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

func adaptiveExcess(value, floor, margin float64) float64 {
	threshold := floor + margin
	if value <= threshold {
		return 0
	}
	return clamp01((value - threshold) / math.Max(1e-9, 1-threshold))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
