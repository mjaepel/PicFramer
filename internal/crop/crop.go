// Package crop holds the view transform that places a source image under a
// frame (the crop shape's bounding box).
//
// A View is independent of the frame's pixel size: the same View yields the
// same picture for an on-screen preview and for a full-resolution export, as
// long as the frame aspect ratio is the same.
package crop

import (
	"math"

	"github.com/mjaepel/picframer/internal/geom"
)

// Zoom limits relative to the "cover" scale.
const (
	MinZoom = 0.05
	MaxZoom = 64
)

// View positions an image inside a frame.
type View struct {
	// Zoom is relative to the scale at which the unrotated image just
	// covers the frame. Zoom 1 = cover.
	Zoom float64
	// Rotation in degrees, clockwise.
	Rotation float64
	// CX, CY is the point of the source image (normalized 0..1) shown at
	// the frame's center.
	CX, CY float64
}

// Size is a width/height pair in pixels or frame units.
type Size struct{ W, H float64 }

// Default returns a view that covers the frame, centered.
func Default() View { return View{Zoom: 1, CX: 0.5, CY: 0.5} }

func (v View) rad() float64 { return v.Rotation * math.Pi / 180 }

// coverScale is the frame-units-per-source-pixel scale at Zoom 1.
func coverScale(img, frame Size) float64 {
	return math.Max(frame.W/img.W, frame.H/img.H)
}

// Scale returns the effective frame-units-per-source-pixel scale.
func (v View) Scale(img, frame Size) float64 {
	return coverScale(img, frame) * v.Zoom
}

// ImageToFrame maps source pixel coordinates to frame coordinates.
func (v View) ImageToFrame(img, frame Size) geom.Aff {
	s := v.Scale(img, frame)
	return geom.Translate(-v.CX*img.W, -v.CY*img.H).
		Then(geom.Scale(s, s)).
		Then(geom.Rotate(v.rad())).
		Then(geom.Translate(frame.W/2, frame.H/2))
}

// FrameToImage maps frame coordinates to source pixel coordinates.
func (v View) FrameToImage(img, frame Size) geom.Aff {
	return v.ImageToFrame(img, frame).Invert()
}

// Pan moves the image by (dx, dy) frame units.
func (v View) Pan(dx, dy float64, img, frame Size) View {
	s := v.Scale(img, frame)
	d := geom.Pt{X: dx, Y: dy}.Rotate(-v.rad()).Mul(1 / s)
	v.CX -= d.X / img.W
	v.CY -= d.Y / img.H
	return v
}

// ZoomAt multiplies the zoom by factor, keeping the image point under the
// frame point p fixed.
func (v View) ZoomAt(p geom.Pt, factor float64, img, frame Size) View {
	q := v.FrameToImage(img, frame).Apply(p)
	v.Zoom = clampZoom(v.Zoom * factor)
	return v.pinned(q, p, img, frame)
}

// SetZoom changes the zoom around the frame center.
func (v View) SetZoom(z float64) View {
	v.Zoom = clampZoom(z)
	return v
}

// RotateAt rotates the image around the frame point p.
func (v View) RotateAt(p geom.Pt, deg float64, img, frame Size) View {
	q := v.FrameToImage(img, frame).Apply(p)
	v.Rotation = NormDeg(v.Rotation + deg)
	return v.pinned(q, p, img, frame)
}

// pinned adjusts the center so that source point q appears at frame point p.
func (v View) pinned(q, p geom.Pt, img, frame Size) View {
	s := v.Scale(img, frame)
	c := q.Sub(p.Sub(geom.Pt{X: frame.W / 2, Y: frame.H / 2}).Rotate(-v.rad()).Mul(1 / s))
	v.CX, v.CY = c.X/img.W, c.Y/img.H
	return v
}

// Fill returns a centered view with the smallest zoom at which the
// (rotated) image covers the whole frame.
func (v View) Fill(img, frame Size) View {
	sn, cs := math.Sincos(v.rad())
	sn, cs = math.Abs(sn), math.Abs(cs)
	s := math.Max((frame.W*cs+frame.H*sn)/img.W, (frame.W*sn+frame.H*cs)/img.H)
	v.Zoom = clampZoom(s / coverScale(img, frame))
	v.CX, v.CY = 0.5, 0.5
	return v
}

// Fit returns a centered view with the largest zoom at which the whole
// (rotated) image is visible inside the frame.
func (v View) Fit(img, frame Size) View {
	sn, cs := math.Sincos(v.rad())
	sn, cs = math.Abs(sn), math.Abs(cs)
	s := math.Min(frame.W/(img.W*cs+img.H*sn), frame.H/(img.W*sn+img.H*cs))
	v.Zoom = clampZoom(s / coverScale(img, frame))
	v.CX, v.CY = 0.5, 0.5
	return v
}

// ClampCover limits zoom and center so that the image always covers the
// whole frame. For rotations other than quarter turns the check uses the
// rotated frame's bounding box, which is slightly conservative.
func (v View) ClampCover(img, frame Size) View {
	fill := v.Fill(img, frame)
	if v.Zoom < fill.Zoom {
		v.Zoom = fill.Zoom
	}
	s := v.Scale(img, frame)
	sn, cs := math.Sincos(v.rad())
	sn, cs = math.Abs(sn), math.Abs(cs)
	hx := (frame.W*cs + frame.H*sn) / (2 * s) / img.W
	hy := (frame.W*sn + frame.H*cs) / (2 * s) / img.H
	v.CX = clampCenter(v.CX, hx)
	v.CY = clampCenter(v.CY, hy)
	return v
}

func clampCenter(c, half float64) float64 {
	if half >= 0.5 {
		return 0.5
	}
	return math.Min(math.Max(c, half), 1-half)
}

// NativeSize returns the frame size (with the given aspect) at which one
// output pixel corresponds to one source pixel.
func (v View) NativeSize(img Size, aspect float64) Size {
	// coverScale(img, frame) = W * max(1/img.W, 1/(aspect*img.H)).
	k := math.Max(1/img.W, 1/(aspect*img.H))
	w := 1 / (v.Zoom * k)
	return Size{w, w / aspect}
}

func clampZoom(z float64) float64 {
	if math.IsNaN(z) || z <= 0 {
		return 1
	}
	return math.Min(math.Max(z, MinZoom), MaxZoom)
}

// NormDeg maps an angle to (-180, 180].
func NormDeg(d float64) float64 {
	d = math.Mod(d, 360)
	if d > 180 {
		d -= 360
	} else if d <= -180 {
		d += 360
	}
	return d
}
