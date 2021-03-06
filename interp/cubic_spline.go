package interp

import (
	"fmt"
	"sort"

	"github.com/unixpickle/num-analysis/kahan"
	"github.com/unixpickle/num-analysis/linalg"
	"github.com/unixpickle/num-analysis/linalg/ludecomp"
)

// A SplineStyle determines how the shape of
// a spline interpolation is computed.
// Different styles ensure different properties
// of the resulting spline.
type SplineStyle int

// These are the valid SplineStyle values.
const (
	// StandardStyle generates the default type
	// of spline without any unusual properties.
	StandardStyle SplineStyle = iota

	// MidArcStyle generates a spline whose slopes
	// are computed using the two points on either
	// side of a given point without considering
	// the point itself.
	MidArcStyle

	// MonotoneStyle forces the generated spline
	// to retain the original data's monotonicity.
	MonotoneStyle
)

// A CubicFunc represents a cubic function
// of a single variable.
//
// The four numbers [a b c d] in a CubicFunc
// correspond to a + bx + cx^2 + dx^3.
type CubicFunc [4]float64

// Eval evaluates the cubic function at a
// point x.
func (f CubicFunc) Eval(x float64) float64 {
	sum := kahan.NewSummer64()
	prod := 1.0
	for _, coeff := range f[:] {
		sum.Add(coeff * prod)
		prod *= x
	}
	return sum.Sum()
}

// Deriv evaluates the derivative of the
// cubic function at a point x.
func (f CubicFunc) Deriv(x float64) float64 {
	return f[1] + 2*f[2]*x + 3*f[3]*x*x
}

// Integ evaluates the definite integral
// of this function on an interval.
func (f CubicFunc) Integ(x1, x2 float64) float64 {
	return f.indefIntegral(x2) - f.indefIntegral(x1)
}

func (f CubicFunc) indefIntegral(x float64) float64 {
	x2 := x * x
	x3 := x2 * x
	x4 := x3 * x
	return f[0]*x + 1.0/2.0*f[1]*x2 + 1.0/3.0*f[2]*x3 + 1.0/4.0*f[3]*x4
}

// A CubicSpline is a piecewise function made up of
// cubic components, designed to have be continuous
// up to the first derivative.
type CubicSpline struct {
	style  SplineStyle
	x      []float64
	y      []float64
	slopes []float64
	funcs  []CubicFunc
}

// NewCubicSpline creates a CubicSpline with no pieces.
func NewCubicSpline(style SplineStyle) *CubicSpline {
	return &CubicSpline{style: style}
}

// Add adds a point to the cubic spline, generating
// a new piece and affecting old pieces in the process.
func (c *CubicSpline) Add(x, y float64) {
	idx := sort.SearchFloat64s(c.x, x)

	c.x = append(c.x, 0)
	copy(c.x[idx+1:], c.x[idx:])
	c.y = append(c.y, 0)
	copy(c.y[idx+1:], c.y[idx:])
	c.slopes = append(c.slopes, 0)
	copy(c.slopes[idx+1:], c.slopes[idx:])

	c.x[idx] = x
	c.y[idx] = y

	if len(c.x) > 1 {
		c.funcs = append(c.funcs, CubicFunc{})
		if idx != len(c.funcs) {
			copy(c.funcs[idx+1:], c.funcs[idx:])
		}
	}

	c.updateSlope(idx)
	if idx > 0 {
		c.updateSlope(idx - 1)
		c.updateFunc(idx - 1)
		if idx > 1 {
			c.updateFunc(idx - 2)
		}
	}
	if idx < len(c.slopes)-1 {
		c.updateSlope(idx + 1)
		c.updateFunc(idx)
		if idx < len(c.slopes)-2 {
			c.updateFunc(idx + 1)
		}
	}
}

// Eval evaluates the cubic spline at a given point.
func (c *CubicSpline) Eval(x float64) float64 {
	if yCount := len(c.y); yCount == 1 {
		return c.y[0]
	} else if yCount == 0 {
		return 0
	}

	idx := c.funcIndexForX(x)
	return c.funcs[idx].Eval(x)
}

