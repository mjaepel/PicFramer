package collage

import (
	"math"
	"math/rand"
	"testing"

	"github.com/mjaepel/picframer/internal/crop"
	"github.com/mjaepel/picframer/internal/geom"
)

func rc(x, y, w, h float64) geom.Rect { return geom.Rect{X: x, Y: y, W: w, H: h} }

func mkCells(aspects ...float64) []*Cell {
	cells := make([]*Cell, len(aspects))
	for i, a := range aspects {
		cells[i] = &Cell{ID: i, Aspect: a, View: crop.Default()}
	}
	return cells
}

// checkLayout verifies that cells don't overlap, stay inside r and cover
// r except for the gaps.
func checkLayout(t *testing.T, root *Node, r geom.Rect, gap float64) {
	t.Helper()
	if err := Validate(root); err != nil {
		t.Fatalf("invalid tree: %v", err)
	}
	placed, divs := Layout(root, r, gap)
	var area float64
	for i, a := range placed {
		if a.Rect.W < -1e-9 || a.Rect.H < -1e-9 {
			t.Fatalf("negative size %+v", a.Rect)
		}
		if a.Rect.X < r.X-1e-6 || a.Rect.Y < r.Y-1e-6 ||
			a.Rect.X+a.Rect.W > r.X+r.W+1e-6 || a.Rect.Y+a.Rect.H > r.Y+r.H+1e-6 {
			t.Fatalf("cell %+v outside %+v", a.Rect, r)
		}
		area += a.Rect.Area()
		for _, b := range placed[i+1:] {
			if ov := a.Rect.Intersect(b.Rect).Area(); ov > 1e-6 {
				t.Fatalf("cells overlap by %v: %+v %+v", ov, a.Rect, b.Rect)
			}
		}
	}
	for _, d := range divs {
		area += d.Rect.Area()
	}
	if gap == 0 && math.Abs(area-r.Area()) > 1e-6*r.Area() {
		t.Fatalf("cells cover %v of %v", area, r.Area())
	}
	if gap > 0 && area > r.Area()+1e-6 {
		t.Fatalf("cells and gaps cover %v > %v", area, r.Area())
	}
}

func TestLayoutSimple(t *testing.T) {
	c := mkCells(1, 1, 1)
	root := Split(Row, Leaf(c[0]), Split(Column, Leaf(c[1]), Leaf(c[2])))
	placed, divs := Layout(root, geom.Rect{W: 210, H: 100}, 10)
	if len(placed) != 3 || len(divs) != 2 {
		t.Fatalf("got %d cells, %d dividers", len(placed), len(divs))
	}
	want := []geom.Rect{rc(0, 0, 100, 100), rc(110, 0, 100, 45), rc(110, 55, 100, 45)}
	for i, p := range placed {
		if p.Rect != want[i] {
			t.Errorf("cell %d: %+v, want %+v", i, p.Rect, want[i])
		}
	}
}

func TestAutoExactFit(t *testing.T) {
	for _, aspects := range [][]float64{
		{1.5},
		{1.5, 0.66},
		{1.5, 1.5, 0.75},
		{1.33, 0.75, 1, 1.78, 0.56},
		{1.5, 1.5, 1.5, 1.5, 0.66, 0.66, 1, 1},
		{1.5, 0.66, 1.5, 0.66, 1.5, 0.66, 1.5, 0.66, 1, 1, 1.78, 0.56},
	} {
		cells := mkCells(aspects...)
		root := Auto(cells, 0)
		if err := Validate(root); err != nil {
			t.Fatalf("%v: %v", aspects, err)
		}
		if got := len(root.Cells()); got != len(cells) {
			t.Fatalf("%v: %d cells in tree", aspects, got)
		}
		// Keeps the order.
		for i, c := range root.Cells() {
			if c != cells[i] {
				t.Fatalf("%v: order changed", aspects)
			}
		}
		a := root.NaturalAspect()
		placed, _ := Layout(root, geom.Rect{W: a * 1000, H: 1000}, 0)
		for _, p := range placed {
			if got, want := p.Rect.W/p.Rect.H, p.Cell.Aspect; math.Abs(got/want-1) > 1e-6 {
				t.Errorf("%v: cell aspect %v, want %v", aspects, got, want)
			}
		}
		if a > autoMaxAspect*1.5 || a < 1/(autoMaxAspect*1.5) {
			t.Errorf("%v: unpleasant aspect %v", aspects, a)
		}
	}
}

