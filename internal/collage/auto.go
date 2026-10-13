package collage

import (
	"math"
)

// bruteForceMax is the largest number of pictures for which every slicing
// layout is evaluated. Above it, only rows/columns layouts are considered.
const bruteForceMax = 8

// Preferred aspect range when the canvas aspect is not fixed.
const autoMaxAspect = 1.8

// Without a target, layouts close to this landscape aspect are slightly preferred.
const (
	autoPreferred    = 4.0 / 3
	autoPreferWeight = 0.5
)

// Auto builds a layout for cells, keeping their order. target is the
// desired canvas aspect ratio; 0 lets the layout choose a pleasant one.
// The weights are set so that every picture fits its cell exactly at the
// layout's natural aspect ratio.
func Auto(cells []*Cell, target float64) *Node {
	switch len(cells) {
	case 0:
		return nil
	case 1:
		return Leaf(cells[0])
	}
	var cands []*Node
	if len(cells) <= bruteForceMax {
		cands = enumerate(cells, 0, len(cells))
	} else {
		cands = rowsAndColumns(cells)
	}
	var best *Node
	bestCost := math.Inf(1)
	for _, t := range cands {
		if c := Cost(t, target); c < bestCost-1e-9 {
			best, bestCost = t, c
		}
	}
	best = cloneStructure(best)
	best.FitWeights()
	return best
}

// Cost rates a layout: how far its natural aspect is from the target and
// how unequal the picture sizes are. Lower is better.
func Cost(t *Node, target float64) float64 {
	a := t.NaturalAspect()
	var c float64
	if target > 0 {
		c = 3 * math.Abs(math.Log(a/target))
	} else {
		c = 3*math.Max(0, math.Abs(math.Log(a))-math.Log(autoMaxAspect)) +
			autoPreferWeight*math.Abs(math.Log(a/autoPreferred))
	}
	var areas []float64
	naturalAreas(t, a, 1, &areas)
	var total float64
	for _, v := range areas {
		total += v
	}
	n := float64(len(areas))
	var dev float64
	for _, v := range areas {
		l := math.Log(v / total * n)
		dev += l * l
	}
	return c + math.Sqrt(dev/n)
}

// naturalAreas appends the area of each picture when t is laid out at its
// natural aspect in a w×h box.
func naturalAreas(t *Node, w, h float64, out *[]float64) {
	if t.IsLeaf() {
		*out = append(*out, w*h)
		return
	}
	for _, c := range t.Children {
		a := c.NaturalAspect()
		if t.Dir == Row {
			naturalAreas(c, a*h, h, out)
		} else {
			naturalAreas(c, w, w/a, out)
		}
	}
}

// enumerate returns every slicing tree over cells[i:j] (order preserved,
// no nested splits of the same direction). Subtrees are shared between
// results, so callers must not modify them.
func enumerate(cells []*Cell, i, j int) []*Node {
	if j-i == 1 {
		return []*Node{Leaf(cells[i])}
	}
	return append(splits(cells, i, j, Row), splits(cells, i, j, Column)...)
}

// splits returns all trees over cells[i:j] whose root splits in direction d.
func splits(cells []*Cell, i, j int, d Dir) []*Node {
	var out []*Node
	// sub returns trees for a group: a leaf or a split of the other direction.
	sub := func(a, b int) []*Node {
		if b-a == 1 {
			return []*Node{Leaf(cells[a])}
		}
		return splits(cells, a, b, d.other())
	}
	var rec func(start int, acc []*Node)
	rec = func(start int, acc []*Node) {
		if start == j {
			if len(acc) >= 2 {
				out = append(out, &Node{Dir: d, Children: append([]*Node(nil), acc...)})
			}
			return
		}
		for end := start + 1; end <= j; end++ {
			if start == i && end == j {
				continue // a single group covering everything is not a split
			}
			for _, t := range sub(start, end) {
				rec(end, append(acc, t))
			}
		}
	}
	rec(i, nil)
	return out
}

// rowsAndColumns returns "justified" layouts: k rows of pictures (or k
// columns), with the groups balanced by their total width (height).
func rowsAndColumns(cells []*Cell) []*Node {
	var out []*Node
	n := len(cells)
	for _, d := range []Dir{Column, Row} {
		size := make([]float64, n)
		for i, c := range cells {
			size[i] = c.EffAspect() // a row's width at unit height
			if d == Row {
				size[i] = 1 / size[i] // a column's height at unit width
			}
		}
		for k := 1; k <= n; k++ {
			groups := partition(size, k)
			var children []*Node
			for _, g := range groups {
				if g[1]-g[0] == 1 {
					children = append(children, Leaf(cells[g[0]]))
				} else {
					var leaves []*Node
					for _, c := range cells[g[0]:g[1]] {
						leaves = append(leaves, Leaf(c))
					}
					children = append(children, Split(d.other(), leaves...))
				}
			}
			if len(children) == 1 {
				out = append(out, children[0])
			} else {
				out = append(out, Split(d, children...))
			}
		}
	}
	return out
}

// partition splits sizes into k contiguous groups minimizing the largest
// group sum (linear partition). It returns [start, end) index pairs.
func partition(sizes []float64, k int) [][2]int {
	n := len(sizes)
	prefix := make([]float64, n+1)
	for i, s := range sizes {
		prefix[i+1] = prefix[i] + s
	}
	// cost[g][i]: minimal max-sum for the first i items in g groups.
	cost := make([][]float64, k+1)
	cut := make([][]int, k+1)
	for g := range cost {
		cost[g] = make([]float64, n+1)
		cut[g] = make([]int, n+1)
		for i := range cost[g] {
			cost[g][i] = math.Inf(1)
		}
	}
	cost[0][0] = 0
	for g := 1; g <= k; g++ {
		for i := g; i <= n; i++ {
			for p := g - 1; p < i; p++ {
				c := math.Max(cost[g-1][p], prefix[i]-prefix[p])
				if c < cost[g][i] {
					cost[g][i], cut[g][i] = c, p
				}
			}
		}
	}
	groups := make([][2]int, k)
	end := n
	for g := k; g >= 1; g-- {
		start := cut[g][end]
		groups[g-1] = [2]int{start, end}
		end = start
	}
	return groups
}

// cloneStructure copies the split nodes of t, keeping the cell pointers.
func cloneStructure(t *Node) *Node {
	if t.IsLeaf() {
		return &Node{Cell: t.Cell}
	}
	m := &Node{Dir: t.Dir, Weights: make([]float64, len(t.Children))}
	for _, c := range t.Children {
		m.Children = append(m.Children, cloneStructure(c))
	}
	return m
}
