package audioinput

import "math"

func normalizeFrameSize(n int) int {
	if n < 256 {
		n = 256
	}
	size := 1
	for size < n {
		size <<= 1
	}
	return size
}

func hannWindow(n int) []float64 {
	w := make([]float64, n)
	if n <= 1 {
		if n == 1 {
			w[0] = 1
		}
		return w
	}
	for i := 0; i < n; i++ {
		w[i] = 0.5 - 0.5*math.Cos((2*math.Pi*float64(i))/float64(n-1))
	}
	return w
}

func fft(real, imag []float64) {
	n := len(real)
	if n <= 1 {
		return
	}

	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j &^= bit
		}
		j |= bit
		if i < j {
			real[i], real[j] = real[j], real[i]
			imag[i], imag[j] = imag[j], imag[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wLenReal := math.Cos(angle)
		wLenImag := math.Sin(angle)
		half := length >> 1

		for i := 0; i < n; i += length {
			wReal := 1.0
			wImag := 0.0
			for j := 0; j < half; j++ {
				uReal := real[i+j]
				uImag := imag[i+j]
				vReal := real[i+j+half]*wReal - imag[i+j+half]*wImag
				vImag := real[i+j+half]*wImag + imag[i+j+half]*wReal

				real[i+j] = uReal + vReal
				imag[i+j] = uImag + vImag
				real[i+j+half] = uReal - vReal
				imag[i+j+half] = uImag - vImag

				nextReal := wReal*wLenReal - wImag*wLenImag
				wImag = wReal*wLenImag + wImag*wLenReal
				wReal = nextReal
			}
		}
	}
}

func logBandEdges(sampleRate, frameSize int) [SpectrumBands + 1]int {
	var edges [SpectrumBands + 1]int
	nyquistBin := frameSize / 2
	if nyquistBin < 2 {
		for i := range edges {
			edges[i] = 0
		}
		return edges
	}

	minHz := 20.0
	maxHz := math.Min(float64(sampleRate)/2, 12000)
	if maxHz <= minHz {
		maxHz = float64(sampleRate) / 2
	}
	binHz := float64(sampleRate) / float64(frameSize)

	for i := 0; i <= SpectrumBands; i++ {
		t := float64(i) / float64(SpectrumBands)
		hz := minHz * math.Pow(maxHz/minHz, t)
		bin := int(math.Round(hz / binHz))
		if bin < 1 {
			bin = 1
		}
		if bin > nyquistBin-1 {
			bin = nyquistBin - 1
		}
		edges[i] = bin
	}

	edges[0] = 1
	edges[SpectrumBands] = nyquistBin
	for i := 1; i < len(edges); i++ {
		if edges[i] <= edges[i-1] {
			edges[i] = edges[i-1] + 1
			if edges[i] > nyquistBin {
				edges[i] = nyquistBin
			}
		}
	}

	return edges
}
