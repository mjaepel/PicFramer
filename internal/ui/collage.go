package ui

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"strconv"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/mjaepel/picframer/internal/collage"
	"github.com/mjaepel/picframer/internal/crop"
	"github.com/mjaepel/picframer/internal/geom"
	"github.com/mjaepel/picframer/internal/imgio"
	"github.com/mjaepel/picframer/internal/render"
	"github.com/mjaepel/picframer/internal/shape"
)

const (
	collagePreviewSize = 1200
	defaultLongEdge    = 3000
	maxGap             = 100 // slider range; larger values can be typed
	maxMargin          = 200
	maxSpacing         = 2000
	maxGridCols        = 12
	// Limits of the exported collage: JPEG allows at most 65535 px per
	// side, and the image is held in memory (4 bytes per pixel).
	maxCollageSide   = 65535
	maxCollagePixels = 300e6
)

// collageItem is a loaded picture. The full image is decoded again from
// path when exporting, so only the small preview stays in memory.
type collageItem struct {
	path    string
	w, h    float64
	preview paint.ImageOp
}

// collageSnap is the undoable state of the collage mode.
type collageSnap struct {
	root       *collage.Node
	aspect     float64
	autoAspect bool
	ratio      shape.Ratio
}

type dragMode int

const (
	dragNone dragMode = iota
	dragPan
	dragDivider
	dragMove
	dragRotate
)

type cellButtons struct {
	fit, rotate, remove button
}

type collagePage struct {
	a *App

	items  map[int]*collageItem
	nextID int

	root       *collage.Node
	aspect     float64
	autoAspect bool
	ratio      shape.Ratio
	defContain bool
	hist       history[collageSnap]

	gapPx, marginPx int
	longEdge        int
	nativeSize      bool // size so that no picture is scaled down
	jpeg            bool
	arrangement     collage.Arrangement
	gridCols        int

	addBtn, emptyAddBtn, clearBtn, relayoutBtn, exportBtn, flipBtn button
	ratioChoice, fitChoice, format, arrChoice, sizeChoice          choice
	ratioField, longField                                          field
	ratioInvalid                                                   bool
	gap, margin, cols                                              numSlider
	quality                                                        slider
	bg                                                             *bgPicker
	cellBtns                                                       map[int]*cellButtons

	// Canvas state, in canvas coordinates.
	tag      struct{}
	board    geom.Rect // the collage area
	scale    float64   // canvas units per output pixel
	placed   []collage.Placed
	divs     []collage.Divider
	pointer  f32.Point
	hover    *collage.Cell
	hoverDiv int // index into divs, -1 if none

	drag       dragMode
	dragCell   *collage.Cell
	dragDivRef collage.Divider
	lastPos    f32.Point
	dropCell   *collage.Cell
	dropEdge   collage.Edge
	// Rotation by right-dragging: unsnapped target angle, last pointer angle
	// and the picture's aspect ratio when the drag started.
	rotAccum, lastAngle, rotAspect float64
}

func newCollagePage(a *App) *collagePage {
	p := &collagePage{
		a:          a,
		items:      map[int]*collageItem{},
		autoAspect: true,
		ratio:      shape.Ratio{W: 3, H: 2},
		gapPx:      20,
		marginPx:   20,
		longEdge:   defaultLongEdge,
		gridCols:   3,
		bg:         newBgPicker(false, rgb(0xffffff)),
		cellBtns:   map[int]*cellButtons{},
		hoverDiv:   -1,
	}
	p.quality.Value = (92 - 50) / 50.0
	p.ratioField.SetText(p.ratio.String())
	p.longField.SetText(strconv.Itoa(p.longEdge))
	return p
}

func (p *collagePage) cells() []*collage.Cell {
	if p.root == nil {
		return nil
	}
	return p.root.Cells()
}

func (p *collagePage) snap() collageSnap {
	return collageSnap{root: p.root.Clone(), aspect: p.aspect, autoAspect: p.autoAspect, ratio: p.ratio}
}

func (p *collagePage) restore(s collageSnap) {
	p.root, p.aspect, p.autoAspect, p.ratio = s.root, s.aspect, s.autoAspect, s.ratio
	p.hover, p.dragCell, p.dropCell, p.drag = nil, nil, nil, dragNone
	p.ratioField.SetText(p.ratio.String())
}

func (p *collagePage) change(key string) { p.hist.push(p.snap(), key) }

func (p *collagePage) canUndo() bool { return p.hist.canUndo() }
func (p *collagePage) canRedo() bool { return p.hist.canRedo() }

func (p *collagePage) undo() {
	if s, ok := p.hist.doUndo(p.snap()); ok {
		p.restore(s)
	}
}

func (p *collagePage) redo() {
	if s, ok := p.hist.doRedo(p.snap()); ok {
		p.restore(s)
	}
}

func (p *collagePage) open() {
	p.a.chooseImages(true, p.addPaths)
}

