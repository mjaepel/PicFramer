package crop

import (
	"math"
	"testing"

	"github.com/mjaepel/picframer/internal/geom"
)

func pt(x, y float64) geom.Pt { return geom.Pt{X: x, Y: y} }

func near(a, b geom.Pt) bool { return math.Abs(a.X-b.X) < 1e-6 && math.Abs(a.Y-b.Y) < 1e-6 }

var (
	img   = Size{W: 4000, H: 3000}
	frame = Size{W: 500, H: 400}
)

func TestDefaultCoversFrame(t *testing.T) {
	v := Default()
	a := v.ImageToFrame(img, frame)
	// The image center maps to the frame center.
	if p := a.Apply(geom.Pt{X: 2000, Y: 1500}); !near(p, geom.Pt{X: 250, Y: 200}) {
		t.Errorf("center maps to %v", p)
	}
	// 4000x3000 into 500x400: cover scale = max(0.125, 0.1333) = 0.1333.
	if s := v.Scale(img, frame); math.Abs(s-400.0/3000) > 1e-12 {
		t.Errorf("scale %v", s)
	}
}

func TestInverse(t *testing.T) {
	v := View{Zoom: 1.7, Rotation: 33, CX: 0.3, CY: 0.6}
	a, b := v.ImageToFrame(img, frame), v.FrameToImage(img, frame)
	for _, p := range []geom.Pt{pt(0, 0), pt(123, 456), pt(4000, 3000)} {
		if q := b.Apply(a.Apply(p)); !near(p, q) {
			t.Errorf("round trip %v -> %v", p, q)
		}
	}
}

func TestPanMovesImage(t *testing.T) {
	v := View{Zoom: 2, Rotation: 30, CX: 0.5, CY: 0.5}
	q := geom.Pt{X: 1000, Y: 700}
	before := v.ImageToFrame(img, frame).Apply(q)
	v = v.Pan(15, -8, img, frame)
	after := v.ImageToFrame(img, frame).Apply(q)
	if !near(after.Sub(before), geom.Pt{X: 15, Y: -8}) {
		t.Errorf("pan moved point by %v", after.Sub(before))
	}
}

func TestZoomAtKeepsPoint(t *testing.T) {
	v := View{Zoom: 1, Rotation: -20, CX: 0.4, CY: 0.5}
	p := geom.Pt{X: 100, Y: 300}
	q := v.FrameToImage(img, frame).Apply(p)
	v = v.ZoomAt(p, 2.5, img, frame)
	if got := v.ImageToFrame(img, frame).Apply(q); !near(got, p) {
		t.Errorf("zoom moved anchor to %v", got)
	}
	if v.Zoom != 2.5 {
		t.Errorf("zoom %v", v.Zoom)
	}
}

func TestRotateAtKeepsPoint(t *testing.T) {
	v := Default()
	p := geom.Pt{X: 50, Y: 60}
	q := v.FrameToImage(img, frame).Apply(p)
	v = v.RotateAt(p, 45, img, frame)
	if got := v.ImageToFrame(img, frame).Apply(q); !near(got, p) {
		t.Errorf("rotate moved anchor to %v", got)
	}
}

// corners of the frame must lie inside the image after Fill.
func TestFillCovers(t *testing.T) {
	for _, deg := range []float64{0, 10, 45, 90, -135} {
		v := View{Rotation: deg}.Fill(img, frame)
		m := v.FrameToImage(img, frame)
		for _, c := range []geom.Pt{pt(0, 0), pt(frame.W, 0), pt(0, frame.H), pt(frame.W, frame.H)} {
			q := m.Apply(c)
			if q.X < -1e-6 || q.Y < -1e-6 || q.X > img.W+1e-6 || q.Y > img.H+1e-6 {
				t.Errorf("rot %v: frame corner %v outside image at %v", deg, c, q)
			}
		}
	}
}

// image corners must lie inside the frame after Fit.
func TestFitContains(t *testing.T) {
	for _, deg := range []float64{0, 10, 45, 90, -135} {
		v := View{Rotation: deg}.Fit(img, frame)
		m := v.ImageToFrame(img, frame)
		for _, c := range []geom.Pt{pt(0, 0), pt(img.W, 0), pt(0, img.H), pt(img.W, img.H)} {
			q := m.Apply(c)
			if q.X < -1e-6 || q.Y < -1e-6 || q.X > frame.W+1e-6 || q.Y > frame.H+1e-6 {
				t.Errorf("rot %v: image corner %v outside frame at %v", deg, c, q)
			}
		}
	}
}

func TestClampCover(t *testing.T) {
	for _, v := range []View{
		{Zoom: 0.3, CX: 0.5, CY: 0.5},
		{Zoom: 2, CX: -1, CY: 3},
		{Zoom: 1.2, Rotation: 90, CX: 0.95, CY: 0.05},
		{Zoom: 3, Rotation: 20, CX: 0, CY: 1},
	} {
		c := v.ClampCover(img, frame)
		m := c.FrameToImage(img, frame)
		for _, p := range []geom.Pt{pt(0, 0), pt(frame.W, 0), pt(0, frame.H), pt(frame.W, frame.H)} {
			q := m.Apply(p)
			if q.X < -1e-6 || q.Y < -1e-6 || q.X > img.W+1e-6 || q.Y > img.H+1e-6 {
				t.Errorf("%+v clamped to %+v: frame corner %v outside image at %v", v, c, p, q)
			}
		}
	}
	// A view that already covers stays unchanged.
	v := View{Zoom: 2, CX: 0.4, CY: 0.6}
	if c := v.ClampCover(img, frame); c != v {
		t.Errorf("covering view changed: %+v -> %+v", v, c)
	}
}

func TestNativeSize(t *testing.T) {
	v := View{Zoom: 2, CX: 0.5, CY: 0.5}
	n := v.NativeSize(img, frame.W/frame.H)
	if s := v.Scale(img, Size{n.W, n.H}); math.Abs(s-1) > 1e-9 {
		t.Errorf("native scale %v (size %v)", s, n)
	}
}

func TestNormDeg(t *testing.T) {
	for in, want := range map[float64]float64{0: 0, 180: 180, 181: -179, -180: 180, 540: 180, -450: -90} {
		if got := NormDeg(in); got != want {
			t.Errorf("NormDeg(%v) = %v, want %v", in, got, want)
		}
	}
}
