package collage

// Arrangement selects how pictures are laid out.
type Arrangement int

const (
	ArrangeAuto   Arrangement = iota // best slicing layout (see Auto)
	ArrangeRow                       // all pictures side by side
	ArrangeColumn                    // all pictures stacked
	ArrangeGrid                      // rows of a fixed number of pictures
)

// Arrange builds a layout for cells in their order. target is the desired
// canvas aspect ratio (0 = any) and is only used by ArrangeAuto; cols is the
// number of pictures per row for ArrangeGrid. The weights are set so that
// every picture fits its cell exactly at the layout's natural aspect ratio.
func Arrange(cells []*Cell, a Arrangement, target float64, cols int) *Node {
	switch {
	case len(cells) == 0:
		return nil
	case len(cells) == 1:
		return Leaf(cells[0])
	}
	var root *Node
	switch a {
	case ArrangeRow:
		root = line(Row, cells)
	case ArrangeColumn:
		root = line(Column, cells)
	case ArrangeGrid:
		cols = max(cols, 1)
		var rows []*Node
		for i := 0; i < len(cells); i += cols {
			rows = append(rows, line(Row, cells[i:min(i+cols, len(cells))]))
		}
		if len(rows) == 1 {
			root = rows[0]
		} else {
			root = Split(Column, rows...)
		}
	default:
		return Auto(cells, target)
	}
	root.FitWeights()
	return root
}

// line returns a single picture or a split of all cells in direction d.
func line(d Dir, cells []*Cell) *Node {
	if len(cells) == 1 {
		return Leaf(cells[0])
	}
	leaves := make([]*Node, len(cells))
	for i, c := range cells {
		leaves[i] = Leaf(c)
	}
	return Split(d, leaves...)
}
