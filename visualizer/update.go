package visualizer

import (
	"math"

	"cinder/audioinput"
	"cinder/config"
)

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

	// Phase-lock the synthetic clock to real onsets: when a strong onset
	// lands off the synthetic beat, pull songClock toward it so the groove
	// and the music stop fighting each other instead of layering two
	// slightly-offset rhythms.
	onsetRising := s.audio.Active && s.audio.Onset > 0.58 && s.prevOnset <= 0.58
	if onsetRising && beatsPerSec > 0 {
		phaseErr := fract(s.songClock*beatsPerSec + s.rhythmOffset)
		if phaseErr > 0.5 {
			phaseErr -= 1
		}
		s.songClock -= 0.30 * phaseErr / beatsPerSec
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

	audioLow := s.kick
	audioMid := s.snare
	audioHigh := s.hat
	if s.audio.Active {
		var lowSum, midSum, highSum float64
		for i := 0; i < audioinput.SpectrumBands; i++ {
			v := s.audio.Spectrum[i]
			switch {
			case i <= 3:
				lowSum += v
			case i <= 10:
				midSum += v
			default:
				highSum += v
			}
		}
		audioLow = clamp01(lowSum / 4.0)
		audioMid = clamp01(midSum / 7.0)
		audioHigh = clamp01(highSum / 5.0)
	}

	s.audioPresence *= math.Exp(-dt * 3.2)
	if s.audio.Active {
		audioKick := clamp01(0.14*s.audio.Level + 0.56*audioLow + 0.20*s.audio.Onset + 0.04*s.audio.Flux)
		audioSnare := clamp01(0.12*s.audio.Level + 0.46*audioMid + 0.16*s.audio.Onset + 0.08*s.audio.Flux)
		audioHat := clamp01(0.08*s.audio.Level + 0.50*audioHigh + 0.12*s.audio.Centroid + 0.08*s.audio.Flux)
		audioBlend := clamp01(0.52 + 0.18*s.audioPresence)
		rawKick = mix(rawKick*0.40, audioKick, audioBlend)
		rawSnare = mix(rawSnare*0.42, audioSnare, audioBlend)
		rawHat = mix(rawHat*0.45, audioHat, audioBlend)
		s.audioPresence = math.Max(s.audioPresence, clamp01(0.58*s.audio.Level+0.34*audioLow+0.20*s.audio.Onset+0.12*s.audio.Flux))
		if s.audio.Onset > 0.44 {
			s.shockwave = math.Max(s.shockwave, 0.04+0.26*s.audio.Onset)
		}
	}

	rhythmBlend := 1 - math.Exp(-dt*(3.4+2.2*s.profile.punch+2.8*s.audioPresence))
	s.kick += (rawKick - s.kick) * rhythmBlend
	s.snare += (rawSnare - s.snare) * rhythmBlend
	s.hat += (rawHat - s.hat) * rhythmBlend
	s.audioLow = s.audioLow + (audioLow-s.audioLow)*(1-math.Exp(-dt*5.0))
	s.audioMid = s.audioMid + (audioMid-s.audioMid)*(1-math.Exp(-dt*5.5))
	s.audioHigh = s.audioHigh + (audioHigh-s.audioHigh)*(1-math.Exp(-dt*6.0))
	s.sectionMorph = 0.5 + 0.5*math.Sin(2*math.Pi*section+math.Pi*s.profile.trippy)
	if onsetRising && (s.songClock-s.lastBurstClock)*beatsPerSec >= 0.45 {
		s.lastBurstClock = s.songClock
		s.audioBurst(clamp01(0.14*s.audio.Onset + 0.06*s.audioHigh + 0.10*s.audioLow))
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

	// Field forces fade out with energy so pause actually freezes the
	// cloud (README behavior) instead of turbulence pumping it forever.
	forceDrive := 0.06 + 0.94*s.energy

	s.rebuildFluidGrid()
	// Cohesion (audio level) gathers loose clusters; separation pressure
	// (bass) pushes near neighbors apart. Both are true forces: scaled by
	// the neighbor's mass here, divided by the particle's own mass below.
	cohesionK := 0.20 + 0.40*s.audio.Level
	pressureK := 1.20 + 2.50*s.audio.Bass

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

		// --- turbulence field: multi-scale noise ---
		// Creates fluid-like swirling motion that varies with chaos/trippy profile
		tPhase := s.phase * 0.65
		turbScale := 0.12 + 0.25*s.profile.chaos
		turbStrength := 0.35 + 1.2*s.profile.chaos + 0.35*s.audio.Flux

		// Coarse turbulence (large-scale flow)
		ax += math.Sin(p.Y*turbScale+tPhase) * turbStrength
		ay += math.Cos(p.X*turbScale+tPhase) * turbStrength

		// Fine turbulence (micro-swirls)
		ax += math.Sin(p.Y*turbScale*3.1-tPhase*1.4) * turbStrength * 0.35
		ay += math.Cos(p.X*turbScale*3.1-tPhase*1.4) * turbStrength * 0.35

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
			shock := s.shockwave * math.Exp(-dist*0.05) * (3.2 + 3.6*s.profile.punch)
			ax += rx * shock
			ay += ry * shock
		}
		audioPush := 0.0
		if s.audio.Active {
			audioPush = 0.11*s.audioLow + 0.03*s.audio.Flux + 0.02*s.audio.Onset
		}
		ax += rx * (0.16 + (1.2+1.6*s.profile.punch)*rhythmDrive + audioPush) * math.Exp(-dist*(0.03+0.01*s.profile.drift))
		ay += ry * (0.16 + (1.2+1.6*s.profile.punch)*rhythmDrive + audioPush) * math.Exp(-dist*(0.03+0.01*s.profile.drift))

		if s.audio.Active {
			spin := (0.05 + 0.16*s.audioMid + 0.10*s.audio.Centroid) * math.Exp(-dist*0.04)
			ax += tx * spin
			ay += ty * spin
		}

		// --- local fluid forces (cohesion + separation pressure) ---
		fluidFx, fluidFy := s.fluidForces(i, cohesionK, pressureK)
		ax += fluidFx / p.Mass
		ay += fluidFy / p.Mass

		drag := 0.72 + 0.30*s.profile.pace + (1.0-s.energy)*1.8
		damp := math.Exp(-drag * dt)
		if damp < 0.58 {
			damp = 0.58
		}

		// Realistic aerodynamic drag force proportional to v^2
		speedSq := p.VX*p.VX + p.VY*p.VY
		if speedSq > 0.01 {
			speed := math.Sqrt(speedSq)
			dragCoef := (0.004 + 0.002*s.profile.pace) * drag
			ax -= (dragCoef * p.VX * speed) / p.Mass
			ay -= (dragCoef * p.VY * speed) / p.Mass
		}

		ax *= forceDrive
		ay *= forceDrive
		ax = clampSigned(ax, 26.0)
		ay = clampSigned(ay, 26.0)
		p.VX = (p.VX + ax*dt*48.0) * damp
		p.VY = (p.VY + ay*dt*48.0) * damp
		maxV := 34.0 + 18.0*s.profile.pace + 8.0*s.audioLow
		p.VX = clampSigned(p.VX, maxV)
		p.VY = clampSigned(p.VY, maxV)

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

	// --- vortex phase: always spinning, bass accelerates it with inertia ---
	bassDriver := s.kick*0.8 + s.snare*0.3 + s.hat*0.15
	if s.audio.Active {
		bassDriver = clamp01(0.25*bassDriver + 0.65*s.audio.Bass + 0.10*s.audio.Flux)
	}
	targetVortexVel := 0.8 + 2.5*s.profile.pace + 3.0*bassDriver
	// Add "weight" to the vortex: it doesn't just track bass, it has momentum
	inertia := 1 - math.Exp(-dt*(4.0+2.0*s.profile.punch))
	s.vortexVel += (targetVortexVel - s.vortexVel) * inertia
	s.vortexPhase += dt * s.vortexVel
	s.vortexBassAngle += dt * (0.15 + 0.9*s.kick + 0.3*s.snare)

	// --- pulse rings: fire on rising edge of kick, at most once per beat ---
	ringReady := (s.songClock-s.lastRingClock)*beatsPerSec >= 0.45
	if s.kick > 0.60 && s.prevKick <= 0.60 && ringReady {
		s.lastRingClock = s.songClock
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
		// Expansion easing: starts fast, slows down as it ages
		expansion := 0.2 + 0.8*math.Exp(-(1.0-r.life)*3.5)
		r.radius += r.speed * dt * expansion
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

	// --- smooth waveform / spectrum for display with damped spring physics ---
	// waveform: blend audio (if active) with synthetic oscillators
	waveFollow := 1 - math.Exp(-dt*45)
	for i := 0; i < audioinput.WaveformLen; i++ {
		if s.audio.Active {
			// The audio buffer scrolls under the display every hop, so a
			// spring per column chases a moving target and smears
			// transients into vertical wobble. Track the real waveform
			// directly with a fast blend instead.
			s.waveSmooth[i] += (s.audio.WaveformBuf[i] - s.waveSmooth[i]) * waveFollow
			s.waveVel[i] = 0
			continue
		}

		fi := float64(i) / float64(audioinput.WaveformLen-1) // 0..1 left to right
		// synthetic wave: sum of oscillators, spatially varying
		synthSample := 0.0
		synthSample += math.Sin(s.synthWavePhase[0]+fi*math.Pi*2*(1.0+s.profile.chaos)) * (0.5 + 0.5*s.kick)
		synthSample += math.Sin(s.synthWavePhase[1]+fi*math.Pi*4*s.profile.trippy) * (0.3 + 0.3*s.snare)
		synthSample += math.Sin(s.synthWavePhase[2]+fi*math.Pi*6*(0.5+s.profile.chaos)) * (0.2 + 0.2*s.hat)
		synthSample += math.Sin(s.synthWavePhase[3]+fi*math.Pi*8*s.profile.trippy) * 0.1
		synthSample *= 0.8 // normalise so it stays in roughly ±1
		target := synthSample * (0.4 + 0.6*s.energy)

		// damped spring: gentle organic bounce for the synthetic wave
		const stiffness = 120.0
		const damping = 18.0
		accel := (target - s.waveSmooth[i]) * stiffness
		s.waveVel[i] += accel * dt
		s.waveVel[i] *= math.Max(0, 1.0-damping*dt)
		s.waveSmooth[i] += s.waveVel[i] * dt
	}
	for i := 0; i < audioinput.SpectrumBands; i++ {
		var target float64
		if s.audio.Active {
			target = s.audio.Spectrum[i]
		} else {
			target = s.synthSpec[i]
		}

		// Damped Spring for spectrum bars: gives them "weight" and "bounce"
		stiffness := 350.0
		damping := 26.0
		if target < s.specSmooth[i] {
			// slow decay (lower stiffness/damping when falling)
			stiffness = 140.0
			damping = 16.0
		}

		accel := (target - s.specSmooth[i]) * stiffness
		s.specVel[i] += accel * dt
		s.specVel[i] *= math.Max(0, 1.0-damping*dt)
		s.specSmooth[i] += s.specVel[i] * dt
	}
}
