package visualizer

import (
	"hash/fnv"
	"math"
	"math/rand"
	"time"

	"cinder/audioinput"
	"cinder/config"
)

// VisMode selects which animation mode is active.
type VisMode int

const (
	ModeNebula   VisMode = iota // original nebula / particle cloud
	ModeWaveform                // scrolling waveform (highs & lows)
	ModeSpectrum                // frequency-bar spectrum analyser
	ModeVortex                  // spinning vortex reacting to bass/treble
	ModePulse                   // concentric beat-driven rings
	modeCount
)

func (m VisMode) String() string {
	switch m {
	case ModeNebula:
		return "Nebula"
	case ModeWaveform:
		return "Waveform"
	case ModeSpectrum:
		return "Spectrum"
	case ModeVortex:
		return "Vortex"
	case ModePulse:
		return "Pulse"
	}
	return "?"
}

type Particle struct {
	X          float64
	Y          float64
	VX         float64
	VY         float64
	Mass       float64
	Life       float64
	MaxLife    float64
	Decay      float64
	Orbit      float64
	Twist      float64
	Brightness float64
}

type Orbiter struct {
	angle   float64
	radius  float64
	speed   float64
	ellipse float64
	phase   float64
	x       float64
	y       float64
	prevX   float64
	prevY   float64
	bright  float64
	pull    float64
}

type AudioFeatures struct {
	Active   bool
	Level    float64
	Bass     float64
	Treble   float64
	MidRange float64
	Flux     float64
	Onset    float64
	Centroid float64
	BPM      float64

	// Rolling waveform (signed amplitude, oldest→newest)
	WaveformBuf [audioinput.WaveformLen]float64
	// Spectrum band energies [0..1] (sub-bass → high-treble)
	Spectrum [audioinput.SpectrumBands]float64
}

type System struct {
	particles []Particle
	orbiters  []Orbiter
	rnd       *rand.Rand
	palette   config.Palette

	width  int
	height int
	cx     float64
	cy     float64

	energy       float64
	targetEnergy float64
	shockwave    float64
	phase        float64
	songClock    float64
	bpm          float64
	rhythmOffset float64
	sectionLen   float64
	sectionMorph float64
	kick         float64
	snare        float64
	hat          float64
	voidRadius   float64
	profile      motionProfile
	audio        AudioFeatures
	trail        []pixel

	// animation mode
	mode VisMode

	// waveform display smoothing
	waveSmooth [audioinput.WaveformLen]float64
	waveVel    [audioinput.WaveformLen]float64
	// spectrum bar heights (display, smoothed)
	specSmooth [audioinput.SpectrumBands]float64
	specVel    [audioinput.SpectrumBands]float64
	// synthetic spectrum driven from beat clock (always-on fallback)
	synthSpec [audioinput.SpectrumBands]float64

	// pulse rings
	pulseRings []pulseRing
	prevKick   float64 // edge-detect: only spawn on rising edge
	prevOnset  float64
	// refractory clocks (songClock timestamps) so one beat can't double-fire
	lastRingClock  float64
	lastBurstClock float64

	// vortex state
	vortexPhase     float64
	vortexVel       float64 // rotational momentum
	vortexBassAngle float64 // slow bass-driven rotation offset
	audioPresence   float64
	audioLow        float64
	audioMid        float64
	audioHigh       float64

	// synthetic waveform oscillators (always-on, used when audio inactive)
	synthWavePhase [4]float64 // 4 oscillator phases

	// fluid neighbor grid (linked cells), rebuilt each Update
	fluidHead []int
	fluidNext []int

	// reusable render buffers (see framebuffer.go)
	frameBuf  []pixel
	outBuf    []byte
	glyphBand []byte // per-cell glyph band from last frame, for hysteresis
}

type pulseRing struct {
	radius float64
	life   float64 // 0..1
	color  config.RGB
	speed  float64
}

func NewSystem(count int) *System {
	s := &System{
		particles:    make([]Particle, count),
		rnd:          rand.New(rand.NewSource(time.Now().UnixNano())),
		palette:      config.DefaultPalette(),
		energy:       1,
		targetEnergy: 1,
		bpm:          120,
		sectionLen:   32,
		voidRadius:   4.5,
		profile: motionProfile{
			pace:     0.55,
			chaos:    0.45,
			trippy:   0.55,
			drift:    0.45,
			punch:    0.55,
			glow:     0.55,
			trail:    0.55,
			orbiters: 5,
			voidSize: 5.0,
		},
	}
	for i := range s.particles {
		s.particles[i] = s.spawn(true)
	}
	s.initOrbiters(6)
	return s
}

func (s *System) Resize(w, h int) {
	s.width = w
	s.height = h
	s.cx = float64(w) * 0.5
	s.cy = float64(h) * 0.5
	s.trail = make([]pixel, w*h)
	for i := range s.particles {
		s.particles[i] = s.spawn(false)
	}
	s.reseedOrbitersGeometry()
}

func (s *System) Mode() VisMode { return s.mode }

func (s *System) NextMode() VisMode {
	s.mode = (s.mode + 1) % modeCount
	return s.mode
}

func (s *System) SetPalette(p config.Palette) {
	s.palette = p
}

func (s *System) SetAudioFeatures(features AudioFeatures) {
	s.audio = features
}

func (s *System) SetSongSignature(songKey, track, artist string) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(songKey))
	seed := h.Sum64()

	s.profile = buildMotionProfile(seed, track, artist)
	s.songClock = 0
	s.bpm = 72 + s.profile.pace*92 + float64((seed>>6)%18)
	s.rhythmOffset = float64((seed>>8)%1000) / 1000.0
	s.sectionLen = 24 + float64((seed>>20)%36) // 24..59 beats
	s.sectionMorph = 0.5
	s.kick = 0
	s.snare = 0
	s.hat = 0
	s.lastRingClock = -10
	s.lastBurstClock = -10
	s.voidRadius = s.profile.voidSize + float64((seed>>30)%16)/20.0
	s.initOrbiters(s.profile.orbiters)
}

