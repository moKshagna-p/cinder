package visualizer

import "math"

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func clampSigned(v, maxAbs float64) float64 {
	if maxAbs <= 0 {
		return 0
	}
	if v > maxAbs {
		return maxAbs
	}
	if v < -maxAbs {
		return -maxAbs
	}
	return v
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
