package visualizer

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
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
	Life       float64
	MaxLife    float64
	Decay      float64
	Orbit      float64
	Twist      float64
	Brightness float64
}

type pixel struct {
	r float64
	g float64
	b float64
	a float64
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
	// spectrum bar heights (display, smoothed)
	specSmooth [audioinput.SpectrumBands]float64
	// synthetic spectrum driven from beat clock (always-on fallback)
	synthSpec [audioinput.SpectrumBands]float64

	// pulse rings
	pulseRings []pulseRing
	prevKick   float64 // edge-detect: only spawn on rising edge
	prevOnset  float64

	// vortex state
	vortexPhase     float64
	vortexBassAngle float64 // slow bass-driven rotation offset
	audioPresence   float64

	// synthetic waveform oscillators (always-on, used when audio inactive)
	synthWavePhase [4]float64 // 4 oscillator phases
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
		p.VX += math.Cos(a) * speed
		p.VY += math.Sin(a) * speed * (0.7 + 0.3*s.rnd.Float64())
		p.Brightness = math.Max(p.Brightness, 1.0+0.8*strength)
		p.Life = math.Max(p.Life, 0.9+0.9*strength)
		p.MaxLife = math.Max(p.MaxLife, p.Life)
	}
}

func (s *System) Update(dt float64) {
	if s.width < 2 || s.height < 2 {
		return
	}

	blendRate := 1 - math.Exp(-dt*2.5)
	s.energy += (s.targetEnergy - s.energy) * blendRate
	speedScale := 0.60 + 1.70*s.profile.pace
	tripScale := 0.50 + 1.40*s.profile.trippy
	chaosScale := 0.30 + 1.35*s.profile.chaos
	s.phase += dt * (0.20 + s.energy*0.80 + speedScale + 0.45*tripScale)
	s.shockwave *= math.Exp(-dt * 2.2)
	if s.energy > 0.02 {
		s.songClock += dt
	}

	beatsPerSec := s.bpm / 60.0
	if s.audio.Active && s.audio.BPM >= 60 {
		beatsPerSec = (0.30*s.bpm + 0.70*s.audio.BPM) / 60.0
	}
	beat := fract(s.songClock*beatsPerSec + s.rhythmOffset)
	bar := fract(s.songClock*beatsPerSec/4.0 + s.rhythmOffset*0.37)
	section := fract(s.songClock*beatsPerSec/s.sectionLen + s.rhythmOffset*0.17)
	subBeat := fract(s.songClock*beatsPerSec*(1.4+2.8*s.profile.pace) + s.rhythmOffset*0.63)

	// Keep the groove synthetic, but vary the pulse shape and subdivision per song.
	rawKick := pulse(beat, 0.00, 0.12-0.05*s.profile.pace) * (0.85 + 0.15*math.Sin(2*math.Pi*(bar+0.12*s.profile.chaos)))
	rawSnare := pulse(beat, 0.50, 0.10-0.04*s.profile.pace) * (0.75 + 0.25*math.Sin(2*math.Pi*(bar+0.20*s.profile.trippy)))
	rawHat := 0.10 + 0.22*pulse(subBeat, 0.0, 0.20-0.06*s.profile.pace)
	rawHat += 0.14 * math.Sin(2*math.Pi*subBeat+2.1)
	rawHat += 0.12 * math.Sin(4*math.Pi*beat+1.2+1.8*s.profile.trippy)
	if rawHat < 0 {
		rawHat = 0
	}
	s.audioPresence *= math.Exp(-dt * 3.2)
	if s.audio.Active {
		audioKick := clamp01(0.20*s.audio.Level + 1.30*s.audio.Bass + 1.10*s.audio.Onset)
		audioSnare := clamp01(0.18*s.audio.Level + 0.72*s.audio.MidRange + 0.95*s.audio.Onset + 0.22*s.audio.Flux)
		audioHat := clamp01(0.18*s.audio.Level + 0.95*s.audio.Treble + 0.55*s.audio.Centroid + 0.38*s.audio.Flux)
		audioBlend := clamp01(0.25 + 0.55*s.audio.Level + 0.50*s.audio.Onset + 0.20*s.audio.Flux)
		rawKick = mix(rawKick, audioKick, audioBlend)
		rawSnare = mix(rawSnare, audioSnare, clamp01(audioBlend+0.08))
		rawHat = mix(rawHat, audioHat, clamp01(audioBlend+0.12))
		s.audioPresence = math.Max(s.audioPresence, clamp01(0.60*s.audio.Level+0.85*s.audio.Onset+0.35*s.audio.Flux))
		if s.audio.Onset > 0.18 {
			s.shockwave = math.Max(s.shockwave, 0.12+0.75*s.audio.Onset)
		}
	}

	rhythmBlend := 1 - math.Exp(-dt*(9.0+7.0*s.profile.punch))
	s.kick += (rawKick - s.kick) * rhythmBlend
	s.snare += (rawSnare - s.snare) * rhythmBlend
	s.hat += (rawHat - s.hat) * rhythmBlend
	s.sectionMorph = 0.5 + 0.5*math.Sin(2*math.Pi*section+math.Pi*s.profile.trippy)
	if s.audio.Active && s.audio.Onset > 0.26 && s.prevOnset <= 0.26 {
		s.audioBurst(clamp01(0.45*s.audio.Onset + 0.25*s.audio.Treble + 0.20*s.audio.Bass))
	}
	s.prevOnset = s.audio.Onset

	coreBreath := 0.78 + 0.22*math.Sin(s.phase*(0.7+0.9*s.profile.drift))
	if s.audio.Active {
		coreBreath += 0.10 * s.audio.Level
	}
	rhythmDrive := (0.70+0.60*s.profile.punch)*s.kick + (0.30+0.40*s.profile.chaos)*s.snare
	voidPulse := 0.7 + 0.3*s.kick

	for i := range s.orbiters {
		o := &s.orbiters[i]
		o.prevX = o.x
		o.prevY = o.y
		o.angle += dt * (o.speed*(0.65+speedScale) + 0.30*s.kick + 0.10*s.sectionMorph + 0.06*tripScale)
		r := o.radius * (0.70 + 0.22*s.sectionMorph + 0.18*voidPulse + 0.18*s.profile.trippy)
		ex := math.Cos(o.angle + o.phase)
		ey := math.Sin(o.angle+o.phase) * (o.ellipse + 0.10*s.profile.chaos)
		wobbleX := math.Sin(s.phase*0.55+o.phase*1.7) * (1.2 + 3.2*s.profile.trippy)
		wobbleY := math.Cos(s.phase*0.60+o.phase*1.3) * (0.7 + 2.1*s.profile.trippy)
		o.x = s.cx + ex*r + wobbleX
		o.y = s.cy + ey*r*(0.42+0.24*s.profile.trippy) + wobbleY
	}

	for i := range s.particles {
		p := &s.particles[i]

		dx := p.X - s.cx
		dy := p.Y - s.cy
		dist := math.Hypot(dx, dy)
		if dist < 0.001 {
			dist = 0.001
		}

		tx := -dy / dist
		ty := dx / dist
		rx := dx / dist
		ry := dy / dist

		orbital := p.Orbit * (0.16 + 0.42*s.energy + 0.24*s.sectionMorph + 0.34*s.profile.pace)
		corePull := -0.16 * coreBreath * (0.75 + 0.65*s.profile.drift)
		if s.energy > 0.6 {
			corePull += 0.10
		}
		corePull += 0.16 * s.kick
		wave := math.Sin(s.phase*(1.0+1.6*s.profile.trippy) + p.Twist*(1.6+1.8*s.profile.chaos) + dist*(0.03+0.05*s.profile.trippy))
		drift := (0.14 + 0.42*s.profile.trippy) * wave * (0.25 + 0.75*s.hat + 0.28*s.audio.Treble + 0.12*s.audio.Centroid)
		shear := math.Sin(s.phase*0.45+p.Twist*3.2+dist*0.07) * (0.06 + 0.18*chaosScale)

		ax := tx*orbital + rx*corePull + tx*drift + ry*shear
		ay := ty*orbital + ry*corePull + ty*drift - rx*shear

		for j := range s.orbiters {
			o := &s.orbiters[j]
			odx := o.x - p.X
			ody := o.y - p.Y
			d2 := odx*odx + ody*ody + 0.7
			invDist := 1.0 / math.Sqrt(d2)
			ox := odx * invDist
			oy := ody * invDist
			swirlX := -oy
			swirlY := ox
			pull := (0.18 + 0.26*s.profile.drift + 0.35*s.snare) * o.pull / d2
			swirl := (0.10 + 0.30*s.profile.trippy + 0.25*s.hat) * o.pull / d2
			ax += ox*pull + swirlX*swirl
			ay += oy*pull + swirlY*swirl
		}

		if s.shockwave > 0.01 {
			shock := s.shockwave * math.Exp(-dist*0.05) * (6.0 + 6.0*s.profile.punch)
			ax += rx * shock
			ay += ry * shock
		}
		audioPush := 0.0
		if s.audio.Active {
			audioPush = 0.42*s.audio.Bass + 0.18*s.audio.Flux + 0.10*s.audio.Onset
		}
		ax += rx * (0.16 + (1.2+1.6*s.profile.punch)*rhythmDrive + audioPush) * math.Exp(-dist*(0.03+0.01*s.profile.drift))
		ay += ry * (0.16 + (1.2+1.6*s.profile.punch)*rhythmDrive + audioPush) * math.Exp(-dist*(0.03+0.01*s.profile.drift))

		if s.audio.Active {
			spin := (0.08 + 0.24*s.audio.MidRange + 0.16*s.audio.Centroid) * math.Exp(-dist*0.04)
			ax += tx * spin
			ay += ty * spin
		}

		damp := 0.988 - 0.014*s.profile.pace - (1.0-s.energy)*0.06
		if damp < 0.79 {
			damp = 0.79
		}

		p.VX = (p.VX + ax*dt*60.0) * damp
		p.VY = (p.VY + ay*dt*60.0) * damp

		if s.energy < 0.05 {
			p.VX *= 0.92
			p.VY *= 0.92
		}

		p.X += p.VX * dt
		p.Y += p.VY * dt

		decay := p.Decay * (0.20 + 0.64*s.energy + 0.18*s.snare)
		if s.energy < 0.1 {
			decay = p.Decay * 0.1
		}
		p.Life -= dt * decay

		out := p.X < -2 || p.X > float64(s.width+2) || p.Y < -2 || p.Y > float64(s.height+2)
		if p.Life <= 0 || out {
			*p = s.spawn(false)
		}
	}

	// --- vortex phase: always spinning, bass accelerates it ---
	bassDriver := s.kick*0.8 + s.snare*0.3 + s.hat*0.15
	if s.audio.Active {
		bassDriver = clamp01(bassDriver + s.audio.Bass*1.2 + s.audio.Flux*0.4)
	}
	s.vortexPhase += dt * (0.8 + 2.5*s.profile.pace + 3.0*bassDriver)
	s.vortexBassAngle += dt * (0.15 + 0.9*s.kick + 0.3*s.snare)

	// --- pulse rings: fire on rising edge of kick (once per beat) ---
	if s.kick > 0.45 && s.prevKick <= 0.45 {
		maxR := math.Min(float64(s.width)*0.5, float64(s.height))
		ringStrength := 0.55 + 0.45*s.kick
		c := config.Mix(s.palette.Core, s.palette.Highlight, clamp01(s.kick))
		s.pulseRings = append(s.pulseRings, pulseRing{
			radius: s.voidRadius * 1.2,
			life:   1.0,
			color:  c,
			speed:  maxR * (0.25 + 0.40*ringStrength),
		})
		// snare fires a second smaller ring on the backbeat
		if s.snare > 0.35 {
			c2 := config.Mix(s.palette.Mid, s.palette.Highlight, s.snare)
			s.pulseRings = append(s.pulseRings, pulseRing{
				radius: s.voidRadius * 0.8,
				life:   0.75,
				color:  c2,
				speed:  maxR * (0.15 + 0.25*s.snare),
			})
		}
	}
	s.prevKick = s.kick
	alive := s.pulseRings[:0]
	for i := range s.pulseRings {
		r := &s.pulseRings[i]
		r.radius += r.speed * dt
		r.life -= dt * (0.55 + 0.45*s.profile.pace)
		if r.life > 0 {
			alive = append(alive, *r)
		}
	}
	s.pulseRings = alive

	// --- synthetic waveform oscillators (drive motion even without audio) ---
	// Slower rates so the waveform breathes gently with the song
	rates := [4]float64{
		beatsPerSec * 0.25,                          // very slow fundamental — one full wave per 4 beats
		beatsPerSec * 0.5,                           // half-beat shimmer
		beatsPerSec * (0.12 + s.profile.trippy*0.4), // ultra-slow drift
		beatsPerSec * (0.8 + s.profile.chaos*0.6),   // mild high-freq texture
	}
	for i := range s.synthWavePhase {
		s.synthWavePhase[i] += dt * rates[i] * 2 * math.Pi
	}

	// --- synthetic spectrum bands (always-on, scaled by beat signals) ---
	for b := 0; b < audioinput.SpectrumBands; b++ {
		bf := float64(b) / float64(audioinput.SpectrumBands-1) // 0=bass, 1=treble
		// low bands react to kick/bass, high bands react to hat/treble
		beatDrive := mix(s.kick*(1.0+s.snare*0.5), s.hat*(0.8+s.snare*0.3), bf)
		// add phase-modulated shimmer that varies per band
		shimmer := 0.5 + 0.5*math.Sin(s.phase*(1.0+bf*4.0)+float64(b)*0.7)
		synth := clamp01(beatDrive*(0.6+0.4*shimmer) + 0.08*shimmer*(0.3+0.7*s.energy))
		s.synthSpec[b] = synth
	}

	// --- smooth waveform / spectrum for display ---
	// waveform: blend audio (if active) with synthetic oscillators
	for i := 0; i < audioinput.WaveformLen; i++ {
		fi := float64(i) / float64(audioinput.WaveformLen-1) // 0..1 left to right
		// synthetic wave: sum of oscillators, spatially varying
		synthSample := 0.0
		synthSample += math.Sin(s.synthWavePhase[0]+fi*math.Pi*2*(1.0+s.profile.chaos)) * (0.5 + 0.5*s.kick)
		synthSample += math.Sin(s.synthWavePhase[1]+fi*math.Pi*4*s.profile.trippy) * (0.3 + 0.3*s.snare)
		synthSample += math.Sin(s.synthWavePhase[2]+fi*math.Pi*6*(0.5+s.profile.chaos)) * (0.2 + 0.2*s.hat)
		synthSample += math.Sin(s.synthWavePhase[3]+fi*math.Pi*8*s.profile.trippy) * 0.1
		synthSample *= 0.8 // normalise so it stays in roughly ±1

		var target float64
		if s.audio.Active {
			target = s.audio.WaveformBuf[i]*0.92 + synthSample*0.08
		} else {
			target = synthSample * (0.4 + 0.6*s.energy)
		}
		s.waveSmooth[i] += (target - s.waveSmooth[i]) * clamp01(dt*8)
	}
	for i := 0; i < audioinput.SpectrumBands; i++ {
		var target float64
		if s.audio.Active {
			target = s.audio.Spectrum[i]*0.90 + s.synthSpec[i]*0.10
		} else {
			target = s.synthSpec[i]
		}
		// fast attack, slow decay
		if target > s.specSmooth[i] {
			s.specSmooth[i] += (target - s.specSmooth[i]) * clamp01(dt*28)
		} else {
			s.specSmooth[i] += (target - s.specSmooth[i]) * clamp01(dt*7)
		}
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

// renderNebula is the original particle-cloud rendering path.
func (s *System) renderNebula() string {
	b := make([]pixel, s.width*s.height)
	bassPulse := s.kick
	midDrive := s.snare
	trebleShimmer := s.hat
	onsetFlash := 0.0
	brightness := 0.0
	centroidTint := 0.0
	if s.audio.Active {
		audioWeight := clamp01(0.35 + 0.65*s.audioPresence)
		bassPulse = mix(bassPulse, s.audio.Bass, audioWeight)
		midDrive = mix(midDrive, s.audio.MidRange, audioWeight)
		trebleShimmer = mix(trebleShimmer, s.audio.Treble, audioWeight)
		onsetFlash = s.audio.Onset
		brightness = s.audio.Level
		centroidTint = s.audio.Centroid
	} else {
		onsetFlash = s.kick
		brightness = 0.55*s.energy + 0.45*s.kick
		centroidTint = s.hat
	}
	if len(s.trail) == len(b) {
		for i := range s.trail {
			colorFade := 0.80 + 0.10*s.profile.trail + 0.04*brightness
			alphaFade := 0.60 + 0.16*s.profile.trail + 0.10*brightness + 0.08*trebleShimmer
			s.trail[i].r *= colorFade
			s.trail[i].g *= colorFade
			s.trail[i].b *= colorFade
			s.trail[i].a *= alphaFade
			b[i] = s.trail[i]
		}
	}
	addNebulaCloud(&b, s.width, s.height, s.cx, s.cy, s.palette, s.phase, s.energy, s.sectionMorph, bassPulse, midDrive, s.profile)
	applyVoid(&b, s.width, s.height, s.cx, s.cy, s.voidRadius*(0.82+0.55*bassPulse))
	drawOrbiters(&b, s.width, s.height, s.palette, s.orbiters, 0.35+0.55*s.energy+0.25*midDrive, s.profile)

	for _, p := range s.particles {
		age := p.Life / p.MaxLife
		if age < 0 {
			age = 0
		}
		if age > 1 {
			age = 1
		}

		dx := p.X - s.cx
		dy := p.Y - s.cy
		radiusNorm := math.Hypot(dx, dy) / math.Max(4, math.Min(float64(s.width), float64(s.height))*0.5)
		if radiusNorm > 1 {
			radiusNorm = 1
		}

		c := config.Mix(s.palette.Core, s.palette.Mid, radiusNorm*0.8)
		if radiusNorm > 0.55 {
			c = config.Mix(c, s.palette.Outer, (radiusNorm-0.55)/0.45)
		}
		if centroidTint > 0 {
			c = config.Mix(c, s.palette.Highlight, clamp01(centroidTint*(0.18+0.42*radiusNorm)))
		}
		if age > 0.85 {
			c = config.Mix(c, s.palette.Highlight, (age-0.85)/0.15)
		}
		if onsetFlash > 0.12 {
			c = config.Mix(c, s.palette.Highlight, clamp01(0.10+0.55*onsetFlash))
		}

		alpha := (0.22 + 0.78*age) * p.Brightness
		alpha *= 0.28 + 0.48*s.energy + 0.18*s.profile.glow + 0.16*brightness
		alpha *= 0.76 + 0.42*bassPulse + 0.22*onsetFlash + 0.10*trebleShimmer
		splat(&b, s.width, s.height, p.X, p.Y, c, alpha)
	}

	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, bassPulse, onsetFlash, s.profile)
	if len(s.trail) == len(b) {
		copy(s.trail, b)
	}
	return pixelBufToString(b, s.width, s.height)
}

// renderWaveform draws a live waveform that always moves with the beat.
// When audio is active it shows real amplitude; otherwise synthetic oscillators drive it.
func (s *System) renderWaveform() string {
	b := make([]pixel, s.width*s.height)
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
	scale := mid * (0.35 + 0.55*audioLevel + 0.20*s.kick)

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

		// primary wave position
		ys := mid - sample*scale

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

			alpha := math.Max(edgeAlpha, fillAlpha) * (0.55 + 0.55*audioLevel)
			b[y*s.width+x] = blend(b[y*s.width+x], c, clamp01(alpha))
		}

		// bright dot on the wave surface
		dotY := int(math.Round(ys))
		if dotY >= 0 && dotY < s.height {
			b[dotY*s.width+x] = blend(b[dotY*s.width+x], s.palette.Highlight, 0.85+0.15*s.kick)
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
	if s.kick > 0.65 {
		flashAlpha := (s.kick - 0.65) * 1.5
		for y := 0; y < s.height; y++ {
			x := s.width / 2
			b[y*s.width+x] = blend(b[y*s.width+x], s.palette.Highlight, flashAlpha*0.4)
		}
	}

	return pixelBufToString(b, s.width, s.height)
}

// renderSpectrum draws animated frequency bars, always moving with the beat.
func (s *System) renderSpectrum() string {
	b := make([]pixel, s.width*s.height)
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

	for band := 0; band < bands; band++ {
		energy := s.specSmooth[band]
		bandNorm := float64(band) / float64(bands-1) // 0=bass, 1=treble

		// bar height scales with energy and overall kick boost
		barH := int(math.Round(energy * float64(s.height) * (0.88 + 0.15*s.kick)))
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
	if s.kick > 0.50 {
		for x := 0; x < s.width; x++ {
			b[0*s.width+x] = blend(b[0*s.width+x], s.palette.Highlight, (s.kick-0.50)*0.9)
		}
	}

	return pixelBufToString(b, s.width, s.height)
}

// renderVortex draws a spinning vortex fully driven by the beat clock and audio.
func (s *System) renderVortex() string {
	b := make([]pixel, s.width*s.height)
	// terminal cells are roughly 2× taller than wide — correct for circle
	const aX = 1.0
	const aY = 2.0

	maxR := math.Min(float64(s.width)/aX, float64(s.height)/aY) * 0.49

	arms := 3 + int(math.Round(s.profile.trippy*3+s.profile.pace*2))

	// beat-reactive modulation values (always driven from synthetic clock)
	kickMod := s.kick
	snareMod := s.snare
	hatMod := s.hat
	bassMod := s.kick*0.7 + s.snare*0.3
	trebleMod := s.hat
	if s.audio.Active {
		bassMod = s.audio.Bass*0.6 + kickMod*0.4
		trebleMod = s.audio.Treble*0.6 + hatMod*0.4
	}

	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			fx := (float64(x) - s.cx) / aX
			fy := (float64(y) - s.cy) / aY
			r := math.Hypot(fx, fy)
			if r > maxR || r < 0.5 {
				continue
			}

			theta := math.Atan2(fy, fx)
			rNorm := r / maxR

			// primary spiral: phase-driven rotation + radial twist
			rot := s.vortexPhase + s.vortexBassAngle*bassMod
			spiral := theta + rot + rNorm*math.Pi*(2.5+3.5*s.profile.chaos+2.0*bassMod)

			// arm brightness: multiple harmonics of the spiral angle
			arm1 := math.Cos(float64(arms) * spiral)
			arm2 := math.Cos(float64(arms)*spiral*1.5 + trebleMod*math.Pi)
			arm3 := math.Sin(float64(arms+1)*spiral + snareMod*math.Pi*0.7)
			v := 0.50*arm1 + 0.30*arm2 + 0.20*arm3
			if v < 0 {
				v = 0
			}
			v = math.Pow(v, 0.5) // soften the arms so fill is visible

			// outer edge shimmer (hat/treble makes outer ring glitter)
			if rNorm > 0.7 {
				outer := math.Sin(float64(arms*2)*spiral + s.phase*2.0 + trebleMod*4.0)
				v += clamp01(outer) * (rNorm - 0.7) * trebleMod * 0.6
			}

			// radial falloff: tighter when energy is low, expands on beat
			fallExp := 1.5 - 0.6*s.energy - 0.5*kickMod
			falloff := math.Exp(-rNorm * fallExp)
			a := v * falloff * (0.12 + 0.60*s.energy) * (1.0 + 0.5*kickMod + 0.2*snareMod)

			// colour: inner warm → outer cool, with beat tinting
			c := config.Mix(s.palette.Core, s.palette.Mid, rNorm)
			if rNorm > 0.45 {
				c = config.Mix(c, s.palette.Highlight, (rNorm-0.45)/0.55)
			}
			// bass paints the inner glow warm, treble adds cool outer shimmer
			c = config.Mix(c, s.palette.Outer, clamp01(trebleMod*rNorm*0.6))
			c = config.Mix(c, s.palette.Core, clamp01(bassMod*(1-rNorm)*0.5))

			b[y*s.width+x] = blend(b[y*s.width+x], c, clamp01(a))
		}
	}

	// core glow pulsing on kick
	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, kickMod, snareMod, s.profile)
	return pixelBufToString(b, s.width, s.height)
}

