package model

// Point is a coordinate pair in PDF user space, where y increases upwards.
// This is the opposite of most screen coordinate systems, and is a common
// source of vertically mirrored output.
type Point struct{ X, Y float64 }

// Rect is an axis-aligned rectangle with Min at the lower-left corner.
//
// The format does not guarantee that ordering: /MediaBox and similar entries
// may be written with their corners in any order. This library rejects an
// inverted rectangle rather than silently normalising it, so that a Rect
// always satisfies Min.X <= Max.X and Min.Y <= Max.Y.
type Rect struct{ Min, Max Point }

// Matrix is a PDF transformation matrix [a b c d e f], which maps (x,y) to
// (a*x + c*y + e, b*x + d*y + f).
//
// The six values are the first two columns of a 3x3 affine matrix whose third
// column is fixed at [0 0 1], so PDF omits it. Coordinates are in user space;
// page rotation and /UserUnit are kept separately and are not folded in here.
type Matrix [6]float64

// IdentityMatrix returns a transformation that leaves coordinates unchanged.
func IdentityMatrix() Matrix { return Matrix{1, 0, 0, 1, 0, 0} }

// Transform maps p through m. It does not apply page rotation or /UserUnit.
func (m Matrix) Transform(p Point) Point {
	return Point{X: m[0]*p.X + m[2]*p.Y + m[4], Y: m[1]*p.X + m[3]*p.Y + m[5]}
}

// Mul composes two transformations so that
// m.Mul(n).Transform(p) equals m.Transform(n.Transform(p)).
//
// Note the order: n is applied first, then m. An interpreter handling the cm
// operator therefore writes ctm = ctm.Mul(operand), because cm maps coordinates
// through its operand and only then through the matrix already in effect.
func (m Matrix) Mul(n Matrix) Matrix {
	return Matrix{
		m[0]*n[0] + m[2]*n[1],
		m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3],
		m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4],
		m[1]*n[4] + m[3]*n[5] + m[5],
	}
}