// addPaths loads pictures in the background and adds them to the collage.
func (p *collagePage) addPaths(paths []string) {
	go func() {
		type loaded struct {
			path string
			w, h float64
			prev paint.ImageOp
		}
		var res []loaded
		var errs int
		for i, path := range paths {
			i := i
			p.a.post(func() { p.a.setStatus(fmt.Sprintf("Loading %d of %d …", i+1, len(paths))) })
			img, err := imgio.Load(path)
			if err != nil {
				errs++
				continue
			}
			b := img.Bounds()
			res = append(res, loaded{path, float64(b.Dx()), float64(b.Dy()), paint.NewImageOp(imgio.Thumbnail(img, collagePreviewSize))})
		}
		p.a.post(func() {
			if len(res) > 0 {
				p.change("")
			}
			cells := p.cells()
			for _, l := range res {
				id := p.nextID
				p.nextID++
				p.items[id] = &collageItem{path: l.path, w: l.w, h: l.h, preview: l.prev}
				cells = append(cells, &collage.Cell{ID: id, Aspect: l.w / l.h, Contain: p.defContain, View: crop.Default()})
			}
			p.arrange(cells)
			msg := fmt.Sprintf("%d pictures in the collage.", len(cells))
			if errs > 0 {
				p.a.setError(fmt.Errorf("%d file(s) could not be opened; %s", errs, msg))
				return
			}
			p.a.setStatus(msg)
		})
	}()
}

func (p *collagePage) target() float64 {
	if p.autoAspect {
		return 0
	}
	return p.ratio.Aspect()
}

// arrange builds a new layout for cells using the chosen arrangement.
func (p *collagePage) arrange(cells []*collage.Cell) {
	p.root = collage.Arrange(cells, p.arrangement, p.target(), p.gridCols)
	p.refit()
}

// refit sizes all cells to fit their pictures and updates the canvas aspect.
func (p *collagePage) refit() {
	if p.root == nil {
		return
	}
	p.root.FitWeights()
	if p.autoAspect {
		p.aspect = p.root.NaturalAspect()
	} else {
		p.aspect = p.ratio.Aspect()
	}
}

func (p *collagePage) outputSize() (int, int) {
	w, h, _ := p.size()
	return w, h
}

// size returns the export size and whether it had to be reduced to the
// maximum size.
func (p *collagePage) size() (w, h int, limited bool) {
	l := float64(p.longEdge)
	if p.nativeSize {
		l = p.nativeLongEdge()
	}
	lw, lh := p.dims(l)
	if f := math.Min(maxCollageSide/math.Max(lw, lh), math.Sqrt(maxCollagePixels/(lw*lh))); f < 1 {
		lw, lh = p.dims(l * f)
		limited = true
	}
	return max(1, int(math.Round(lw))), max(1, int(math.Round(lh))), limited
}

// dims returns width and height for a long edge of l pixels.
func (p *collagePage) dims(l float64) (float64, float64) {
	a := p.aspect
	if a <= 0 {
		a = 1
	}
	if a >= 1 {
		return l, l / a
	}
	return l * a, l
}

// nativeLongEdge returns the long edge at which no picture is shown smaller
// than its original resolution (smaller pictures may be enlarged).
func (p *collagePage) nativeLongEdge() float64 {
	if p.root == nil {
		return float64(p.longEdge)
	}
	l := float64(defaultLongEdge)
	// The scale of each picture grows linearly with the long edge, apart from
	// the fixed gaps and margins, so a few iterations converge.
	for i := 0; i < 8; i++ {
		w, h := p.dims(l)
		m, g := float64(p.marginPx), float64(p.gapPx)
		placed, _ := collage.Layout(p.root, geom.Rect{X: m, Y: m, W: w - 2*m, H: h - 2*m}, g)
		minScale := math.Inf(1)
		for _, pl := range placed {
			it := p.items[pl.Cell.ID]
			if it == nil || pl.Rect.W <= 0 || pl.Rect.H <= 0 {
				continue
			}
			img := crop.Size{W: it.w, H: it.h}
			cell := crop.Size{W: pl.Rect.W, H: pl.Rect.H}
			s := render.CellView(pl.Cell.View, pl.Cell.Contain, img, cell).Scale(img, cell)
			minScale = math.Min(minScale, s)
		}
		if math.IsInf(minScale, 1) || minScale <= 0 {
			break
		}
		next := math.Min(l/minScale, 4*maxCollageSide)
		done := math.Abs(next-l) < 0.5
		l = next
		if done {
			break
		}
	}
	return math.Ceil(l)
}