// renderPulse draws concentric rings expanding on every beat — always animated.
func (s *System) renderPulse() string {
	b := make([]pixel, s.width*s.height)

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
			splat(&b, s.width, s.height, px, py, c, opacity)
		}
	}

	// --- expanding beat rings ---
	for _, ring := range s.pulseRings {
		alpha := ring.life * ring.life * 0.90
		if alpha < 0.01 {
			continue
		}
		rX := ring.radius
		rY := ring.radius / aY

		angStep := math.Max(0.008, 0.025-ring.radius*0.0002)
		for angle := 0.0; angle < 2*math.Pi; angle += angStep {
			px := s.cx + math.Cos(angle)*rX
			py := s.cy + math.Sin(angle)*rY
			splat(&b, s.width, s.height, px, py, ring.color, alpha)
		}
		// slightly thicker ring (second pass at slight offset)
		for angle := 0.0; angle < 2*math.Pi; angle += angStep {
			px := s.cx + math.Cos(angle)*(rX+1.5)
			py := s.cy + math.Sin(angle)*(rY+0.75)
			splat(&b, s.width, s.height, px, py, ring.color, alpha*0.5)
		}
	}

	// --- core glow ---
	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, s.kick, s.snare, s.profile)
	return pixelBufToString(b, s.width, s.height)
}

