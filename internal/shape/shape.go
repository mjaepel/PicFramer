// Package shape describes the crop shapes and builds their outlines.
package shape

import (
	"math"

	"github.com/mjaepel/picframer/internal/geom"
)

// Kind identifies a shape.
type Kind string

const (
	Rect        Kind = "rect"
	RoundedRect Kind = "rounded"
	Ellipse     Kind = "ellipse"
	Circle      Kind = "circle"
	Triangle    Kind = "triangle"
	Polygon     Kind = "polygon"
	Star        Kind = "star"
)

// Kinds lists all shapes in UI order.
var Kinds = []Kind{Rect, RoundedRect, Ellipse, Circle, Triangle, Polygon, Star}

// Label returns a human readable name.
func (k Kind) Label() string {
	switch k {
	case Rect:
		return "Rectangle"
	case RoundedRect:
		return "Rounded"
	case Ellipse:
		return "Ellipse"
	case Circle:
		return "Circle"
	case Triangle:
		return "Triangle"
	case Polygon:
		return "Polygon"
	case Star:
		return "Star"
	}
	return string(k)
}

// Direction is where a triangle's apex points.
type Direction string

const (
	Up    Direction = "up"
	Down  Direction = "down"
	Left  Direction = "left"
	Right Direction = "right"
)

// Spec fully describes a shape. Fields that do not apply to Kind are ignored.
type Spec struct {
	Kind Kind
	// Ratio of the shape's bounding box. A zero ratio means the shape's
	// natural ratio (see NaturalAspect). Circle always uses 1:1.
	Ratio Ratio
	// Corner is the corner radius of RoundedRect as a fraction (0..0.5)
	// of the shorter side.
	Corner float64
	// Apex is the direction of a Triangle's tip.
	Apex Direction
	// Sides is the number of corners of a Polygon or points of a Star (3..24).
	Sides int
	// Inner is a Star's inner radius relative to its outer radius (0.1..0.95).
	Inner float64
}

// Default returns a sensible spec for kind k.
func Default(k Kind) Spec {
	s := Spec{Kind: k, Corner: 0.15, Apex: Up, Sides: 6, Inner: 0.5}
	switch k {
	case Rect, RoundedRect, Ellipse:
		s.Ratio = Ratio{4, 3}
	case Star:
		s.Sides = 5
	}
	return s
}

func (s Spec) sides() int {
	n := s.Sides
	if n < 3 {
		n = 3
	}
	if n > 24 {
		n = 24
	}
	return n
}

func (s Spec) inner() float64 {
	return math.Min(math.Max(s.Inner, 0.1), 0.95)
}

// Aspect returns the width/height ratio of the shape's bounding box.
func (s Spec) Aspect() float64 {
	if s.Kind == Circle {
		return 1
	}
	if s.Ratio.Valid() {
		return s.Ratio.Aspect()
	}
	return s.NaturalAspect()
}

// NaturalAspect is the undistorted aspect ratio of the shape: equilateral
// triangle, regular polygon, regular star; 1 for all others.
func (s Spec) NaturalAspect() float64 {
	switch s.Kind {
	case Triangle, Polygon, Star:
		b := pointsBounds(s.unitPoints())
		return b.W / b.H
	}
	return 1
}

// HasNatural reports whether the shape has a meaningful natural ratio that
// differs from "any rectangle".
func (s Spec) HasNatural() bool {
	switch s.Kind {
	case Triangle, Polygon, Star:
		return true
	}
	return false
}

// RatioFixed reports whether the ratio cannot be changed (circle).
func (s Spec) RatioFixed() bool { return s.Kind == Circle }

// kappa is the cubic Bézier control distance for a quarter circle.
const kappa = 0.5522847498307936