func (p *collagePage) export() {
	if p.root == nil {
		p.a.setStatus("Add some pictures first.")
		return
	}
	w, h := p.outputSize()
	m, g := float64(p.marginPx), float64(p.gapPx)
	placed, _ := collage.Layout(p.root, geom.Rect{X: m, Y: m, W: float64(w) - 2*m, H: float64(h) - 2*m}, g)
	type job struct {
		path    string
		view    crop.View
		rect    geom.Rect
		contain bool
	}
	var jobs []job
	for _, pl := range placed {
		it := p.items[pl.Cell.ID]
		jobs = append(jobs, job{it.path, pl.Cell.View, pl.Rect, pl.Cell.Contain})
	}
	bg := p.bg.Value()
	f := imgio.PNG
	if p.jpeg {
		f = imgio.JPEG
		if bg.A == 0 {
			bg = color.NRGBA{255, 255, 255, 255}
		}
	}
	q := 50 + int(math.Round(float64(p.quality.Value)*50))
	p.a.saveFile("collage"+f.Ext(), func(file *os.File) error {
		out := render.Canvas(w, h, bg)
		for _, j := range jobs {
			img, err := imgio.Load(j.path)
			if err != nil {
				return err
			}
			render.DrawCell(out, render.Cell{Src: img, View: j.view, Rect: j.rect, Contain: j.contain})
		}
		bw := bufio.NewWriter(file)
		if err := imgio.Encode(bw, out, f, q); err != nil {
			return err
		}
		return bw.Flush()
	})
}

var collageRatios = []shape.Ratio{{W: 1, H: 1}, {W: 4, H: 3}, {W: 3, H: 2}, {W: 16, H: 9}, {W: 3, H: 4}, {W: 2, H: 3}, {W: 9, H: 16}}

func (p *collagePage) panel(gtx C) []layout.Widget {
	th := p.a.th
	p.handlePanelEvents(gtx)
	n := len(p.cells())

	labels := []string{"Auto"}
	sel := -1
	if p.autoAspect {
		sel = 0
	}
	for i, r := range collageRatios {
		labels = append(labels, r.String())
		if !p.autoAspect && math.Abs(r.Aspect()-p.ratio.Aspect()) < 1e-9 {
			sel = i + 1
		}
	}
	fitSel := 0
	if p.defContain {
		fitSel = 1
	}
	w, h := p.outputSize()
	fmtSel := 0
	if p.jpeg {
		fmtSel = 1
	}

	return []layout.Widget{
		func(gtx C) D {
			txt := "No pictures yet"
			if n > 0 {
				txt = fmt.Sprintf("%d pictures", n)
			}
			return section(gtx, th, "PICTURES",
				func(gtx C) D {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D { return p.addBtn.layout(gtx, th, icAdd, "Add pictures…", btnNormal, true) }),
						layout.Rigid(layout.Spacer{Width: 6}.Layout),
						layout.Rigid(func(gtx C) D {
							if n == 0 {
								gtx = gtx.Disabled()
							}
							return p.clearBtn.layout(gtx, th, icClear, "", btnNormal, false)
						}),
					)
				},
				hint(th, txt),
			)
		},
		func(gtx C) D {
			ws := []layout.Widget{
				func(gtx C) D {
					d, clicked := p.arrChoice.layout(gtx, th, []string{"Auto", "Row", "Column", "Grid"}, int(p.arrangement), 4)
					if clicked >= 0 {
						p.setArrangement(collage.Arrangement(clicked))
					}
					return d
				},
			}
			if p.arrangement == collage.ArrangeGrid {
				ws = append(ws, func(gtx C) D { return p.cols.layout(gtx, th, "Pictures per row", float64(p.gridCols), 0, "") })
			}
			ws = append(ws,
				func(gtx C) D { return boldLabel(th, 12, colMuted, "CANVAS").Layout(gtx) },
				func(gtx C) D {
					d, clicked := p.ratioChoice.layout(gtx, th, labels, sel, 4)
					if clicked == 0 {
						p.setAspect(true, p.ratio)
					} else if clicked > 0 {
						p.setAspect(false, collageRatios[clicked-1])
						p.ratioField.SetText(p.ratio.String())
					}
					return d
				},
				func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return p.ratioField.layout(gtx, th, "e.g. 3:2", "custom", p.ratioInvalid)
						}),
						layout.Rigid(layout.Spacer{Width: 6}.Layout),
						layout.Rigid(func(gtx C) D { return p.flipBtn.icon(gtx, icSwap, 18, colControl) }),
					)
				},
				func(gtx C) D {
					if n < 2 {
						gtx = gtx.Disabled()
					}
					return p.relayoutBtn.layout(gtx, th, icRelayout, "Re-arrange", btnNormal, true)
				},
				func(gtx C) D {
					d, clicked := p.fitChoice.layout(gtx, th, []string{"Fill cells", "Show whole"}, fitSel, 2)
					if clicked >= 0 {
						p.setContainAll(clicked == 1)
					}
					return d
				},
				hint(th, "Drag a picture by its handle onto another to swap them, or onto an edge to place it there. Drag inside a picture to choose the crop, use the wheel to zoom and right-drag to rotate (right-click turns a whole picture by 90°). Drag the gaps to resize."),
			)
			return section(gtx, th, "LAYOUT", ws...)
		},
		func(gtx C) D {
			return section(gtx, th, "SPACING",
				func(gtx C) D { return p.gap.layout(gtx, th, "Gap", float64(p.gapPx), 0, "px") },
				func(gtx C) D { return p.margin.layout(gtx, th, "Outer margin", float64(p.marginPx), 0, "px") },
				func(gtx C) D { return boldLabel(th, 12, colMuted, "BACKGROUND").Layout(gtx) },
				func(gtx C) D {
					d, _ := p.bg.layout(gtx, th)
					return d
				},
			)
		},
		func(gtx C) D {
			ws := []layout.Widget{
				func(gtx C) D {
					sel := 0
					if p.nativeSize {
						sel = 1
					}
					d, clicked := p.sizeChoice.layout(gtx, th, []string{"Long edge", "Original size"}, sel, 2)
					if clicked >= 0 {
						p.nativeSize = clicked == 1
					}
					return d
				},
				func(gtx C) D {
					if p.nativeSize {
						return hint(th, "Large enough that no picture is scaled down; smaller pictures are enlarged to match.")(gtx)
					}
					return p.longField.layout(gtx, th, "long edge", "px long edge", false)
				},
				func(gtx C) D {
					d, clicked := p.format.layout(gtx, th, []string{"PNG", "JPG"}, fmtSel, 2)
					if clicked >= 0 {
						p.jpeg = clicked == 1
					}
					return d
				},
			}
			if p.jpeg {
				ws = append(ws, func(gtx C) D {
					q := 50 + int(math.Round(float64(p.quality.Value)*50))
					d, _ := p.quality.layout(gtx, th, "JPG quality", strconv.Itoa(q))
					return d
				})
			}
			info := fmt.Sprintf("Result: %d × %d px", w, h)
			if _, _, limited := p.size(); limited {
				info += " · reduced to the maximum size"
			} else if float64(w)*float64(h) > 100e6 {
				info += " · very large, export needs a lot of memory"
			}
			ws = append(ws, hint(th, info))
			return section(gtx, th, "OUTPUT", ws...)
		},
		func(gtx C) D {
			return layout.UniformInset(16).Layout(gtx, func(gtx C) D {
				if n == 0 {
					gtx = gtx.Disabled()
				}
				return p.exportBtn.layout(gtx, th, icExport, "Export…  (Ctrl+E)", btnPrimary, true)
			})
		},
	}
}

