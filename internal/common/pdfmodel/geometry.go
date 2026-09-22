package pdfmodel

type Point struct{ X, Y float64 }

type Rect struct{ Min, Max Point }

// Matrix is [a b c d e f], mapping (x,y) to (a*x+c*y+e,b*x+d*y+f).
// Coordinates use PDF user space; page rotation is retained separately.
type Matrix [6]float64

// IdentityMatrix returns a transformation that leaves coordinates unchanged.
func IdentityMatrix() Matrix { return Matrix{1, 0, 0, 1, 0, 0} }

// Transform maps p through m without applying page rotation or UserUnit.
func (m Matrix) Transform(p Point) Point {
	return Point{m[0]*p.X + m[2]*p.Y + m[4], m[1]*p.X + m[3]*p.Y + m[5]}
}

// Mul composes transformations: m.Mul(n).Transform(p) = m.Transform(n.Transform(p)).
func (m Matrix) Mul(n Matrix) Matrix {
	return Matrix{m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1], m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3], m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5]}
}