// pixelBufToString converts a pixel buffer into an ANSI-colored string.
func pixelBufToString(b []pixel, w, h int) string {
	var out strings.Builder
	out.Grow((w + 1) * h * 4)
	out.WriteString("\x1b[48;2;0;0;0m")
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := b[y*w+x]
			if p.a <= 0.01 {
				out.WriteByte(' ')
				continue
			}
			ir := int(clamp01(p.r) * 255)
			ig := int(clamp01(p.g) * 255)
			ib := int(clamp01(p.b) * 255)
			glyph := glyphFor(p.a)
			out.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm%c", ir, ig, ib, glyph))
		}
		if y < h-1 {
			out.WriteByte('\n')
		}
	}
	out.WriteString("\x1b[0m")
	return out.String()
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
		Life:       1.2 + s.rnd.Float64()*2.8,
		MaxLife:    1.2 + s.rnd.Float64()*2.8,
		Decay:      0.16 + s.rnd.Float64()*0.35,
		Orbit:      0.7 + s.rnd.Float64()*(1.8+1.8*s.profile.pace),
		Twist:      (s.rnd.Float64() - 0.5) * 2.0,
		Brightness: 0.8 + s.rnd.Float64()*(0.95+0.7*s.profile.glow),
	}
}

func addCoreGlow(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, energy, kick, snare float64, profile motionProfile) {
	radius := 4.0 + energy*2.4 + kick*(1.2+1.8*profile.punch) + profile.glow*2.2
	for oy := -7; oy <= 7; oy++ {
		for ox := -14; ox <= 14; ox++ {
			x := int(cx) + ox
			y := int(cy) + oy
			if x < 0 || x >= w || y < 0 || y >= h {
				continue
			}
			d := math.Hypot(float64(ox)*0.6, float64(oy))
			if d > radius {
				continue
			}
			falloff := 1 - d/radius
			c := config.Mix(p.Core, p.Highlight, clamp01(0.18+0.28*energy+0.30*snare+0.18*profile.trippy))
			a := 0.28 * falloff * (0.42 + 0.28*energy + 0.26*kick + 0.28*profile.glow)
			idx := y*w + x
			(*buf)[idx] = blend((*buf)[idx], c, a)
		}
	}
}

