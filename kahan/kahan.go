package kahan

// A Summer64 computes a rolling sum of float64s.
type Summer64 struct {
	sum          float64
	compensation float64
}

// NewSummer64 creates a Summer64 with a starting sum of 0.
func NewSummer64() *Summer64 {
	return &Summer64{}
}

// Add adds a number to the current sum, returning the new sum.
func (s *Summer64) Add(n float64) float64 {
	n -= s.compensation
	sum := s.sum + n
	s.compensation = (sum - s.sum) - n
	s.sum = sum
	return s.sum
}

// Sum returns the current sum.
func (s *Summer64) Sum() float64 {
	return s.sum
}

// Sum64 adds all the floats in a slice and returns the sum
func Sum64(nums []float64) float64 {
	var summer Summer64
	for _, n := range nums {
		summer.Add(n)
	}
	return summer.Sum()
}

// A ComplexSummer128 computes a rolling sum of complex128s.
type ComplexSummer128 struct {
	realSum *Summer64
	imagSum *Summer64
}

// NewComplexSummer128 creates a new ComplexSummer128
// with a starting value of 0.
func NewComplexSummer128() *ComplexSummer128 {
	return &ComplexSummer128{
		realSum: NewSummer64(),
		imagSum: NewSummer64(),
	}
}

// Add adds a number to the current sum, returning
// the new sum.
func (c *ComplexSummer128) Add(n complex128) complex128 {
	c.realSum.Add(real(n))
	c.imagSum.Add(imag(n))
	return c.Sum()
}

// Sum returns the current sum.
func (c *ComplexSummer128) Sum() complex128 {
	return complex(c.realSum.Sum(), c.imagSum.Sum())
}
