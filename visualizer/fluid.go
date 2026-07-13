package visualizer

import "math"

// Local particle-particle fluid forces. Replaces an all-pairs N-body pass:
// same intent (audio level pulls clusters together, bass pressure pushes
// near neighbors apart) but O(n·k) via a uniform grid of linked cells, and
// bounded polynomial kernels instead of 1/d² singularities, so forces never
// saturate the acceleration clamp.
const (
	fluidRadius       = 7.0               // interaction radius, terminal-cell units
	fluidSepRadius    = fluidRadius * 0.4 // inside this, separation pressure wins
	fluidMaxNeighbors = 14                // hard cap so dense clumps stay O(n·k)
	fluidGridPad      = fluidRadius + 4   // particles roam a little off-screen
)

func (s *System) fluidCols() int { return int((float64(s.width)+2*fluidGridPad)/fluidRadius) + 1 }
func (s *System) fluidRows() int { return int((float64(s.height)+2*fluidGridPad)/fluidRadius) + 1 }

func (s *System) fluidCellFor(x, y float64) (int, int) {
	cols, rows := s.fluidCols(), s.fluidRows()
	cx := int((x + fluidGridPad) / fluidRadius)
	cy := int((y + fluidGridPad) / fluidRadius)
	if cx < 0 {
		cx = 0
	}
	if cx >= cols {
		cx = cols - 1
	}
	if cy < 0 {
		cy = 0
	}
	if cy >= rows {
		cy = rows - 1
	}
	return cx, cy
}

func (s *System) rebuildFluidGrid() {
	cells := s.fluidCols() * s.fluidRows()
	if len(s.fluidHead) < cells {
		s.fluidHead = make([]int, cells)
	}
	if len(s.fluidNext) < len(s.particles) {
		s.fluidNext = make([]int, len(s.particles))
	}
	head := s.fluidHead[:cells]
	for i := range head {
		head[i] = -1
	}
	cols := s.fluidCols()
	for i := range s.particles {
		cx, cy := s.fluidCellFor(s.particles[i].X, s.particles[i].Y)
		c := cy*cols + cx
		s.fluidNext[i] = head[c]
		head[c] = i
	}
}

// fluidForces returns the net pair force on particle i from its neighbors.
// The result is a force (scaled by neighbor mass); the caller divides by the
// particle's own mass.
func (s *System) fluidForces(i int, cohesionK, pressureK float64) (float64, float64) {
	p := &s.particles[i]
	cols, rows := s.fluidCols(), s.fluidRows()
	pcx, pcy := s.fluidCellFor(p.X, p.Y)

	var fx, fy float64
	checked := 0
	for gy := pcy - 1; gy <= pcy+1; gy++ {
		if gy < 0 || gy >= rows {
			continue
		}
		for gx := pcx - 1; gx <= pcx+1; gx++ {
			if gx < 0 || gx >= cols {
				continue
			}
			for j := s.fluidHead[gy*cols+gx]; j >= 0; j = s.fluidNext[j] {
				if j == i {
					continue
				}
				p2 := &s.particles[j]
				dx := p2.X - p.X
				dy := p2.Y - p.Y
				d2 := dx*dx + dy*dy
				if d2 >= fluidRadius*fluidRadius || d2 < 1e-6 {
					continue
				}
				d := math.Sqrt(d2)
				nx := dx / d
				ny := dy / d

				w := 1 - d/fluidRadius
				f := cohesionK * w * p2.Mass
				if d < fluidSepRadius {
					q := 1 - d/fluidSepRadius
					f -= pressureK * q * q * p2.Mass
				}
				fx += nx * f
				fy += ny * f

				checked++
				if checked >= fluidMaxNeighbors {
					return fx, fy
				}
			}
		}
	}
	return fx, fy
}