func splat(buf *[]pixel, w, h int, x, y float64, c config.RGB, alpha float64) {
	ix := int(math.Round(x))
	iy := int(math.Round(y))

	for oy := -2; oy <= 2; oy++ {
		for ox := -2; ox <= 2; ox++ {
			tx := ix + ox
			ty := iy + oy
			if tx < 0 || tx >= w || ty < 0 || ty >= h {
				continue
			}

			d := math.Hypot(float64(ox), float64(oy))
			if d > 2.2 {
				continue
			}
			wgt := 1 - d/2.2
			idx := ty*w + tx
			(*buf)[idx] = blend((*buf)[idx], c, alpha*wgt)
		}
	}
}

func addNebulaCloud(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, phase, energy, sectionMorph, kick, snare float64, profile motionProfile) {
	if w < 4 || h < 4 {
		return
	}
	rx := math.Max(8, float64(w)*(0.28+0.08*sectionMorph+0.03*profile.trippy))
	ry := math.Max(4, float64(h)*(0.18+0.08*(1-sectionMorph)+0.04*profile.drift))

	for y := 0; y < h; y++ {
		fy := (float64(y) - cy) / ry
		for x := 0; x < w; x++ {
			fx := (float64(x) - cx) / rx
			r2 := fx*fx + fy*fy
			if r2 > 2.4 {
				continue
			}

			theta := math.Atan2(fy, fx)
			ribbon := 0.5 + 0.5*math.Sin(theta*(1.1+2.8*profile.trippy)+phase*(0.5+0.9*profile.pace)+r2*(4.0+3.4*profile.chaos))
			wave := 0.5 + 0.5*math.Sin((fx-fy)*(4.2+4.8*profile.trippy)-phase*(0.3+0.7*profile.pace)+theta*(0.8+1.6*sectionMorph))
			field := 0.58*ribbon + 0.42*wave
			falloff := math.Exp(-r2 * (1.8 + 0.5*(1-energy) + 0.2*(1-profile.trail)))
			a := falloff * (0.04 + 0.10*field) * (0.18 + 0.58*energy) * (0.88 + 0.18*snare + 0.12*profile.glow)
			if a < 0.01 {
				continue
			}

			c := config.Mix(p.Outer, p.Mid, 0.20+0.60*field)
			if profile.trippy > 0.5 {
				c = config.Mix(c, p.Highlight, (profile.trippy-0.5)*0.35+0.20*wave)
			}
			idx := y*w + x
			(*buf)[idx] = blend((*buf)[idx], c, a)
		}
	}
}

