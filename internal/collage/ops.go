package collage

import (
	"math"

	"github.com/mjaepel/picframer/internal/geom"
)

// Edge says where a dragged picture is dropped relative to a target cell.
type Edge uint8

const (
	Center Edge = iota // swap with the target
	Left
	Right
	Top
	Bottom
)

// find returns the leaf node holding c, its parent (nil for the root) and
// its index in the parent.
func find(n *Node, c *Cell) (leaf, parent *Node, idx int) {
	if n == nil {
		return nil, nil, -1
	}
	if n.IsLeaf() {
		if n.Cell == c {
			return n, nil, -1
		}
		return nil, nil, -1
	}
	for i, ch := range n.Children {
		if ch.IsLeaf() && ch.Cell == c {
			return ch, n, i
		}
		if l, p, j := find(ch, c); l != nil {
			return l, p, j
		}
	}
	return nil, nil, -1
}

// Contains reports whether c is part of the tree.
func Contains(root *Node, c *Cell) bool {
	l, _, _ := find(root, c)
	return l != nil
}

// Swap exchanges the positions of two pictures.
func Swap(root *Node, a, b *Cell) {
	la, _, _ := find(root, a)
	lb, _, _ := find(root, b)
	if la == nil || lb == nil {
		return
	}
	la.Cell, lb.Cell = lb.Cell, la.Cell
}

// Remove takes c out of the tree and returns the new root (nil if the
// collage is empty afterwards).
func Remove(root *Node, c *Cell) *Node {
	leaf, parent, idx := find(root, c)
	if leaf == nil {
		return root
	}
	if parent == nil {
		return nil
	}
	parent.Children = append(parent.Children[:idx], parent.Children[idx+1:]...)
	parent.Weights = append(parent.Weights[:idx], parent.Weights[idx+1:]...)
	renormalize(parent.Weights)
	return normalize(root)
}

// Insert places c next to target on the given edge and returns the new root.
// The target's area is shared between both pictures according to their
// aspect ratios. Edge Center is treated like Right.
func Insert(root *Node, c, target *Cell, edge Edge) *Node {
	t, parent, idx := find(root, target)
	if t == nil {
		return root
	}
	dir := Row
	if edge == Top || edge == Bottom {
		dir = Column
	}
	nl := Leaf(c)
	children := []*Node{t, nl}
	if edge == Left || edge == Top {
		children = []*Node{nl, t}
	}
	s := &Node{Dir: dir, Children: children, Weights: make([]float64, 2)}
	s.FitWeights()
	if parent == nil {
		return normalize(s)
	}
	parent.Children[idx] = s
	return normalize(root)
}

// Move takes c out of its place and inserts it at edge of target. Edge
// Center swaps the two pictures instead.
func Move(root *Node, c, target *Cell, edge Edge) *Node {
	if c == target || !Contains(root, c) || !Contains(root, target) {
		return root
	}
	if edge == Center {
		Swap(root, c, target)
		return root
	}
	root = Remove(root, c)
	return Insert(root, c, target, edge)
}

// Add inserts c beside the largest picture and returns the new root.
func Add(root *Node, c *Cell) *Node {
	if root == nil {
		return Leaf(c)
	}
	a := root.NaturalAspect()
	cells, _ := Layout(root, geom.Rect{W: a, H: 1}, 0)
	best := cells[0]
	for _, p := range cells[1:] {
		if p.Rect.Area() > best.Rect.Area() {
			best = p
		}
	}
	edge := Bottom
	if best.Rect.W >= best.Rect.H {
		edge = Right
	}
	return Insert(root, c, best.Cell, edge)
}

// MoveDivider shifts divider d by delta layout units, keeping each of the
// two neighbouring children at least minSize units long.
func MoveDivider(d Divider, delta, minSize float64) {
	n, i := d.Node, d.Index
	if d.Avail <= 0 || i < 0 || i+1 >= len(n.Weights) {
		return
	}
	pair := n.Weights[i] + n.Weights[i+1]
	minW := math.Min(minSize/d.Avail, pair/2)
	w := n.Weights[i] + delta/d.Avail
	w = math.Min(math.Max(w, minW), pair-minW)
	n.Weights[i], n.Weights[i+1] = w, pair-w
}

// normalize collapses splits with fewer than two children and merges
// nested splits of the same direction. It returns the (possibly new) root.
func normalize(n *Node) *Node {
	if n == nil || n.IsLeaf() {
		return n
	}
	var children []*Node
	var weights []float64
	for i, c := range n.Children {
		c = normalize(c)
		if c == nil {
			continue
		}
		if !c.IsLeaf() && c.Dir == n.Dir {
			for j, cc := range c.Children {
				children = append(children, cc)
				weights = append(weights, n.Weights[i]*c.Weights[j])
			}
			continue
		}
		children = append(children, c)
		weights = append(weights, n.Weights[i])
	}
	switch len(children) {
	case 0:
		return nil
	case 1:
		return children[0]
	}
	renormalize(weights)
	n.Children, n.Weights = children, weights
	return n
}

func renormalize(w []float64) {
	var sum float64
	for _, v := range w {
		sum += v
	}
	if sum <= 0 {
		for i := range w {
			w[i] = 1 / float64(len(w))
		}
		return
	}
	for i := range w {
		w[i] /= sum
	}
}