func (p *collagePage) handlePanelEvents(gtx C) {
	if p.addBtn.Clicked(gtx) || p.emptyAddBtn.Clicked(gtx) {
		p.open()
	}
	if p.clearBtn.Clicked(gtx) && p.root != nil {
		p.change("")
		p.root = nil
		p.hover = nil
	}
	if p.relayoutBtn.Clicked(gtx) && p.root != nil {
		p.change("")
		p.arrange(p.cells())
	}
	if p.exportBtn.Clicked(gtx) {
		p.export()
	}
	if p.flipBtn.Clicked(gtx) {
		r := p.ratio
		if p.autoAspect && p.aspect > 0 {
			r = shape.Ratio{W: 1, H: math.Round(p.aspect*1000) / 1000}
		} else {
			r = shape.Ratio{W: r.H, H: r.W}
		}
		p.setAspect(false, r)
		p.ratioField.SetText(p.ratio.String())
	}
	if p.ratioField.edited(gtx) {
		r, err := shape.ParseRatio(p.ratioField.Text())
		p.ratioInvalid = err != nil
		if err == nil && (p.autoAspect || math.Abs(r.Aspect()-p.ratio.Aspect()) > 1e-9) {
			p.setAspect(false, r)
		}
	}
	if p.longField.edited(gtx) {
		if v, err := strconv.Atoi(p.longField.Text()); err == nil && v > 0 {
			p.longEdge = min(v, maxCollageSide)
		}
	}
	if !gtx.Focused(&p.longField.Editor) && p.longField.Text() != strconv.Itoa(p.longEdge) {
		p.longField.SetText(strconv.Itoa(p.longEdge))
	}
	p.longField.Filter, p.longField.MaxLen = "0123456789", 5
	p.gapPx = spacingInput(gtx, &p.gap, p.gapPx, maxGap)
	p.marginPx = spacingInput(gtx, &p.margin, p.marginPx, maxMargin)
	cols := p.gridCols
	if p.cols.Update(gtx) {
		cols = 1 + int(math.Round(float64(p.cols.Value)*(maxGridCols-1)))
	} else if v, ok := p.cols.typed(gtx); ok {
		cols = min(max(int(math.Round(v)), 1), 100)
	}
	if cols != p.gridCols {
		p.change("cols")
		p.gridCols = cols
		if p.root != nil {
			p.arrange(p.cells())
		}
	}
	if !p.cols.Dragging() {
		p.cols.Value = float32(min(p.gridCols-1, maxGridCols-1)) / (maxGridCols - 1)
	}
}

// spacingInput applies slider and typed changes to a pixel value and keeps
// the slider in sync. Typed values may exceed the slider range.
func spacingInput(gtx C, s *numSlider, v, sliderMax int) int {
	if s.Update(gtx) {
		v = int(math.Round(float64(s.Value) * float64(sliderMax)))
	} else if t, ok := s.typed(gtx); ok {
		v = min(max(int(math.Round(t)), 0), maxSpacing)
	}
	if !s.Dragging() {
		s.Value = float32(math.Min(float64(v)/float64(sliderMax), 1))
	}
	return v
}