func applyVoid(buf *[]pixel, w, h int, cx, cy, radius float64) {
	if radius < 1 {
		return
	}
	x0 := int(cx - radius - 2)
	x1 := int(cx + radius + 2)
	y0 := int(cy - radius - 2)
	y1 := int(cy + radius + 2)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > w-1 {
		x1 = w - 1
	}
	if y1 > h-1 {
		y1 = h - 1
	}

	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			d := math.Hypot(dx, dy*1.25)
			if d > radius {
				continue
			}
			t := 1 - d/radius
			dim := 1 - 0.96*t*t
			idx := y*w + x
			(*buf)[idx].r *= dim
			(*buf)[idx].g *= dim
			(*buf)[idx].b *= dim
			(*buf)[idx].a *= dim
		}
	}
}

func drawOrbiters(buf *[]pixel, w, h int, p config.Palette, orbiters []Orbiter, energy float64, profile motionProfile) {
	for i := range orbiters {
		o := orbiters[i]
		trace := config.Mix(p.Highlight, p.Core, 0.18+0.18*float64(i%3)+0.18*profile.trippy)
		traceA := (0.04 + 0.08*o.bright) * (0.28 + 0.56*energy + 0.18*profile.glow)
		drawLineGlow(buf, w, h, o.prevX, o.prevY, o.x, o.y, trace, traceA)
		head := config.Mix(p.Highlight, p.Mid, 0.25+0.25*profile.trippy)
		splat(buf, w, h, o.x, o.y, head, (0.16+0.30*o.bright)*(0.30+0.44*energy+0.16*profile.glow))
	}
}

