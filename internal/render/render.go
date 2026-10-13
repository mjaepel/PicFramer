// Package render produces the full-resolution export images. It uses the
// same geometry (shape paths and crop views) as the on-screen preview.
package render

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/vector"

	"github.com/mjaepel/picframer/internal/crop"
	"github.com/mjaepel/picframer/internal/geom"
	"github.com/mjaepel/picframer/internal/shape"
)

// Crop renders src through view v into a w×h image clipped to shape sp.
// Pixels outside the shape (and inside the shape but not covered by the
// image) get bg; use a transparent bg for a transparent result.
func Crop(src image.Image, v crop.View, sp shape.Spec, w, h int, bg color.NRGBA) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(out, out.Bounds(), bg)
	layer := transformed(src, v, w, h)
	mask := Mask(sp.Path(float64(w), float64(h)), w, h)
	draw.DrawMask(out, out.Bounds(), layer, image.Point{}, mask, image.Point{}, draw.Over)
	return out
}

// transformed draws src through v into a new transparent w×h image.
func transformed(src image.Image, v crop.View, w, h int) *image.RGBA {
	layer := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	img := crop.Size{W: float64(b.Dx()), H: float64(b.Dy())}
	frame := crop.Size{W: float64(w), H: float64(h)}
	// The view works in source coordinates relative to the image origin.
	a := geom.Translate(-float64(b.Min.X), -float64(b.Min.Y)).Then(v.ImageToFrame(img, frame))
	draw.CatmullRom.Transform(layer, f64.Aff3(a), src, b, draw.Src, nil)
	return layer
}

// Mask rasterizes path p into an anti-aliased w×h alpha mask.
func Mask(p geom.Path, w, h int) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	for _, s := range p {
		f := func(i int) (float32, float32) { return float32(s.P[i].X), float32(s.P[i].Y) }
		switch s.Kind {
		case geom.MoveTo:
			z.MoveTo(f(0))
		case geom.LineTo:
			z.LineTo(f(0))
		case geom.QuadTo:
			x1, y1 := f(0)
			x2, y2 := f(1)
			z.QuadTo(x1, y1, x2, y2)
		case geom.CubeTo:
			x1, y1 := f(0)
			x2, y2 := f(1)
			x3, y3 := f(2)
			z.CubeTo(x1, y1, x2, y2, x3, y3)
		case geom.Close:
			z.ClosePath()
		}
	}
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

func fill(dst *image.RGBA, r image.Rectangle, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	draw.Draw(dst, r, image.NewUniform(c), image.Point{}, draw.Src)
}

// Cell is one picture of a collage.
type Cell struct {
	Src  image.Image
	View crop.View
	// Rect is the cell's area in output pixels (fractional values are
	// rounded consistently, so adjacent cells never overlap).
	Rect geom.Rect
	// Contain shows the whole image (letterboxed) instead of covering the cell.
	Contain bool
}

// Collage renders cells onto a w×h canvas filled with bg.
func Collage(cells []Cell, w, h int, bg color.NRGBA) *image.RGBA {
	out := Canvas(w, h, bg)
	for _, c := range cells {
		DrawCell(out, c)
	}
	return out
}

// Canvas returns a w×h image filled with bg.
func Canvas(w, h int, bg color.NRGBA) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(out, out.Bounds(), bg)
	return out
}

// DrawCell draws one collage picture onto dst.
func DrawCell(dst *image.RGBA, c Cell) {
	r := PixelRect(c.Rect)
	if r.Empty() {
		return
	}
	b := c.Src.Bounds()
	v := CellView(c.View, c.Contain, crop.Size{W: float64(b.Dx()), H: float64(b.Dy())}, crop.Size{W: float64(r.Dx()), H: float64(r.Dy())})
	layer := transformed(c.Src, v, r.Dx(), r.Dy())
	draw.Draw(dst, r, layer, image.Point{}, draw.Over)
}

// CellView returns the view actually used for a collage cell: the whole
// image for contain mode, otherwise v limited so that the image covers the
// cell.
func CellView(v crop.View, contain bool, img, cell crop.Size) crop.View {
	if contain {
		return v.Fit(img, cell)
	}
	return v.ClampCover(img, cell)
}

// PixelRect rounds each edge of r to the nearest pixel.
func PixelRect(r geom.Rect) image.Rectangle {
	return image.Rect(
		int(math.Round(r.X)), int(math.Round(r.Y)),
		int(math.Round(r.X+r.W)), int(math.Round(r.Y+r.H)),
	)
}
