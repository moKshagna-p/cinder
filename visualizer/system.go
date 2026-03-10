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

func (s *System) SetSongSignature(songKey string) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(songKey))
	seed := h.Sum64()

	s.songClock = 0
	s.bpm = 92 + float64(seed%49) // 92..140
	s.rhythmOffset = float64((seed>>8)%1000) / 1000.0
	s.sectionLen = 32 + float64((seed>>20)%24) // 32..55 beats
	s.sectionMorph = 0.5
	s.kick = 0
	s.snare = 0
	s.hat = 0
	s.voidRadius = 4.8 + float64((seed>>30)%26)/10.0
	s.initOrbiters(4 + int((seed>>14)%4))
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
	s.phase += dt * (0.6 + s.energy*1.8)
	s.shockwave *= math.Exp(-dt * 2.2)
	if s.energy > 0.02 {
		s.songClock += dt
	}

	beatsPerSec := s.bpm / 60.0
	beat := fract(s.songClock*beatsPerSec + s.rhythmOffset)
	bar := fract(s.songClock*beatsPerSec/4.0 + s.rhythmOffset*0.37)
	section := fract(s.songClock*beatsPerSec/s.sectionLen + s.rhythmOffset*0.17)

	// Keep a simple synthetic groove: kick + snare + soft hats.
	rawKick := pulse(beat, 0.00, 0.08)
	rawSnare := pulse(beat, 0.50, 0.07) * (0.85 + 0.15*math.Sin(2*math.Pi*bar))
	rawHat := 0.22 + 0.20*math.Sin(2*math.Pi*beat+2.1) + 0.10*math.Sin(4*math.Pi*beat+1.2)
	if rawHat < 0 {
		rawHat = 0
	}

	rhythmBlend := 1 - math.Exp(-dt*10.0)
	s.kick += (rawKick - s.kick) * rhythmBlend
	s.snare += (rawSnare - s.snare) * rhythmBlend
	s.hat += (rawHat - s.hat) * rhythmBlend
	s.sectionMorph = 0.5 + 0.5*math.Sin(2*math.Pi*section)

	coreBreath := 0.82 + 0.18*math.Sin(s.phase*1.1)
	rhythmDrive := 0.95*s.kick + 0.45*s.snare
	voidPulse := 0.7 + 0.3*s.kick

	for i := range s.orbiters {
		o := &s.orbiters[i]
		o.prevX = o.x
		o.prevY = o.y
		o.angle += dt * (o.speed + 0.24*s.kick + 0.08*s.sectionMorph)
		r := o.radius * (0.84 + 0.18*s.sectionMorph + 0.16*voidPulse)
		ex := math.Cos(o.angle + o.phase)
		ey := math.Sin(o.angle+o.phase) * o.ellipse
		o.x = s.cx + ex*r
		o.y = s.cy + ey*r*0.62
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

		orbital := p.Orbit * (0.22 + 0.52*s.energy + 0.26*s.sectionMorph)
		corePull := -0.18 * coreBreath
		if s.energy > 0.6 {
			corePull += 0.10
		}
		corePull += 0.20 * s.kick
		wave := math.Sin(s.phase*1.4 + p.Twist*2.2 + dist*0.05)
		drift := 0.30 * wave * (0.2 + 0.8*s.hat)

		ax := tx*orbital + rx*corePull + tx*drift
		ay := ty*orbital + ry*corePull + ty*drift

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
			pull := (0.28 + 0.45*s.snare) * o.pull / d2
			swirl := (0.18 + 0.28*s.hat) * o.pull / d2
			ax += ox*pull + swirlX*swirl
			ay += oy*pull + swirlY*swirl
		}

		if s.shockwave > 0.01 {
			shock := s.shockwave * math.Exp(-dist*0.05) * 8.0
			ax += rx * shock
			ay += ry * shock
		}
		ax += rx * (0.24 + 2.0*rhythmDrive) * math.Exp(-dist*0.04)
		ay += ry * (0.24 + 2.0*rhythmDrive) * math.Exp(-dist*0.04)

		damp := 0.994 - (1.0-s.energy)*0.06
		if damp < 0.82 {
			damp = 0.82
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
			colorFade := 0.90 + 0.04*s.sectionMorph
			alphaFade := 0.80 + 0.05*s.hat
			s.trail[i].r *= colorFade
			s.trail[i].g *= colorFade
			s.trail[i].b *= colorFade
			s.trail[i].a *= alphaFade
			b[i] = s.trail[i]
		}
	}
	addNebulaCloud(&b, s.width, s.height, s.cx, s.cy, s.palette, s.phase, s.energy, s.sectionMorph, s.kick, s.snare)
	applyVoid(&b, s.width, s.height, s.cx, s.cy, s.voidRadius*(0.9+0.45*s.kick))
	drawOrbiters(&b, s.width, s.height, s.palette, s.orbiters, 0.45+0.55*s.energy)

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

		alpha := (0.28 + 0.86*age) * p.Brightness
		alpha *= 0.40 + 0.56*s.energy
		alpha *= 0.90 + 0.50*s.kick + 0.18*s.snare
		splat(&b, s.width, s.height, p.X, p.Y, c, alpha)
	}

	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy, s.kick, s.snare)
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

	base := 4.0 + s.rnd.Float64()*7.0
	if initial {
		base *= 0.95
	}

	return Particle{
		X:          x,
		Y:          y,
		VX:         math.Cos(a+math.Pi/2)*(base*(0.4+s.rnd.Float64())) + (s.rnd.Float64()-0.5)*2.0,
		VY:         math.Sin(a+math.Pi/2)*(base*(0.4+s.rnd.Float64())) + (s.rnd.Float64()-0.5)*2.0,
		Life:       1.2 + s.rnd.Float64()*2.8,
		MaxLife:    1.2 + s.rnd.Float64()*2.8,
		Decay:      0.16 + s.rnd.Float64()*0.35,
		Orbit:      0.8 + s.rnd.Float64()*2.4,
		Twist:      (s.rnd.Float64() - 0.5) * 2.0,
		Brightness: 0.9 + s.rnd.Float64()*1.25,
	}
}

