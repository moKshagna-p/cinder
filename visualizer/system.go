package visualizer

import (
	"fmt"
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

type System struct {
	particles []Particle
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
	trail        []pixel
}

func NewSystem(count int) *System {
	s := &System{
		particles:    make([]Particle, count),
		rnd:          rand.New(rand.NewSource(time.Now().UnixNano())),
		palette:      config.DefaultPalette(),
		energy:       1,
		targetEnergy: 1,
	}
	for i := range s.particles {
		s.particles[i] = s.spawn(true)
	}
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
}

func (s *System) SetPalette(p config.Palette) {
	s.palette = p
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

	coreBreath := 0.8 + 0.2*math.Sin(s.phase*1.5)

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

		orbital := p.Orbit * (0.30 + 0.75*s.energy)
		corePull := -0.24 * coreBreath
		if s.energy > 0.6 {
			corePull += 0.15
		}
		jitter := (s.rnd.Float64() - 0.5) * (1.6 + 6.0*s.energy)

		ax := tx*orbital + rx*corePull + tx*jitter*0.25 + rx*jitter*0.5
		ay := ty*orbital + ry*corePull + ty*jitter*0.25 + ry*jitter*0.5

		if s.shockwave > 0.01 {
			shock := s.shockwave * math.Exp(-dist*0.05) * 11.0
			ax += rx * shock
			ay += ry * shock
		}

		damp := 0.996 - (1.0-s.energy)*0.08
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

		decay := p.Decay * (0.25 + 0.75*s.energy)
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
			s.trail[i].r *= 0.90
			s.trail[i].g *= 0.90
			s.trail[i].b *= 0.90
			s.trail[i].a *= 0.82
			b[i] = s.trail[i]
		}
	}
	addNebulaCloud(&b, s.width, s.height, s.cx, s.cy, s.palette, s.phase, s.energy)

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

		alpha := (0.35 + 1.05*age) * p.Brightness
		alpha *= 0.45 + 0.55*s.energy
		splat(&b, s.width, s.height, p.X, p.Y, c, alpha)
	}

	addCoreGlow(&b, s.width, s.height, s.cx, s.cy, s.palette, s.energy)
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

func addCoreGlow(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, energy float64) {
	radius := 5.5 + energy*4.0
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
			c := config.Mix(p.Core, p.Highlight, 0.3+0.4*energy)
			a := 0.42 * falloff * (0.65 + 0.35*energy)
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

func addNebulaCloud(buf *[]pixel, w, h int, cx, cy float64, p config.Palette, phase, energy float64) {
	if w < 4 || h < 4 {
		return
	}
	rx := math.Max(8, float64(w)*0.35)
	ry := math.Max(4, float64(h)*0.24)

	for y := 0; y < h; y++ {
		fy := (float64(y) - cy) / ry
		for x := 0; x < w; x++ {
			fx := (float64(x) - cx) / rx
			r2 := fx*fx + fy*fy
			if r2 > 2.4 {
				continue
			}

			theta := math.Atan2(fy, fx)
			ribbon := 0.5 + 0.5*math.Sin(theta*3.0+phase*1.2+r2*8.0)
			falloff := math.Exp(-r2 * (1.8 + 0.5*(1-energy)))
			a := falloff * (0.09 + 0.13*ribbon) * (0.35 + 0.65*energy)
			if a < 0.01 {
				continue
			}

			c := config.Mix(p.Outer, p.Mid, ribbon)
			idx := y*w + x
			(*buf)[idx] = blend((*buf)[idx], c, a)
		}
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