func TestAutoTarget(t *testing.T) {
	// Four landscape pictures on a square canvas: a 2x2 grid is ideal.
	cells := mkCells(1.5, 1.5, 1.5, 1.5)
	root := Auto(cells, 1.5)
	if a := root.NaturalAspect(); math.Abs(a-1.5) > 1e-9 {
		t.Errorf("aspect %v, want 1.5 (2x2 grid)", a)
	}
	// Three portraits side by side give a wide strip.
	cells = mkCells(0.5, 0.5, 0.5)
	root = Auto(cells, 1.5)
	if a := root.NaturalAspect(); math.Abs(a-1.5) > 1e-9 {
		t.Errorf("aspect %v, want 1.5 (one row)", a)
	}
}

func TestRotationChangesAspect(t *testing.T) {
	c := &Cell{Aspect: 2, View: crop.View{Zoom: 1, Rotation: 90}}
	if c.EffAspect() != 0.5 {
		t.Errorf("eff aspect %v", c.EffAspect())
	}
	c.View.Rotation = -180
	if c.EffAspect() != 2 {
		t.Errorf("eff aspect %v", c.EffAspect())
	}
}

func TestSwapMoveRemove(t *testing.T) {
	c := mkCells(1, 1, 1)
	root := Split(Row, Leaf(c[0]), Leaf(c[1]), Leaf(c[2]))
	Swap(root, c[0], c[2])
	if got := root.Cells(); got[0] != c[2] || got[2] != c[0] {
		t.Fatal("swap failed")
	}
	// Move c[2] (now first) below c[1].
	root = Move(root, c[2], c[1], Bottom)
	if err := Validate(root); err != nil {
		t.Fatal(err)
	}
	if root.Dir != Row || len(root.Children) != 2 || root.Children[0].IsLeaf() == false && root.Children[0].Dir != Column {
		t.Fatalf("unexpected structure after move")
	}
	got := root.Cells()
	if got[0] != c[1] || got[1] != c[2] || got[2] != c[0] {
		t.Fatalf("order after move: %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}
	root = Remove(root, c[1])
	root = Remove(root, c[2])
	if !root.IsLeaf() || root.Cell != c[0] {
		t.Fatal("remove did not collapse to the remaining leaf")
	}
	if Remove(root, c[0]) != nil {
		t.Fatal("removing the last picture should give an empty tree")
	}
}

func TestMoveDivider(t *testing.T) {
	c := mkCells(1, 1)
	root := Split(Row, Leaf(c[0]), Leaf(c[1]))
	_, divs := Layout(root, geom.Rect{W: 200, H: 100}, 0)
	MoveDivider(divs[0], 50, 20)
	if math.Abs(root.Weights[0]-0.75) > 1e-9 {
		t.Errorf("weights %v", root.Weights)
	}
	MoveDivider(divs[0], 1000, 20)
	if math.Abs(root.Weights[1]-0.1) > 1e-9 {
		t.Errorf("min size not respected: %v", root.Weights)
	}
}

func TestClone(t *testing.T) {
	c := mkCells(1, 2)
	root := Split(Row, Leaf(c[0]), Leaf(c[1]))
	cl := root.Clone()
	cl.Weights[0] = 0.9
	cl.Children[0].Cell.Aspect = 5
	if root.Weights[0] != 0.5 || c[0].Aspect != 1 {
		t.Fatal("clone shares state with the original")
	}
}

// Random sequences of operations keep the tree valid and the layout free of
// overlaps.
func TestRandomOperations(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for run := 0; run < 200; run++ {
		var root *Node
		var cells []*Cell
		next := 0
		for step := 0; step < 40; step++ {
			switch op := rng.Intn(6); {
			case op == 0 || len(cells) < 2:
				c := &Cell{ID: next, Aspect: 0.4 + rng.Float64()*2, View: crop.Default()}
				next++
				cells = append(cells, c)
				root = Add(root, c)
			case op == 1:
				a, b := cells[rng.Intn(len(cells))], cells[rng.Intn(len(cells))]
				Swap(root, a, b)
			case op == 2:
				a, b := cells[rng.Intn(len(cells))], cells[rng.Intn(len(cells))]
				root = Move(root, a, b, Edge(rng.Intn(5)))
			case op == 3:
				i := rng.Intn(len(cells))
				root = Remove(root, cells[i])
				cells = append(cells[:i], cells[i+1:]...)
			case op == 4:
				_, divs := Layout(root, geom.Rect{W: 800, H: 600}, 4)
				if len(divs) > 0 {
					d := divs[rng.Intn(len(divs))]
					MoveDivider(d, (rng.Float64()-0.5)*400, 16)
				}
			case op == 5:
				root = Auto(cells, []float64{0, 1, 1.5, 0.8}[rng.Intn(4)])
			}
			if len(cells) == 0 {
				if root != nil {
					t.Fatal("empty collage should have nil root")
				}
				continue
			}
			if got := len(root.Cells()); got != len(cells) {
				t.Fatalf("run %d step %d: tree has %d cells, want %d", run, step, got, len(cells))
			}
			checkLayout(t, root, geom.Rect{X: 10, Y: 20, W: 800, H: 600}, 0)
			checkLayout(t, root, geom.Rect{W: 800, H: 600}, 6)
		}
	}
}

func TestArrange(t *testing.T) {
	aspects := []float64{1.5, 0.75, 1, 1.78, 0.56, 1.33, 1.5}
	sum, inv := 0.0, 0.0
	for _, a := range aspects {
		sum += a
		inv += 1 / a
	}
	for _, tc := range []struct {
		name   string
		a      Arrangement
		cols   int
		aspect float64 // expected natural aspect, 0 = don't check
	}{
		{"row", ArrangeRow, 0, sum},
		{"column", ArrangeColumn, 0, 1 / inv},
		{"grid3", ArrangeGrid, 3, 0},
		{"grid1", ArrangeGrid, 1, 1 / inv},
		{"grid-wide", ArrangeGrid, 20, sum},
		{"auto", ArrangeAuto, 0, 0},
	} {
		cells := mkCells(aspects...)
		root := Arrange(cells, tc.a, 0, tc.cols)
		if err := Validate(root); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		for i, c := range root.Cells() {
			if c != cells[i] {
				t.Fatalf("%s: order changed", tc.name)
			}
		}
		a := root.NaturalAspect()
		if tc.aspect > 0 && math.Abs(a-tc.aspect) > 1e-9 {
			t.Errorf("%s: aspect %v, want %v", tc.name, a, tc.aspect)
		}
		placed, _ := Layout(root, geom.Rect{W: a * 1000, H: 1000}, 0)
		for _, p := range placed {
			if got := p.Rect.W / p.Rect.H; math.Abs(got/p.Cell.Aspect-1) > 1e-6 {
				t.Errorf("%s: cell aspect %v, want %v", tc.name, got, p.Cell.Aspect)
			}
		}
	}
	// Grid with 3 columns: rows of 3, 3 and 1 pictures.
	root := Arrange(mkCells(aspects...), ArrangeGrid, 3, 3)
	if root.Dir != Column || len(root.Children) != 3 || !root.Children[2].IsLeaf() || len(root.Children[0].Children) != 3 {
		t.Errorf("unexpected grid structure")
	}
	if Arrange(nil, ArrangeRow, 0, 0) != nil {
		t.Error("empty arrangement should be nil")
	}
}
