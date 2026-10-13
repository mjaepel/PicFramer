// Package geom provides the small set of 2D primitives shared by the
// preview (Gio) and the export renderer, so both draw exactly the same
// geometry.
package geom

import "math"

// Pt is a point in a continuous, y-down coordinate system.
type Pt struct{ X, Y float64 }

func (p Pt) Add(q Pt) Pt      { return Pt{p.X + q.X, p.Y + q.Y} }
func (p Pt) Sub(q Pt) Pt      { return Pt{p.X - q.X, p.Y - q.Y} }
func (p Pt) Mul(s float64) Pt { return Pt{p.X * s, p.Y * s} }
func (p Pt) Len() float64     { return math.Hypot(p.X, p.Y) }
func (p Pt) Rotate(rad float64) Pt { // clockwise on screen (y axis points down)
	s, c := math.Sincos(rad)
	return Pt{p.X*c - p.Y*s, p.X*s + p.Y*c}
}

// Rect is an axis-aligned rectangle given by origin and size.
type Rect struct{ X, Y, W, H float64 }

func (r Rect) Min() Pt              { return Pt{r.X, r.Y} }
func (r Rect) Max() Pt              { return Pt{r.X + r.W, r.Y + r.H} }
func (r Rect) Center() Pt           { return Pt{r.X + r.W/2, r.Y + r.H/2} }
func (r Rect) Contains(p Pt) bool   { return p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H }
func (r Rect) Inset(d float64) Rect { return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }
func (r Rect) Translate(p Pt) Rect  { return Rect{r.X + p.X, r.Y + p.Y, r.W, r.H} }
func (r Rect) Scale(s float64) Rect { return Rect{r.X * s, r.Y * s, r.W * s, r.H * s} }
func (r Rect) Empty() bool          { return r.W <= 0 || r.H <= 0 }
func (r Rect) Area() float64        { return math.Max(r.W, 0) * math.Max(r.H, 0) }
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := math.Max(r.X, o.X), math.Max(r.Y, o.Y)
	x1, y1 := math.Min(r.X+r.W, o.X+o.W), math.Min(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// FitRect returns the largest rectangle with the given aspect ratio
// (width/height) that fits into r, centered.
func FitRect(r Rect, aspect float64) Rect {
	w, h := r.W, r.W/aspect
	if h > r.H {
		h = r.H
		w = h * aspect
	}
	return Rect{r.X + (r.W-w)/2, r.Y + (r.H-h)/2, w, h}
}

// Aff is a 2D affine transform mapping (x, y) to
// (A*x + B*y + C, D*x + E*y + F). The layout matches f64.Aff3 from
// golang.org/x/image/math/f64.
type Aff [6]float64

// Identity is the identity transform.
var Identity = Aff{1, 0, 0, 0, 1, 0}

func Translate(dx, dy float64) Aff { return Aff{1, 0, dx, 0, 1, dy} }
func Scale(sx, sy float64) Aff     { return Aff{sx, 0, 0, 0, sy, 0} }

// Rotate returns a rotation by rad radians, clockwise on screen.
func Rotate(rad float64) Aff {
	s, c := math.Sincos(rad)
	return Aff{c, -s, 0, s, c, 0}
}

// Then returns the transform that applies a first and b second.
func (a Aff) Then(b Aff) Aff {
	return Aff{
		b[0]*a[0] + b[1]*a[3], b[0]*a[1] + b[1]*a[4], b[0]*a[2] + b[1]*a[5] + b[2],
		b[3]*a[0] + b[4]*a[3], b[3]*a[1] + b[4]*a[4], b[3]*a[2] + b[4]*a[5] + b[5],
	}
}

// Apply transforms p.
func (a Aff) Apply(p Pt) Pt {
	return Pt{a[0]*p.X + a[1]*p.Y + a[2], a[3]*p.X + a[4]*p.Y + a[5]}
}

// Invert returns the inverse transform. A singular transform yields the identity.
func (a Aff) Invert() Aff {
	det := a[0]*a[4] - a[1]*a[3]
	if det == 0 {
		return Identity
	}
	inv := 1 / det
	return Aff{
		a[4] * inv, -a[1] * inv, (a[1]*a[5] - a[4]*a[2]) * inv,
		-a[3] * inv, a[0] * inv, (a[3]*a[2] - a[0]*a[5]) * inv,
	}
}

// SegKind identifies a path segment type.
type SegKind uint8

const (
	MoveTo SegKind = iota
	LineTo
	QuadTo
	CubeTo
	Close
)

// Seg is one path segment. Only the first N points are used, where N is
// 1 for MoveTo/LineTo, 2 for QuadTo, 3 for CubeTo and 0 for Close.
type Seg struct {
	Kind SegKind
	P    [3]Pt
}

// Path is a sequence of segments describing one or more closed outlines.
type Path []Seg

func (p *Path) MoveTo(a Pt)         { *p = append(*p, Seg{Kind: MoveTo, P: [3]Pt{a}}) }
func (p *Path) LineTo(a Pt)         { *p = append(*p, Seg{Kind: LineTo, P: [3]Pt{a}}) }
func (p *Path) QuadTo(c, a Pt)      { *p = append(*p, Seg{Kind: QuadTo, P: [3]Pt{c, a}}) }
func (p *Path) CubeTo(c1, c2, a Pt) { *p = append(*p, Seg{Kind: CubeTo, P: [3]Pt{c1, c2, a}}) }
func (p *Path) Close()              { *p = append(*p, Seg{Kind: Close}) }

// NumPts returns how many points a segment of kind k uses.
func (k SegKind) NumPts() int {
	switch k {
	case MoveTo, LineTo:
		return 1
	case QuadTo:
		return 2
	case CubeTo:
		return 3
	}
	return 0
}

// Transform returns a copy of p with every point mapped through a.
func (p Path) Transform(a Aff) Path {
	out := make(Path, len(p))
	for i, s := range p {
		out[i].Kind = s.Kind
		for j := 0; j < s.Kind.NumPts(); j++ {
			out[i].P[j] = a.Apply(s.P[j])
		}
	}
	return out
}

// Bounds returns the bounding box of all points, including control points.
func (p Path) Bounds() Rect {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, s := range p {
		for j := 0; j < s.Kind.NumPts(); j++ {
			q := s.P[j]
			minX, minY = math.Min(minX, q.X), math.Min(minY, q.Y)
			maxX, maxY = math.Max(maxX, q.X), math.Max(maxY, q.Y)
		}
	}
	if minX > maxX {
		return Rect{}
	}
	return Rect{minX, minY, maxX - minX, maxY - minY}
}