func drawLineGlow(buf *[]pixel, w, h int, x0, y0, x1, y1 float64, c config.RGB, alpha float64) {
	dx := x1 - x0
	dy := y1 - y0
	steps := int(math.Hypot(dx, dy) * 1.4)
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + dx*t
		y := y0 + dy*t
		fade := 1 - t*0.7
		splat(buf, w, h, x, y, c, alpha*fade)
	}
}

func blend(dst pixel, c config.RGB, a float64) pixel {
	a = clamp01(a)
	dst.r += c.R * a
	dst.g += c.G * a
	dst.b += c.B * a
	dst.a += a
	return dst
}

func glyphFor(v float64) byte {
	switch {
	case v < 0.08:
		return '.'
	case v < 0.16:
		return ':'
	case v < 0.28:
		return '-'
	case v < 0.42:
		return '*'
	case v < 0.58:
		return 'o'
	case v < 0.78:
		return 'O'
	default:
		return '@'
	}
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

func mix(a, b, t float64) float64 {
	t = clamp01(t)
	return a + (b-a)*t
}

func fract(x float64) float64 {
	return x - math.Floor(x)
}

func pulse(phase, center, width float64) float64 {
	d := math.Abs(phase - center)
	if d > 0.5 {
		d = 1 - d
	}
	if width <= 0 {
		return 0
	}
	n := d / width
	return math.Exp(-n * n * 3.4)
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