func addCoreGlow(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, energy, kick, snare float64) {
	radius := 5.0 + energy*3.2 + kick*2.0
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
			c := config.Mix(p.Core, p.Highlight, clamp01(0.26+0.34*energy+0.28*snare))
			a := 0.36 * falloff * (0.58 + 0.30*energy + 0.40*kick)
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

func addNebulaCloud(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, phase, energy, sectionMorph, kick, snare float64) {
	if w < 4 || h < 4 {
		return
	}
	rx := math.Max(8, float64(w)*(0.30+0.06*sectionMorph))
	ry := math.Max(4, float64(h)*(0.22+0.05*(1-sectionMorph)))

	for y := 0; y < h; y++ {
		fy := (float64(y) - cy) / ry
		for x := 0; x < w; x++ {
			fx := (float64(x) - cx) / rx
			r2 := fx*fx + fy*fy
			if r2 > 2.4 {
				continue
			}

			theta := math.Atan2(fy, fx)
			ribbon := 0.5 + 0.5*math.Sin(theta*(1.8+1.4*sectionMorph)+phase*(0.8+0.4*kick)+r2*(5.0+2.0*snare))
			falloff := math.Exp(-r2 * (1.8 + 0.5*(1-energy)))
			a := falloff * (0.06 + 0.11*ribbon) * (0.30 + 0.62*energy) * (0.90 + 0.22*snare)
			if a < 0.01 {
				continue
			}

			c := config.Mix(p.Outer, p.Mid, ribbon)
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

func drawOrbiters(buf *[]pixel, w, h int, p config.Palette, orbiters []Orbiter, energy float64) {
	for i := range orbiters {
		o := orbiters[i]
		trace := config.Mix(p.Highlight, p.Core, 0.30+0.20*float64(i%3))
		traceA := (0.06 + 0.14*o.bright) * (0.35 + 0.65*energy)
		drawLineGlow(buf, w, h, o.prevX, o.prevY, o.x, o.y, trace, traceA)
		head := config.Mix(p.Highlight, p.Mid, 0.35)
		splat(buf, w, h, o.x, o.y, head, (0.28+0.55*o.bright)*(0.45+0.55*energy))
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