// Deriv evaluates the derivative of the cubic
// spline at a given point.
func (c *CubicSpline) Deriv(x float64) float64 {
	if len(c.y) < 2 {
		return 0
	}

	idx := c.funcIndexForX(x)
	return c.funcs[idx].Deriv(x)
}

// Integ evaluates the definite integral of the
// cubic spline on an interval.
func (c *CubicSpline) Integ(x1, x2 float64) float64 {
	if x1 == x2 {
		return 0
	} else if x1 > x2 {
		return -c.Integ(x2, x1)
	}
	if l := len(c.y); l == 1 {
		return c.y[0] * (x2 - x1)
	} else if l == 0 {
		return 0
	}

	startIdx := c.funcIndexForX(x1)
	endIdx := c.funcIndexForX(x2)

	sum := kahan.NewSummer64()
	for i := startIdx; i <= endIdx; i++ {
		startX := c.x[i]
		endX := c.x[i+1]
		if i == startIdx {
			startX = x1
		}
		if i == endIdx {
			endX = x2
		}
		sum.Add(c.funcs[i].Integ(startX, endX))
	}
	return sum.Sum()
}

func (c *CubicSpline) funcIndexForX(x float64) int {
	idx := sort.SearchFloat64s(c.x, x) - 1
	if idx < 0 {
		idx = 0
	} else if idx >= len(c.funcs) {
		idx = len(c.funcs) - 1
	}
	return idx
}

func (c *CubicSpline) updateSlope(idx int) {
	switch c.style {
	case StandardStyle:
		c.slopes[idx] = c.computeStandardSlope(idx)
	case MidArcStyle:
		c.slopes[idx] = c.computeMidArcSlope(idx)
	case MonotoneStyle:
		c.slopes[idx] = c.computeMonotoneSlope(idx)
	default:
		panic(fmt.Sprintf("unknown style: %d", c.style))
	}
}

func (c *CubicSpline) computeStandardSlope(idx int) float64 {
	if len(c.x) < 2 {
		return 0
	}
	if idx == 0 {
		return (c.y[1] - c.y[0]) / (c.x[1] - c.x[0])
	} else if last := len(c.x) - 1; idx == last {
		return (c.y[last] - c.y[last-1]) / (c.x[last] - c.x[last-1])
	}
	m1 := (c.y[idx] - c.y[idx-1]) / (c.x[idx] - c.x[idx-1])
	m2 := (c.y[idx+1] - c.y[idx]) / (c.x[idx+1] - c.x[idx])
	return (m1 + m2) / 2
}

func (c *CubicSpline) computeMidArcSlope(idx int) float64 {
	if len(c.x) < 2 {
		return 0
	}
	if idx == 0 {
		return (c.y[1] - c.y[0]) / (c.x[1] - c.x[0])
	} else if last := len(c.x) - 1; idx == last {
		return (c.y[last] - c.y[last-1]) / (c.x[last] - c.x[last-1])
	}
	return (c.y[idx+1] - c.y[idx-1]) / (c.x[idx+1] - c.x[idx-1])
}

func (c *CubicSpline) computeMonotoneSlope(idx int) float64 {
	// TODO: this.
	panic("monotone cubic splines not yet implemented.")
}

func (c *CubicSpline) updateFunc(idx int) {
	// TODO: use a closed-form solution to this
	// system to improve performance.
	x0 := c.x[idx]
	x1 := c.x[idx+1]
	system := &linalg.Matrix{
		Rows: 4,
		Cols: 4,
		Data: []float64{
			1, x0, x0 * x0, x0 * x0 * x0,
			1, x1, x1 * x1, x1 * x1 * x1,
			0, 1, 2 * x0, 3 * x0 * x0,
			0, 1, 2 * x1, 3 * x1 * x1,
		},
	}
	solutions := linalg.Vector{c.y[idx], c.y[idx+1], c.slopes[idx], c.slopes[idx+1]}
	lu := ludecomp.Decompose(system)
	solution := lu.Solve(solutions)
	c.funcs[idx] = CubicFunc{solution[0], solution[1], solution[2], solution[3]}
}
