package kernels

import "math"

const nParticles = 256

// fpKernel is scalar float64 particle integration around an attractor.
type fpKernel struct {
	px, py, pz, vx, vy, vz [nParticles]float64
	i                      int
}

func newFP() *fpKernel {
	k := &fpKernel{}
	for i := 0; i < nParticles; i++ {
		f := float64(i)
		k.px[i], k.py[i], k.pz[i] = math.Sin(f)*10, math.Cos(f)*10, f/32
		k.vx[i], k.vy[i], k.vz[i] = 0.1, -0.1, 0.05
	}
	return k
}

func (k *fpKernel) Name() string { return "fp" }

func (k *fpKernel) Run(units int) uint64 {
	const dt, g = 0.01, 4.0
	i := k.i
	for n := units * fpItersPerUnit; n > 0; n-- {
		dx, dy, dz := k.px[i], k.py[i], k.pz[i]
		d := math.Sqrt(dx*dx+dy*dy+dz*dz) + 1
		f := g / (d * d * d)
		k.vx[i] -= dx * f * dt
		k.vy[i] -= dy * f * dt
		k.vz[i] -= dz * f * dt
		k.px[i] += k.vx[i] * dt
		k.py[i] += k.vy[i] * dt
		k.pz[i] += k.vz[i] * dt
		i = (i + 1) & (nParticles - 1)
	}
	k.i = i
	return math.Float64bits(k.px[i]+k.py[i]+k.pz[i]) | 1
}