// setArrangement switches the arrangement and lays out the pictures again.
func (p *collagePage) setArrangement(a collage.Arrangement) {
	if a == p.arrangement {
		return
	}
	p.change("")
	p.arrangement = a
	if p.root != nil {
		p.arrange(p.cells())
	}
}

// setAspect switches between an automatic and a fixed canvas aspect ratio
// and rearranges the pictures for it.
func (p *collagePage) setAspect(auto bool, r shape.Ratio) {
	if auto == p.autoAspect && (auto || math.Abs(r.Aspect()-p.ratio.Aspect()) < 1e-9) {
		return
	}
	p.change("aspect")
	p.autoAspect, p.ratio = auto, r
	if p.root != nil {
		p.arrange(p.cells())
	}
}

func (p *collagePage) setContainAll(contain bool) {
	p.defContain = contain
	if p.root == nil {
		return
	}
	p.change("")
	for _, c := range p.cells() {
		setContain(c, contain)
	}
}

func (p *collagePage) buttons(c *collage.Cell) *cellButtons {
	b, ok := p.cellBtns[c.ID]
	if !ok {
		b = &cellButtons{}
		p.cellBtns[c.ID] = b
	}
	return b
}

// cellAt returns the cell at canvas position pos.
func (p *collagePage) cellAt(pos geom.Pt) (*collage.Cell, geom.Rect) {
	for _, pl := range p.placed {
		if pl.Rect.Contains(pos) {
			return pl.Cell, pl.Rect
		}
	}
	return nil, geom.Rect{}
}

func (p *collagePage) cellRect(c *collage.Cell) geom.Rect {
	for _, pl := range p.placed {
		if pl.Cell == c {
			return pl.Rect
		}
	}
	return geom.Rect{}
}

// dividerAt returns the index of the divider near pos, or -1.
func (p *collagePage) dividerAt(gtx C, pos geom.Pt) int {
	tol := float64(gtx.Dp(5))
	for i, d := range p.divs {
		r := d.Rect
		if d.Node.Dir == collage.Row {
			r = geom.Rect{X: r.X + r.W/2 - tol, Y: r.Y, W: 2 * tol, H: r.H}
			if d.Rect.W > 2*tol {
				r = d.Rect
			}
		} else {
			r = geom.Rect{X: r.X, Y: r.Y + r.H/2 - tol, W: r.W, H: 2 * tol}
			if d.Rect.H > 2*tol {
				r = d.Rect
			}
		}
		if r.Contains(pos) {
			return i
		}
	}
	return -1
}

// handleRect is the move handle in a cell's top-left corner.
func handleRect(gtx C, r geom.Rect) geom.Rect {
	s := float64(gtx.Dp(30))
	in := float64(gtx.Dp(6))
	return geom.Rect{X: r.X + in, Y: r.Y + in, W: s, H: s}
}

// dropAt determines where a dragged picture would be dropped.
func (p *collagePage) dropAt(pos geom.Pt) (*collage.Cell, collage.Edge) {
	c, r := p.cellAt(pos)
	if c == nil || c == p.dragCell {
		return nil, collage.Center
	}
	u, v := (pos.X-r.X)/r.W, (pos.Y-r.Y)/r.H
	if u > 0.28 && u < 0.72 && v > 0.28 && v < 0.72 {
		return c, collage.Center
	}
	d := map[collage.Edge]float64{collage.Left: u, collage.Right: 1 - u, collage.Top: v, collage.Bottom: 1 - v}
	best := collage.Left
	for _, e := range []collage.Edge{collage.Right, collage.Top, collage.Bottom} {
		if d[e] < d[best] {
			best = e
		}
	}
	return c, best
}

func (p *collagePage) itemSize(c *collage.Cell) crop.Size {
	it := p.items[c.ID]
	return crop.Size{W: it.w, H: it.h}
}

func (p *collagePage) handleCanvasInput(gtx C) {
	for {
		ev, ok := gtx.Event(
			pointer.Filter{
				Target:  &p.tag,
				Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Move | pointer.Leave | pointer.Scroll,
				ScrollY: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32},
			},
			key.Filter{Focus: &p.tag, Name: key.NameDeleteForward},
			key.Filter{Focus: &p.tag, Name: key.NameDeleteBackward},
		)
		if !ok {
			break
		}
		// The panel was laid out before this event; draw another frame.
		gtx.Execute(op.InvalidateCmd{})
		switch e := ev.(type) {
		case key.Event:
			if e.State == key.Press && p.hover != nil {
				p.removeCell(p.hover)
			}
		case pointer.Event:
			p.pointerEvent(gtx, e)
		}
	}
}

