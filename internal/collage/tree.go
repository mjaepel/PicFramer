// Package collage models a collage as a slicing tree: every node is either
// a single picture (leaf) or a row/column that splits its area among its
// children. Because each split only divides its own area, pictures can never
// overlap, whatever operations are applied.
package collage

import (
	"errors"
	"fmt"
	"math"

	"github.com/mjaepel/picframer/internal/crop"
	"github.com/mjaepel/picframer/internal/geom"
)

// Dir is the direction in which a split arranges its children.
type Dir uint8

const (
	Row    Dir = iota // children side by side
	Column            // children stacked
)

func (d Dir) other() Dir { return 1 - d }

// Cell is one picture in the collage.
type Cell struct {
	// ID identifies the picture for the caller (index into its image list).
	ID int
	// Aspect is the width/height ratio of the source image.
	Aspect float64
	// Contain shows the whole image instead of covering the cell.
	Contain bool
	// View positions the image inside the cell (cover mode). Only the
	// rotation is used in contain mode.
	View crop.View
}

// EffAspect is the aspect ratio of the picture after its rotation in
// quarter turns.
func (c *Cell) EffAspect() float64 {
	if quarter(c.View.Rotation)%2 == 1 {
		return 1 / c.Aspect
	}
	return c.Aspect
}

func quarter(deg float64) int {
	q := int(math.Round(deg/90)) % 4
	if q < 0 {
		q += 4
	}
	return q
}

// Node is a leaf (Cell != nil) or a split with at least two children.
type Node struct {
	Cell     *Cell
	Dir      Dir
	Children []*Node
	// Weights are the children's shares of the available length; they sum to 1.
	Weights []float64
}

// IsLeaf reports whether n is a single picture.
func (n *Node) IsLeaf() bool { return n.Cell != nil }

// Leaf returns a leaf node for c.
func Leaf(c *Cell) *Node { return &Node{Cell: c} }

// Split returns a split node with equal weights.
func Split(d Dir, children ...*Node) *Node {
	w := make([]float64, len(children))
	for i := range w {
		w[i] = 1 / float64(len(children))
	}
	return &Node{Dir: d, Children: children, Weights: w}
}

// Clone returns a deep copy of the tree, including the cells.
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	if n.IsLeaf() {
		c := *n.Cell
		return &Node{Cell: &c}
	}
	m := &Node{Dir: n.Dir, Weights: append([]float64(nil), n.Weights...)}
	for _, ch := range n.Children {
		m.Children = append(m.Children, ch.Clone())
	}
	return m
}

// Cells returns all cells in depth-first order.
func (n *Node) Cells() []*Cell {
	var out []*Cell
	n.walk(func(m *Node) {
		if m.IsLeaf() {
			out = append(out, m.Cell)
		}
	})
	return out
}

func (n *Node) walk(f func(*Node)) {
	if n == nil {
		return
	}
	f(n)
	for _, c := range n.Children {
		c.walk(f)
	}
}

// NaturalAspect is the aspect ratio at which every picture fits its cell
// exactly, given the tree's structure and weights are chosen to match. It
// ignores weights.
func (n *Node) NaturalAspect() float64 {
	if n.IsLeaf() {
		return n.Cell.EffAspect()
	}
	var sum float64
	for _, c := range n.Children {
		a := c.NaturalAspect()
		if n.Dir == Row {
			sum += a
		} else {
			sum += 1 / a
		}
	}
	if n.Dir == Row {
		return sum
	}
	return 1 / sum
}

// FitWeights sets all weights so that every picture fits its cell exactly
// at the tree's natural aspect ratio.
func (n *Node) FitWeights() {
	if n.IsLeaf() {
		return
	}
	var sum float64
	for i, c := range n.Children {
		c.FitWeights()
		a := c.NaturalAspect()
		if n.Dir == Column {
			a = 1 / a
		}
		n.Weights[i] = a
		sum += a
	}
	for i := range n.Weights {
		n.Weights[i] /= sum
	}
}

// Placed is a cell with its computed rectangle.
type Placed struct {
	Cell *Cell
	Rect geom.Rect
}

// Divider is a draggable boundary between two children of a split.
type Divider struct {
	Node  *Node
	Index int // boundary between Children[Index] and Children[Index+1]
	// Rect is the gap area between the two children (at least 1 unit thick).
	Rect geom.Rect
	// Avail is the split's length available to its children (excluding gaps);
	// moving the divider by d units changes the weights by d/Avail.
	Avail float64
}

// Layout computes the rectangles of all cells and dividers within r.
// gap is the space between neighbouring pictures.
func Layout(n *Node, r geom.Rect, gap float64) ([]Placed, []Divider) {
	var cells []Placed
	var divs []Divider
	layout(n, r, gap, &cells, &divs)
	return cells, divs
}

func layout(n *Node, r geom.Rect, gap float64, cells *[]Placed, divs *[]Divider) {
	if n.IsLeaf() {
		*cells = append(*cells, Placed{n.Cell, r})
		return
	}
	length := r.W
	if n.Dir == Column {
		length = r.H
	}
	k := len(n.Children)
	avail := math.Max(length-gap*float64(k-1), 0)
	pos := 0.0
	for i, c := range n.Children {
		size := n.Weights[i] * avail
		if i == k-1 { // absorb rounding so the children exactly fill r
			size = math.Max(length-pos, 0)
		}
		var cr geom.Rect
		if n.Dir == Row {
			cr = geom.Rect{X: r.X + pos, Y: r.Y, W: size, H: r.H}
		} else {
			cr = geom.Rect{X: r.X, Y: r.Y + pos, W: r.W, H: size}
		}
		layout(c, cr, gap, cells, divs)
		pos += size
		if i < k-1 {
			var dr geom.Rect
			if n.Dir == Row {
				dr = geom.Rect{X: r.X + pos, Y: r.Y, W: gap, H: r.H}
			} else {
				dr = geom.Rect{X: r.X, Y: r.Y + pos, W: r.W, H: gap}
			}
			*divs = append(*divs, Divider{Node: n, Index: i, Rect: dr, Avail: avail})
			pos += gap
		}
	}
}

// Validate checks the structural invariants of the tree.
func Validate(n *Node) error {
	if n == nil {
		return nil
	}
	if n.IsLeaf() {
		if len(n.Children) > 0 {
			return errors.New("leaf with children")
		}
		return nil
	}
	if len(n.Children) < 2 {
		return fmt.Errorf("split with %d children", len(n.Children))
	}
	if len(n.Weights) != len(n.Children) {
		return errors.New("weights do not match children")
	}
	var sum float64
	for i, c := range n.Children {
		if c == nil {
			return errors.New("nil child")
		}
		if !c.IsLeaf() && c.Dir == n.Dir {
			return errors.New("nested split with same direction")
		}
		if !(n.Weights[i] > 0) {
			return fmt.Errorf("non-positive weight %v", n.Weights[i])
		}
		sum += n.Weights[i]
		if err := Validate(c); err != nil {
			return err
		}
	}
	if math.Abs(sum-1) > 1e-6 {
		return fmt.Errorf("weights sum to %v", sum)
	}
	return nil
}
