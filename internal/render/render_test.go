package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/mjaepel/picframer/internal/crop"
	"github.com/mjaepel/picframer/internal/geom"
	"github.com/mjaepel/picframer/internal/shape"
)

var (
	red    = color.NRGBA{255, 0, 0, 255}
	green  = color.NRGBA{0, 255, 0, 255}
	blue   = color.NRGBA{0, 0, 255, 255}
	yellow = color.NRGBA{255, 255, 0, 255}
	white  = color.NRGBA{255, 255, 255, 255}
)

// quadrants returns a w×h image with red (top left), green (top right),
// blue (bottom left) and yellow (bottom right) quadrants.
func quadrants(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := red
			switch {
			case x >= w/2 && y < h/2:
				c = green
			case x < w/2 && y >= h/2:
				c = blue
			case x >= w/2 && y >= h/2:
				c = yellow
			}
			m.SetNRGBA(x, y, c)
		}
	}
	return m
}

func at(m image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(m.At(x, y)).(color.NRGBA)
}

func same(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return x-y < 8 || y-x < 8 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

func TestCropRectDefault(t *testing.T) {
	src := quadrants(400, 200)
	sp := shape.Spec{Kind: shape.Rect, Ratio: shape.Ratio{W: 2, H: 1}}
	out := Crop(src, crop.Default(), sp, 200, 100, color.NRGBA{})
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{{20, 20, red}, {180, 20, green}, {20, 80, blue}, {180, 80, yellow}} {
		if got := at(out, c.x, c.y); !same(got, c.want) {
			t.Errorf("(%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

// A 180° rotation swaps diagonally opposite quadrants.
func TestCropRotated(t *testing.T) {
	src := quadrants(400, 200)
	sp := shape.Spec{Kind: shape.Rect, Ratio: shape.Ratio{W: 2, H: 1}}
	v := crop.Default()
	v.Rotation = 180
	out := Crop(src, v, sp, 200, 100, color.NRGBA{})
	if got := at(out, 20, 20); !same(got, yellow) {
		t.Errorf("top left = %v, want yellow", got)
	}
	if got := at(out, 180, 80); !same(got, red) {
		t.Errorf("bottom right = %v, want red", got)
	}
}

// Zooming in 2x around the top-left quadrant's center shows only red.
func TestCropZoomPan(t *testing.T) {
	src := quadrants(400, 400)
	sp := shape.Spec{Kind: shape.Rect, Ratio: shape.Ratio{W: 1, H: 1}}
	v := crop.View{Zoom: 2, CX: 0.25, CY: 0.25}
	out := Crop(src, v, sp, 100, 100, color.NRGBA{})
	for _, p := range [][2]int{{2, 2}, {97, 2}, {2, 97}, {97, 97}} {
		if got := at(out, p[0], p[1]); !same(got, red) {
			t.Errorf("%v = %v, want red", p, got)
		}
	}
}

func TestCircleMask(t *testing.T) {
	src := quadrants(200, 200)
	sp := shape.Spec{Kind: shape.Circle}
	out := Crop(src, crop.Default(), sp, 100, 100, color.NRGBA{})
	if got := at(out, 1, 1); got.A != 0 {
		t.Errorf("corner should be transparent, got %v", got)
	}
	if got := at(out, 30, 30); !same(got, red) {
		t.Errorf("inside = %v, want red", got)
	}
	out = Crop(src, crop.Default(), sp, 100, 100, white)
	if got := at(out, 1, 1); !same(got, white) {
		t.Errorf("corner with background = %v, want white", got)
	}
}

// Zoomed out, the frame is not covered; the gap gets the background.
func TestCropUncoveredGetsBackground(t *testing.T) {
	src := quadrants(200, 200)
	sp := shape.Spec{Kind: shape.Rect, Ratio: shape.Ratio{W: 1, H: 1}}
	v := crop.View{Zoom: 0.5, CX: 0.5, CY: 0.5}
	out := Crop(src, v, sp, 100, 100, white)
	if got := at(out, 5, 50); !same(got, white) {
		t.Errorf("uncovered = %v, want white", got)
	}
	if got := at(out, 30, 30); !same(got, red) {
		t.Errorf("covered = %v, want red", got)
	}
}

func TestCollage(t *testing.T) {
	left := quadrants(100, 100)
	cells := []Cell{
		{Src: left, View: crop.Default(), Rect: geom.Rect{X: 0, Y: 0, W: 50, H: 50}},
		{Src: left, View: crop.Default(), Rect: geom.Rect{X: 60, Y: 0, W: 40, H: 50}, Contain: true},
	}
	out := Collage(cells, 100, 50, white)
	if got := at(out, 5, 5); !same(got, red) {
		t.Errorf("cell 1 = %v", got)
	}
	if got := at(out, 55, 25); !same(got, white) {
		t.Errorf("gap = %v, want white", got)
	}
	// Contain: 100x100 into 40x50 → 40x40 centered vertically, letterbox above.
	if got := at(out, 80, 2); !same(got, white) {
		t.Errorf("letterbox = %v, want white", got)
	}
	if got := at(out, 62, 10); !same(got, red) {
		t.Errorf("contained top-left = %v, want red", got)
	}
}