func (s *System) initOrbiters(n int) {
	if n < 3 {
		n = 3
	}
	s.orbiters = make([]Orbiter, n)
	for i := range s.orbiters {
		o := &s.orbiters[i]
		o.angle = s.rnd.Float64() * 2 * math.Pi
		o.radius = 6 + s.rnd.Float64()*18
		o.speed = 0.18 + s.rnd.Float64()*0.45
		o.ellipse = 0.55 + s.rnd.Float64()*0.65
		o.phase = s.rnd.Float64() * 2 * math.Pi
		o.bright = 0.6 + s.rnd.Float64()*0.7
		o.pull = 0.4 + s.rnd.Float64()*0.8
		o.x = s.cx
		o.y = s.cy
		o.prevX = s.cx
		o.prevY = s.cy
	}
	s.reseedOrbitersGeometry()
}

func (s *System) reseedOrbitersGeometry() {
	if len(s.orbiters) == 0 || s.width < 2 || s.height < 2 {
		return
	}
	base := math.Min(float64(s.width), float64(s.height)) * 0.23
	if base < 6 {
		base = 6
	}
	for i := range s.orbiters {
		o := &s.orbiters[i]
		o.radius = base*(0.42+0.85*s.rnd.Float64()) + float64(i)
		o.prevX = o.x
		o.prevY = o.y
	}
}

func (s *System) SetPlaying(playing bool) {
	if playing {
		s.targetEnergy = 1.0
	} else {
		s.targetEnergy = 0.0
	}
}

func (s *System) Explode() {
	s.shockwave = 1.0
	for i := range s.particles {
		a := s.rnd.Float64() * math.Pi * 2
		spd := 3.0 + s.rnd.Float64()*28.0
		s.particles[i].X = s.cx + (s.rnd.Float64()-0.5)*2
		s.particles[i].Y = s.cy + (s.rnd.Float64()-0.5)*2
		s.particles[i].VX = math.Cos(a) * spd
		s.particles[i].VY = math.Sin(a) * spd
		s.particles[i].Life = 0.8 + s.rnd.Float64()*1.8
		s.particles[i].MaxLife = s.particles[i].Life
		s.particles[i].Brightness = 0.7 + s.rnd.Float64()*0.5
	}
}

func (s *System) audioBurst(strength float64) {
	if len(s.particles) == 0 || strength <= 0 {
		return
	}
	count := 10 + int(math.Round(26*clamp01(strength)))
	if count > len(s.particles) {
		count = len(s.particles)
	}
	for i := 0; i < count; i++ {
		idx := s.rnd.Intn(len(s.particles))
		p := &s.particles[idx]
		a := s.rnd.Float64() * 2 * math.Pi
		speed := 5.0 + 24.0*strength + s.rnd.Float64()*8.0
		p.X = s.cx + (s.rnd.Float64()-0.5)*(3.0+6.0*strength)
		p.Y = s.cy + (s.rnd.Float64()-0.5)*(2.0+4.0*strength)
		p.VX += (math.Cos(a) * speed) / p.Mass
		p.VY += (math.Sin(a) * speed * (0.7 + 0.3*s.rnd.Float64())) / p.Mass
		p.Brightness = math.Max(p.Brightness, 1.0+0.8*strength)
		p.Life = math.Max(p.Life, 0.9+0.9*strength)
		p.MaxLife = math.Max(p.MaxLife, p.Life)
	}
}

func (s *System) Render() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	switch s.mode {
	case ModeWaveform:
		return s.renderWaveform()
	case ModeSpectrum:
		return s.renderSpectrum()
	case ModeVortex:
		return s.renderVortex()
	case ModePulse:
		return s.renderPulse()
	default:
		return s.renderNebula()
	}
}

func (s *System) spawn(initial bool) Particle {
	a := s.rnd.Float64() * math.Pi * 2
	r := math.Pow(s.rnd.Float64(), 1.65) * math.Min(float64(s.width), float64(s.height)) * 0.33
	if r < 1.0 {
		r = 1.0 + s.rnd.Float64()*2.0
	}

	x := s.cx + math.Cos(a)*r
	y := s.cy + math.Sin(a)*r*0.6

	base := 3.2 + s.rnd.Float64()*(5.4+7.0*s.profile.pace)
	if initial {
		base *= 0.95
	}

	return Particle{
		X:          x,
		Y:          y,
		VX:         math.Cos(a+math.Pi/2)*(base*(0.45+s.rnd.Float64())) + (s.rnd.Float64()-0.5)*(1.4+2.2*s.profile.chaos),
		VY:         math.Sin(a+math.Pi/2)*(base*(0.45+s.rnd.Float64())) + (s.rnd.Float64()-0.5)*(1.2+2.0*s.profile.chaos),
		Mass:       0.5 + s.rnd.Float64()*1.5,
		Life:       1.2 + s.rnd.Float64()*2.8,
		MaxLife:    1.2 + s.rnd.Float64()*2.8,
		Decay:      0.16 + s.rnd.Float64()*0.35,
		Orbit:      0.7 + s.rnd.Float64()*(1.8+1.8*s.profile.pace),
		Twist:      (s.rnd.Float64() - 0.5) * 2.0,
		Brightness: 0.8 + s.rnd.Float64()*(0.95+0.7*s.profile.glow),
	}
}