// Path returns the shape's outline filling the box [0,w]×[0,h].
func (s Spec) Path(w, h float64) geom.Path {
	var p geom.Path
	switch s.Kind {
	case Ellipse, Circle:
		ellipse(&p, geom.Rect{W: w, H: h})
	case RoundedRect:
		r := math.Min(math.Max(s.Corner, 0), 0.5) * math.Min(w, h)
		roundedRect(&p, w, h, r)
	case Triangle, Polygon, Star:
		pts := s.unitPoints()
		b := pointsBounds(pts)
		for i, q := range pts {
			q = geom.Pt{X: (q.X - b.X) / b.W * w, Y: (q.Y - b.Y) / b.H * h}
			if i == 0 {
				p.MoveTo(q)
			} else {
				p.LineTo(q)
			}
		}
		p.Close()
	default:
		p.MoveTo(geom.Pt{})
		p.LineTo(geom.Pt{X: w})
		p.LineTo(geom.Pt{X: w, Y: h})
		p.LineTo(geom.Pt{Y: h})
		p.Close()
	}
	return p
}

// unitPoints returns polygon vertices on the unit circle (before fitting
// to the bounding box).
func (s Spec) unitPoints() []geom.Pt {
	switch s.Kind {
	case Triangle:
		pts := regular(3, -math.Pi/2)
		var rot float64
		switch s.Apex {
		case Down:
			rot = math.Pi
		case Left:
			rot = -math.Pi / 2
		case Right:
			rot = math.Pi / 2
		}
		for i := range pts {
			pts[i] = pts[i].Rotate(rot)
		}
		return pts
	case Polygon:
		n := s.sides()
		// Even polygons get a flat top, odd ones a tip at the top.
		start := -math.Pi / 2
		if n%2 == 0 {
			start += math.Pi / float64(n)
		}
		return regular(n, start)
	case Star:
		n := s.sides()
		in := s.inner()
		pts := make([]geom.Pt, 0, 2*n)
		for i := 0; i < 2*n; i++ {
			r := 1.0
			if i%2 == 1 {
				r = in
			}
			a := -math.Pi/2 + float64(i)*math.Pi/float64(n)
			pts = append(pts, geom.Pt{X: r * math.Cos(a), Y: r * math.Sin(a)})
		}
		return pts
	}
	return nil
}

func regular(n int, start float64) []geom.Pt {
	pts := make([]geom.Pt, n)
	for i := range pts {
		a := start + 2*math.Pi*float64(i)/float64(n)
		pts[i] = geom.Pt{X: math.Cos(a), Y: math.Sin(a)}
	}
	return pts
}

func pointsBounds(pts []geom.Pt) geom.Rect {
	var p geom.Path
	for _, q := range pts {
		p.LineTo(q)
	}
	return p.Bounds()
}

func ellipse(p *geom.Path, r geom.Rect) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rx, ry := r.W/2, r.H/2
	kx, ky := rx*kappa, ry*kappa
	pt := func(x, y float64) geom.Pt { return geom.Pt{X: x, Y: y} }
	p.MoveTo(pt(cx+rx, cy))
	p.CubeTo(pt(cx+rx, cy+ky), pt(cx+kx, cy+ry), pt(cx, cy+ry))
	p.CubeTo(pt(cx-kx, cy+ry), pt(cx-rx, cy+ky), pt(cx-rx, cy))
	p.CubeTo(pt(cx-rx, cy-ky), pt(cx-kx, cy-ry), pt(cx, cy-ry))
	p.CubeTo(pt(cx+kx, cy-ry), pt(cx+rx, cy-ky), pt(cx+rx, cy))
	p.Close()
}

func roundedRect(p *geom.Path, w, h, r float64) {
	pt := func(x, y float64) geom.Pt { return geom.Pt{X: x, Y: y} }
	k := r * kappa
	p.MoveTo(pt(r, 0))
	p.LineTo(pt(w-r, 0))
	p.CubeTo(pt(w-r+k, 0), pt(w, r-k), pt(w, r))
	p.LineTo(pt(w, h-r))
	p.CubeTo(pt(w, h-r+k), pt(w-r+k, h), pt(w-r, h))
	p.LineTo(pt(r, h))
	p.CubeTo(pt(r-k, h), pt(0, h-r+k), pt(0, h-r))
	p.LineTo(pt(0, r))
	p.CubeTo(pt(0, r-k), pt(r-k, 0), pt(r, 0))
	p.Close()
}
