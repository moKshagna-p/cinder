package config

import (
	"crypto/sha1"
	"encoding/binary"
	"math"
)

type RGB struct {
	R float64
	G float64
	B float64
}

type Palette struct {
	Core      RGB
	Mid       RGB
	Outer     RGB
	Highlight RGB
}

func PaletteFromSong(title string) Palette {
	h := sha1.Sum([]byte(title))
	seed := binary.BigEndian.Uint64(h[:8])

	baseHue := float64(seed%360) / 360.0
	satA := 0.70 + float64((seed>>8)%20)/100.0
	satB := 0.55 + float64((seed>>16)%25)/100.0
	valA := 0.85 + float64((seed>>24)%14)/100.0
	valB := 0.55 + float64((seed>>32)%20)/100.0

	core := hsvToRGB(baseHue, satA, min(1.0, valA))
	mid := hsvToRGB(fract(baseHue+0.08), satB, 0.75)
	outer := hsvToRGB(fract(baseHue+0.16), 0.50, valB)
	highlight := hsvToRGB(fract(baseHue+0.52), 0.45, 1.0)

	return Palette{Core: core, Mid: mid, Outer: outer, Highlight: highlight}
}

func DefaultPalette() Palette {
	return Palette{
		Core:      RGB{R: 1.00, G: 0.74, B: 0.30},
		Mid:       RGB{R: 1.00, G: 0.45, B: 0.20},
		Outer:     RGB{R: 0.75, G: 0.20, B: 0.15},
		Highlight: RGB{R: 0.95, G: 0.95, B: 1.00},
	}
}

func Mix(a, b RGB, t float64) RGB {
	t = clamp01(t)
	return RGB{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
	}
}

func fract(x float64) float64 {
	return x - math.Floor(x)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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

func hsvToRGB(h, s, v float64) RGB {
	h = fract(h)
	s = clamp01(s)
	v = clamp01(v)

	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)

	switch i % 6 {
	case 0:
		return RGB{R: v, G: t, B: p}
	case 1:
		return RGB{R: q, G: v, B: p}
	case 2:
		return RGB{R: p, G: v, B: t}
	case 3:
		return RGB{R: p, G: q, B: v}
	case 4:
		return RGB{R: t, G: p, B: v}
	default:
		return RGB{R: v, G: p, B: q}
	}
}
