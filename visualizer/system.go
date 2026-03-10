package visualizer

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
	"time"

	"cinder/config"
)

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
	Active bool
	Level  float64
	Bass   float64
	Treble float64
	Flux   float64
	BPM    float64
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
	if s.audio.Active {
		audioKick := clamp01(0.30*s.audio.Level + 1.05*s.audio.Bass + 0.95*s.audio.Flux)
		audioSnare := clamp01(0.22*s.audio.Level + 0.55*s.audio.Bass + 1.10*s.audio.Flux)
		audioHat := clamp01(0.30*s.audio.Level + 0.85*s.audio.Treble + 0.45*s.audio.Flux)
		audioBlend := clamp01(0.35 + 0.55*s.audio.Level + 0.35*s.audio.Flux)
		rawKick = mix(rawKick, audioKick, audioBlend)
		rawSnare = mix(rawSnare, audioSnare, audioBlend)
		rawHat = mix(rawHat, audioHat, clamp01(audioBlend+0.10))
		if s.audio.Flux > 0.55 {
			s.shockwave = math.Max(s.shockwave, 0.18+0.45*s.audio.Flux)
		}
	}

	rhythmBlend := 1 - math.Exp(-dt*(9.0+7.0*s.profile.punch))
	s.kick += (rawKick - s.kick) * rhythmBlend
	s.snare += (rawSnare - s.snare) * rhythmBlend
	s.hat += (rawHat - s.hat) * rhythmBlend
	s.sectionMorph = 0.5 + 0.5*math.Sin(2*math.Pi*section+math.Pi*s.profile.trippy)

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
		drift := (0.14 + 0.42*s.profile.trippy) * wave * (0.25 + 0.75*s.hat + 0.22*s.audio.Treble)
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
			audioPush = 0.35*s.audio.Bass + 0.18*s.audio.Flux
		}
		ax += rx * (0.16 + (1.2+1.6*s.profile.punch)*rhythmDrive + audioPush) * math.Exp(-dist*(0.03+0.01*s.profile.drift))
		ay += ry * (0.16 + (1.2+1.6*s.profile.punch)*rhythmDrive + audioPush) * math.Exp(-dist*(0.03+0.01*s.profile.drift))

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
}

func (s *System) Render() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}

	b := make([]pixel, s.width*s.height)
	if len(s.trail) == len(b) {
		for i := range s.trail {
			colorFade := 0.82 + 0.12*s.profile.trail + 0.03*s.sectionMorph
			alphaFade := 0.64 + 0.18*s.profile.trail + 0.06*s.hat
			s.trail[i].r *= colorFade
			s.trail[i].g *= colorFade
			s.trail[i].b *= colorFade
			s.trail[i].a *= alphaFade
			b[i] = s.trail[i]
		}
	}
	addNebulaCloud(&b, s.width, s.height, s.cx, s.cy, s.palette, s.phase, s.energy, s.sectionMorph, s.kick, s.snare, s.profile)
	applyVoid(&b, s.width, s.height, s.cx, s.cy, s.voidRadius*(0.85+0.50*s.kick))
	drawOrbiters(&b, s.width, s.height, s.palette, s.orbiters, 0.40+0.60*s.energy, s.profile)

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
		if age > 0.85 {
			c = config.Mix(c, s.palette.Highlight, (age-0.85)/0.15)
		}
		if s.snare > 0.2 {
			c = config.Mix(c, s.palette.Highlight, clamp01(0.12+0.38*s.snare))
		}

		alpha := (0.22 + 0.78*age) * p.Brightness
		alpha *= 0.30 + 0.48*s.energy + 0.18*s.profile.glow
		alpha *= 0.78 + 0.46*s.kick + 0.16*s.snare
		splat(&b, s.width, s.height, p.X, p.Y, c, alpha)
	}

	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, s.kick, s.snare, s.profile)
	if len(s.trail) == len(b) {
		copy(s.trail, b)
	}

	var out strings.Builder
	out.Grow((s.width + 1) * s.height * 4)
	out.WriteString("\x1b[48;2;0;0;0m")
	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			p := b[y*s.width+x]
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
		if y < s.height-1 {
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