func (p *collagePage) pointerEvent(gtx C, e pointer.Event) {
	pos := gpt(e.Position)
	p.pointer = e.Position
	switch e.Kind {
	case pointer.Move:
		p.hover, _ = p.cellAt(pos)
		p.hoverDiv = p.dividerAt(gtx, pos)
	case pointer.Leave:
		if !p.board.Contains(pos) {
			p.hover, p.hoverDiv = nil, -1
		}
	case pointer.Press:
		if p.drag != dragNone {
			return
		}
		if e.Buttons.Contain(pointer.ButtonSecondary) {
			p.rotatePress(gtx, pos)
			return
		}
		if !e.Buttons.Contain(pointer.ButtonPrimary) {
			return
		}
		gtx.Execute(key.FocusCmd{Tag: &p.tag})
		p.lastPos = e.Position
		if i := p.dividerAt(gtx, pos); i >= 0 {
			p.change("")
			p.drag, p.dragDivRef = dragDivider, p.divs[i]
			return
		}
		c, r := p.cellAt(pos)
		if c == nil {
			return
		}
		p.hover = c
		if handleRect(gtx, r).Contains(pos) || c.Contain {
			p.drag, p.dragCell = dragMove, c
			p.dropCell = nil
			return
		}
		p.change("")
		p.drag, p.dragCell = dragPan, c
	case pointer.Drag:
		d := e.Position.Sub(p.lastPos)
		p.lastPos = e.Position
		switch p.drag {
		case dragPan:
			r := p.cellRect(p.dragCell)
			cell := crop.Size{W: r.W, H: r.H}
			img := p.itemSize(p.dragCell)
			v := p.dragCell.View.ClampCover(img, cell)
			p.dragCell.View = v.Pan(float64(d.X), float64(d.Y), img, cell).ClampCover(img, cell)
		case dragDivider:
			for _, dv := range p.divs {
				if dv.Node == p.dragDivRef.Node && dv.Index == p.dragDivRef.Index {
					delta := float64(d.X)
					if dv.Node.Dir == collage.Column {
						delta = float64(d.Y)
					}
					collage.MoveDivider(dv, delta, float64(gtx.Dp(40)))
				}
			}
		case dragMove:
			p.dropCell, p.dropEdge = p.dropAt(pos)
		case dragRotate:
			a := angleDeg(pos, p.cellRect(p.dragCell).Center())
			p.rotAccum += crop.NormDeg(a - p.lastAngle)
			p.lastAngle = a
			target := p.rotAccum
			if e.Modifiers.Contain(key.ModShift) {
				target = math.Round(target/15) * 15
			}
			p.dragCell.View.Rotation = crop.NormDeg(target)
		}
	case pointer.Release, pointer.Cancel:
		if p.drag == dragMove && e.Kind == pointer.Release {
			p.dropCell, p.dropEdge = p.dropAt(pos)
		}
		if p.drag == dragMove && e.Kind == pointer.Release && p.dropCell != nil {
			p.change("")
			p.root = collage.Move(p.root, p.dragCell, p.dropCell, p.dropEdge)
			p.refit()
		}
		// A rotation that turned the picture between landscape and portrait
		// changes the layout, like the rotate button.
		if p.drag == dragRotate && p.dragCell.EffAspect() != p.rotAspect {
			p.refit()
		}
		p.drag, p.dragCell, p.dropCell = dragNone, nil, nil
		p.hover, _ = p.cellAt(pos)
	case pointer.Scroll:
		c, r := p.cellAt(pos)
		if c == nil || c.Contain {
			return
		}
		p.change("zoom")
		cell := crop.Size{W: r.W, H: r.H}
		img := p.itemSize(c)
		steps := float64(e.Scroll.Y) / 120
		v := c.View.ClampCover(img, cell)
		c.View = v.ZoomAt(pos.Sub(r.Min()), math.Pow(1.15, -steps), img, cell).ClampCover(img, cell)
	}
}

// rotatePress handles a right-click on a picture: in cover mode it starts a
// free rotation, in contain mode it turns the picture by 90°.
func (p *collagePage) rotatePress(gtx C, pos geom.Pt) {
	c, r := p.cellAt(pos)
	if c == nil {
		return
	}
	gtx.Execute(key.FocusCmd{Tag: &p.tag})
	p.hover = c
	if c.Contain {
		p.rotateCell(c)
		return
	}
	p.change("")
	c.View = c.View.ClampCover(p.itemSize(c), crop.Size{W: r.W, H: r.H})
	p.drag, p.dragCell = dragRotate, c
	p.rotAccum, p.lastAngle, p.rotAspect = c.View.Rotation, angleDeg(pos, r.Center()), c.EffAspect()
}

func (p *collagePage) removeCell(c *collage.Cell) {
	p.change("")
	p.root = collage.Remove(p.root, c)
	p.refit()
	p.hover = nil
}

func (p *collagePage) rotateCell(c *collage.Cell) {
	p.change("")
	c.View.Rotation = crop.NormDeg(c.View.Rotation + 90)
	c.View.CX, c.View.CY = 0.5, 0.5
	p.refit()
}

