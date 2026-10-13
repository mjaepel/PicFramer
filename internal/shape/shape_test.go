package shape

import (
	"math"
	"testing"
)

func TestParseRatio(t *testing.T) {
	good := map[string]float64{
		"5:4":     1.25,
		" 16 : 9": 16.0 / 9,
		"4/3":     4.0 / 3,
		"3x2":     1.5,
		"1.5":     1.5,
		"1,5":     1.5,
		"2,35:1":  2.35,
		"1:1":     1,
	}
	for in, want := range good {
		r, err := ParseRatio(in)
		if err != nil {
			t.Errorf("ParseRatio(%q): %v", in, err)
			continue
		}
		if math.Abs(r.Aspect()-want) > 1e-9 {
			t.Errorf("ParseRatio(%q) aspect = %v, want %v", in, r.Aspect(), want)
		}
	}
	for _, in := range []string{"", "abc", "5:", ":4", "0:4", "-1:2", "5:0", "1000:1", "NaN", "inf:1"} {
		if _, err := ParseRatio(in); err == nil {
			t.Errorf("ParseRatio(%q) succeeded, want error", in)
		}
	}
}

func TestRatioString(t *testing.T) {
	if s := (Ratio{5, 4}).String(); s != "5:4" {
		t.Errorf("got %q", s)
	}
	if s := (Ratio{2.35, 1}).String(); s != "2.35:1" {
		t.Errorf("got %q", s)
	}
}

func TestPathsFillBox(t *testing.T) {
	const w, h = 300, 200
	for _, k := range Kinds {
		sp := Default(k)
		p := sp.Path(w, h)
		if len(p) < 3 {
			t.Fatalf("%s: path too short", k)
		}
		b := p.Bounds()
		if math.Abs(b.X) > 1e-9 || math.Abs(b.Y) > 1e-9 || math.Abs(b.W-w) > 1e-9 || math.Abs(b.H-h) > 1e-9 {
			t.Errorf("%s: bounds %+v, want 0,0,%d,%d", k, b, w, h)
		}
	}
}

func TestAspect(t *testing.T) {
	if a := (Spec{Kind: Circle, Ratio: Ratio{16, 9}}).Aspect(); a != 1 {
		t.Errorf("circle aspect %v", a)
	}
	if a := (Spec{Kind: Rect, Ratio: Ratio{5, 4}}).Aspect(); a != 1.25 {
		t.Errorf("rect aspect %v", a)
	}
	// Equilateral triangle: width / height = 2 / sqrt(3).
	if a := Default(Triangle).Aspect(); math.Abs(a-2/math.Sqrt(3)) > 1e-9 {
		t.Errorf("triangle aspect %v", a)
	}
	left := Default(Triangle)
	left.Apex = Left
	if a := left.Aspect(); math.Abs(a-math.Sqrt(3)/2) > 1e-9 {
		t.Errorf("left triangle aspect %v", a)
	}
	// Regular hexagon with flat top: width / height = 2 / sqrt(3).
	if a := Default(Polygon).Aspect(); math.Abs(a-2/math.Sqrt(3)) > 1e-9 {
		t.Errorf("hexagon aspect %v", a)
	}
}