func (p *collagePage) canvas(gtx C) D {
	size := gtx.Constraints.Max
	if p.root == nil {
		p.placed, p.divs = nil, nil
		return emptyState(gtx, p.a.th, &p.emptyAddBtn, icCollage, "Add pictures to build a collage", "Add pictures…", "or press Ctrl+O · select several files at once")
	}
	// Toolbar buttons of the hovered cell (handled before the layout so a
	// removed cell is not drawn).
	if c := p.hover; c != nil && collage.Contains(p.root, c) {
		b := p.buttons(c)
		switch {
		case b.remove.Clicked(gtx):
			p.removeCell(c)
		case b.rotate.Clicked(gtx):
			p.rotateCell(c)
		case b.fit.Clicked(gtx):
			p.change("")
			setContain(c, !c.Contain)
		}
	}
	if p.root == nil {
		return D{Size: size}
	}

	w, h := p.outputSize()
	pad := float64(gtx.Dp(32))
	area := geom.Rect{W: float64(size.X), H: float64(size.Y)}.Inset(pad)
	p.board = geom.FitRect(area, float64(w)/float64(h))
	p.scale = p.board.W / float64(w)
	m, g := float64(p.marginPx)*p.scale, float64(p.gapPx)*p.scale
	p.placed, p.divs = collage.Layout(p.root, p.board.Inset(m), g)
	p.handleCanvasInput(gtx)
	if p.root == nil {
		return D{Size: size}
	}
	// Input may have changed the layout.
	p.placed, p.divs = collage.Layout(p.root, p.board.Inset(m), g)
	if p.hover != nil && !collage.Contains(p.root, p.hover) {
		p.hover = nil
	}

	// Background.
	if bg := p.bg.Value(); bg.A == 0 && !p.jpeg {
		checkerboard(gtx, pxRect(p.board))
	} else {
		if bg.A == 0 {
			bg = rgb(0xffffff)
		}
		fillRect(gtx.Ops, pxRect(p.board), bg)
	}

	// Pictures.
	for _, pl := range p.placed {
		p.drawCell(gtx, pl)
	}
	if p.drag == dragRotate {
		r := p.cellRect(p.dragCell)
		cl := clip.Rect(pxRect(r)).Push(gtx.Ops)
		thirds(gtx, r)
		cl.Pop()
	}

	// Divider highlight.
	if i := p.hoverDiv; i >= 0 && i < len(p.divs) && p.drag == dragNone || p.drag == dragDivider {
		for j, d := range p.divs {
			if (p.drag == dragDivider && d.Node == p.dragDivRef.Node && d.Index == p.dragDivRef.Index) || (p.drag == dragNone && j == p.hoverDiv) {
				r := d.Rect
				t := float64(gtx.Dp(3))
				if d.Node.Dir == collage.Row {
					r = geom.Rect{X: r.X + r.W/2 - t/2, Y: r.Y, W: t, H: r.H}
				} else {
					r = geom.Rect{X: r.X, Y: r.Y + r.H/2 - t/2, W: r.W, H: t}
				}
				fillRect(gtx.Ops, pxRect(r), colAccent)
			}
		}
	}

	// Drop preview.
	if p.drag == dragMove && p.dropCell != nil {
		r := p.cellRect(p.dropCell)
		switch p.dropEdge {
		case collage.Left:
			r.W /= 2
		case collage.Right:
			r.X += r.W / 2
			r.W /= 2
		case collage.Top:
			r.H /= 2
		case collage.Bottom:
			r.Y += r.H / 2
			r.H /= 2
		}
		fillRect(gtx.Ops, pxRect(r), withAlpha(colAccent, 0x70))
		paint.FillShape(gtx.Ops, colAccent, clip.Stroke{Path: clip.Rect(pxRect(r)).Path(), Width: float32(gtx.Dp(2))}.Op())
		if p.dropEdge == collage.Center {
			s := gtx.Dp(36)
			c := pxRect(r)
			ctr := c.Min.Add(c.Size().Div(2))
			st := op.Offset(ctr.Sub(image.Pt(s/2, s/2))).Push(gtx.Ops)
			gtx := gtx
			gtx.Constraints = layout.Exact(image.Pt(s, s))
			icSwap.Layout(gtx, rgb(0xffffff))
			st.Pop()
		}
	}

	// Input area (below the cell toolbars).
	ar := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.tag)
	p.cursor(gtx).Add(gtx.Ops)
	ar.Pop()

	// Hover decorations and toolbar.
	if c := p.hover; c != nil && p.drag != dragMove && p.drag != dragDivider {
		p.cellOverlay(gtx, c, p.cellRect(c))
	}

	// Ghost of the dragged picture.
	if p.drag == dragMove && p.dragCell != nil {
		s := float64(gtx.Dp(90))
		a := p.dragCell.EffAspect()
		gr := geom.FitRect(geom.Rect{X: float64(p.pointer.X) + 12, Y: float64(p.pointer.Y) + 12, W: s, H: s}, a)
		gr.X, gr.Y = float64(p.pointer.X)+12, float64(p.pointer.Y)+12
		opac := paint.PushOpacity(gtx.Ops, 0.85)
		p.drawPicture(gtx, p.dragCell, gr, true)
		opac.Pop()
		paint.FillShape(gtx.Ops, rgb(0xffffff), clip.Stroke{Path: clip.Rect(pxRect(gr)).Path(), Width: float32(gtx.Dp(2))}.Op())
	}
	return D{Size: size}
}

func (p *collagePage) cursor(gtx C) pointer.Cursor {
	switch p.drag {
	case dragMove, dragPan:
		return pointer.CursorGrabbing
	case dragRotate:
		return pointer.CursorCrosshair
	case dragDivider:
		if p.dragDivRef.Node.Dir == collage.Row {
			return pointer.CursorColResize
		}
		return pointer.CursorRowResize
	}
	if i := p.hoverDiv; i >= 0 && i < len(p.divs) {
		if p.divs[i].Node.Dir == collage.Row {
			return pointer.CursorColResize
		}
		return pointer.CursorRowResize
	}
	pos := gpt(p.pointer)
	if c, r := p.cellAt(pos); c != nil {
		if handleRect(gtx, r).Contains(pos) || c.Contain {
			return pointer.CursorGrab
		}
		return pointer.CursorAllScroll
	}
	return pointer.CursorDefault
}

// drawPicture draws the picture of c into r. contain forces the whole image.
func (p *collagePage) drawPicture(gtx C, c *collage.Cell, r geom.Rect, contain bool) {
	it := p.items[c.ID]
	if it == nil || r.Empty() {
		return
	}
	cell := crop.Size{W: r.W, H: r.H}
	img := crop.Size{W: it.w, H: it.h}
	v := render.CellView(c.View, contain || c.Contain, img, cell)
	a := v.ImageToFrame(img, cell).Then(geom.Translate(r.X, r.Y))
	cl := clip.Rect(pxRect(r)).Push(gtx.Ops)
	drawImage(gtx.Ops, it.preview, it.w, it.h, a)
	cl.Pop()
}

func (p *collagePage) drawCell(gtx C, pl collage.Placed) {
	if p.drag == dragMove && pl.Cell == p.dragCell {
		opac := paint.PushOpacity(gtx.Ops, 0.25)
		p.drawPicture(gtx, pl.Cell, pl.Rect, false)
		opac.Pop()
		return
	}
	p.drawPicture(gtx, pl.Cell, pl.Rect, false)
}

// cellOverlay draws the hover frame, move handle and toolbar of a cell.
func (p *collagePage) cellOverlay(gtx C, c *collage.Cell, r geom.Rect) {
	if r.Empty() {
		return
	}
	pr := pxRect(r)
	paint.FillShape(gtx.Ops, colAccent, clip.Stroke{Path: clip.Rect(pr).Path(), Width: float32(gtx.Dp(2))}.Op())

	// Move handle.
	hr := pxRect(handleRect(gtx, r))
	if hr.Max.X < pr.Max.X && hr.Max.Y < pr.Max.Y {
		paint.FillShape(gtx.Ops, withAlpha(rgb(0x000000), 0xb0), clip.Ellipse(hr).Op(gtx.Ops))
		in := gtx.Dp(6)
		st := op.Offset(hr.Min.Add(image.Pt(in, in))).Push(gtx.Ops)
		gtx := gtx
		gtx.Constraints = layout.Exact(hr.Size().Sub(image.Pt(2*in, 2*in)))
		icMove.Layout(gtx, rgb(0xffffff))
		st.Pop()
	}

	// Toolbar in the top-right corner.
	b := p.buttons(c)
	fitIcon := icFit
	if c.Contain {
		fitIcon = icFill
	}
	m := op.Record(gtx.Ops)
	gtx.Constraints.Min = image.Point{}
	dims := layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return b.fit.icon(gtx, fitIcon, 16, withAlpha(rgb(0x000000), 0xb0)) }),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx C) D { return b.rotate.icon(gtx, icRotRight, 16, withAlpha(rgb(0x000000), 0xb0)) }),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx C) D { return b.remove.icon(gtx, icClose, 16, withAlpha(rgb(0x000000), 0xb0)) }),
	)
	call := m.Stop()
	in := gtx.Dp(6)
	pos := image.Pt(pr.Max.X-in-dims.Size.X, pr.Min.Y+in)
	if pos.X > hr.Max.X+in && dims.Size.Y+2*in < pr.Dy() {
		st := op.Offset(pos).Push(gtx.Ops)
		call.Add(gtx.Ops)
		st.Pop()
	}
}

// setContain switches a picture between cover and contain mode. A whole
// picture is only shown in quarter turns, so a free rotation is rounded.
func setContain(c *collage.Cell, contain bool) {
	c.Contain = contain
	if contain {
		c.View.Rotation = crop.NormDeg(math.Round(c.View.Rotation/90) * 90)
	}
}
