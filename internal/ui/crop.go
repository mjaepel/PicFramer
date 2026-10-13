package ui

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/mjaepel/picframer/internal/crop"
	"github.com/mjaepel/picframer/internal/geom"
	"github.com/mjaepel/picframer/internal/imgio"
	"github.com/mjaepel/picframer/internal/render"
	"github.com/mjaepel/picframer/internal/shape"
)

// Zoom slider range (relative to "cover").
const (
	sliderZoomMin = 0.1
	sliderZoomMax = 10.0
	maxOutputSide = 16384
	previewSize   = 2048
)

// cropState is the undoable state of the crop mode.
type cropState struct {
	spec        shape.Spec
	view        crop.View
	useOriginal bool // ratio follows the image's aspect ratio
}

type cropPage struct {
	a *App

	path    string
	src     image.Image
	srcW    float64
	srcH    float64
	preview paint.ImageOp

	st   cropState
	hist history[cropState]
	// Last used spec per shape, so switching shapes keeps their settings.
	specs map[shape.Kind]shape.Spec

	openBtn, emptyOpenBtn, exportBtn button
	shapeBtns                        [7]button
	apex                             choice
	corner, sides, inner             slider
	ratio                            choice
	ratioField                       field
	ratioInvalid                     bool
	flipBtn                          button
	zoom, rot                        numSlider
	rotL, rotR, fillBtn, fitBtn      button
	native                           widget.Bool
	wField, hField                   field
	outW                             int
	format                           choice
	jpeg                             bool
	quality                          slider
	bg                               *bgPicker

	tag      struct{}
	dragging bool // left button: move
	rotating bool // right button: rotate
	lastPos  f32.Point
	// Rotation by right-dragging: unsnapped target angle and last pointer angle.
	rotAccum, lastAngle float64
	frame               geom.Rect
}

var shapeKinds = shape.Kinds

func newCropPage(a *App) *cropPage {
	p := &cropPage{
		a:     a,
		specs: map[shape.Kind]shape.Spec{},
		bg:    newBgPicker(true, rgb(0xffffff)),
		outW:  1200,
	}
	p.st.spec = shape.Default(shape.Rect)
	p.st.view = crop.Default()
	p.native.Value = true
	p.quality.Value = (92 - 50) / 50.0
	p.ratioField.SetText(p.st.spec.Ratio.String())
	return p
}

func (p *cropPage) imgSize() crop.Size   { return crop.Size{W: p.srcW, H: p.srcH} }
func (p *cropPage) frameSize() crop.Size { return crop.Size{W: p.frame.W, H: p.frame.H} }

func (p *cropPage) canUndo() bool { return p.hist.canUndo() }
func (p *cropPage) canRedo() bool { return p.hist.canRedo() }

func (p *cropPage) undo() {
	if s, ok := p.hist.doUndo(p.st); ok {
		p.st = s
		p.syncRatioField()
	}
}

func (p *cropPage) redo() {
	if s, ok := p.hist.doRedo(p.st); ok {
		p.st = s
		p.syncRatioField()
	}
}

// change records the current state for undo before a modification.
func (p *cropPage) change(key string) { p.hist.push(p.st, key) }

func (p *cropPage) open() {
	p.a.chooseImages(false, func(paths []string) { p.load(paths[0]) })
}

func (p *cropPage) load(path string) {
	p.a.setStatus("Loading " + filepath.Base(path) + " …")
	go func() {
		img, err := imgio.Load(path)
		if err != nil {
			p.a.post(func() { p.a.setError(err) })
			return
		}
		prev := paint.NewImageOp(imgio.Thumbnail(img, previewSize))
		b := img.Bounds()
		p.a.post(func() {
			p.path, p.src, p.preview = path, img, prev
			p.srcW, p.srcH = float64(b.Dx()), float64(b.Dy())
			p.st.view = crop.Default()
			if p.st.useOriginal {
				p.st.spec.Ratio = shape.Ratio{W: p.srcW, H: p.srcH}
			}
			p.hist.clear()
			p.a.setStatus(fmt.Sprintf("%s · %d × %d px", filepath.Base(path), b.Dx(), b.Dy()))
		})
	}()
}

// outputSize returns the export size in pixels.
func (p *cropPage) outputSize() (int, int) {
	aspect := p.st.spec.Aspect()
	var w float64
	if p.native.Value && p.src != nil {
		w = p.st.view.NativeSize(p.imgSize(), aspect).W
	} else {
		w = float64(p.outW)
	}
	h := w / aspect
	if w > maxOutputSide {
		w, h = maxOutputSide, maxOutputSide/aspect
	}
	if h > maxOutputSide {
		w, h = maxOutputSide*aspect, maxOutputSide
	}
	return max(1, int(math.Round(w))), max(1, int(math.Round(h)))
}

func (p *cropPage) export() {
	if p.src == nil {
		p.a.setStatus("Open an image first.")
		return
	}
	src, st := p.src, p.st
	w, h := p.outputSize()
	bg := p.bg.Value()
	f := imgio.PNG
	if p.jpeg {
		f = imgio.JPEG
		if bg.A == 0 {
			bg = color.NRGBA{255, 255, 255, 255}
		}
	}
	q := p.qualityValue()
	name := baseName(p.path) + "_" + string(st.spec.Kind) + f.Ext()
	p.a.saveFile(name, func(file *os.File) error {
		img := render.Crop(src, st.view, st.spec, w, h, bg)
		bw := bufio.NewWriter(file)
		if err := imgio.Encode(bw, img, f, q); err != nil {
			return err
		}
		return bw.Flush()
	})
}

func (p *cropPage) qualityValue() int { return 50 + int(math.Round(float64(p.quality.Value)*50)) }

// setShape switches to kind k, restoring its previous settings.
func (p *cropPage) setShape(k shape.Kind) {
	if k == p.st.spec.Kind {
		return
	}
	p.change("")
	p.specs[p.st.spec.Kind] = p.st.spec
	sp, ok := p.specs[k]
	if !ok {
		sp = shape.Default(k)
		// Keep the current rectangle-like ratio when switching between them.
		if !sp.HasNatural() && !sp.RatioFixed() && !p.st.spec.HasNatural() && p.st.spec.Ratio.Valid() {
			sp.Ratio = p.st.spec.Ratio
		}
	}
	p.st.spec = sp
	p.syncRatioField()
}

func (p *cropPage) syncRatioField() {
	r := p.st.spec.Ratio
	if !r.Valid() {
		r = shape.Ratio{W: math.Round(p.st.spec.Aspect()*1000) / 1000, H: 1}
	}
	p.ratioField.SetText(r.String())
	p.ratioInvalid = false
}

// ratioOptions returns the labels of the ratio buttons and the index of the
// selected one (-1 if a custom ratio is active).
func (p *cropPage) ratioOptions() ([]string, []shape.Ratio, int) {
	var labels []string
	var ratios []shape.Ratio
	sel := -1
	sp := p.st.spec
	if sp.HasNatural() {
		labels = append(labels, "Regular")
		ratios = append(ratios, shape.Ratio{})
		if !sp.Ratio.Valid() {
			sel = 0
		}
	}
	labels = append(labels, "Original")
	ratios = append(ratios, shape.Ratio{W: -1})
	if p.st.useOriginal {
		sel = len(labels) - 1
	}
	for _, r := range shape.Presets {
		labels = append(labels, r.String())
		ratios = append(ratios, r)
		if sel < 0 && sp.Ratio.Valid() && math.Abs(sp.Ratio.Aspect()-r.Aspect()) < 1e-9 {
			sel = len(labels) - 1
		}
	}
	return labels, ratios, sel
}

func (p *cropPage) setRatio(r shape.Ratio, original bool) {
	p.change("")
	p.st.useOriginal = original
	p.st.spec.Ratio = r
	p.syncRatioField()
}

func (p *cropPage) panel(gtx C) []layout.Widget {
	th := p.a.th
	p.handlePanelEvents(gtx)

	items := []layout.Widget{
		func(gtx C) D {
			txt := "No image loaded"
			if p.src != nil {
				txt = fmt.Sprintf("%s\n%d × %d px", filepath.Base(p.path), int(p.srcW), int(p.srcH))
			}
			return section(gtx, th, "IMAGE",
				func(gtx C) D { return p.openBtn.layout(gtx, th, icOpen, "Open image…", btnNormal, true) },
				hint(th, txt),
			)
		},
		func(gtx C) D {
			ws := []layout.Widget{func(gtx C) D { return p.shapeGrid(gtx) }}
			ws = append(ws, p.shapeParams(gtx)...)
			return section(gtx, th, "SHAPE", ws...)
		},
	}
	if !p.st.spec.RatioFixed() {
		items = append(items, func(gtx C) D {
			labels, _, sel := p.ratioOptions()
			return section(gtx, th, "ASPECT RATIO",
				func(gtx C) D {
					d, _ := p.ratio.layout(gtx, th, labels, sel, 4)
					return d
				},
				func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return p.ratioField.layout(gtx, th, "e.g. 5:4", "custom", p.ratioInvalid)
						}),
						layout.Rigid(layout.Spacer{Width: 6}.Layout),
						layout.Rigid(func(gtx C) D { return p.flipBtn.icon(gtx, icSwap, 18, colControl) }),
					)
				},
			)
		})
	}
	items = append(items,
		func(gtx C) D {
			return section(gtx, th, "POSITION",
				func(gtx C) D { return p.zoom.layout(gtx, th, "Zoom", p.st.view.Zoom*100, 0, "%") },
				func(gtx C) D { return p.rot.layout(gtx, th, "Rotation", p.st.view.Rotation, 1, "°") },
				func(gtx C) D {
					return grid(gtx, 4, 4, 6, func(gtx C, i int) D {
						switch i {
						case 0:
							return p.rotL.layout(gtx, th, icRotLeft, "", btnNormal, true)
						case 1:
							return p.rotR.layout(gtx, th, icRotRight, "", btnNormal, true)
						case 2:
							return p.fillBtn.layout(gtx, th, nil, "Fill", btnNormal, true)
						default:
							return p.fitBtn.layout(gtx, th, nil, "Fit", btnNormal, true)
						}
					})
				},
				hint(th, "Left-drag to move · right-drag to rotate (Shift snaps to 15°) · wheel to zoom · arrow keys to nudge"),
			)
		},
		func(gtx C) D { return p.outputSection(gtx) },
		func(gtx C) D {
			return layout.UniformInset(16).Layout(gtx, func(gtx C) D {
				if p.src == nil {
					gtx = gtx.Disabled()
				}
				return p.exportBtn.layout(gtx, th, icExport, "Export…  (Ctrl+E)", btnPrimary, true)
			})
		},
	)
	return items
}

func (p *cropPage) handlePanelEvents(gtx C) {
	if p.openBtn.Clicked(gtx) || p.emptyOpenBtn.Clicked(gtx) {
		p.open()
	}
	if p.exportBtn.Clicked(gtx) {
		p.export()
	}
	for i := range p.shapeBtns {
		if p.shapeBtns[i].Clicked(gtx) {
			p.setShape(shapeKinds[i])
		}
	}
	// Aspect ratio buttons.
	labels, ratios, _ := p.ratioOptions()
	if len(p.ratio.btns) == len(labels) {
		for i := range p.ratio.btns {
			if !p.ratio.btns[i].Clicked(gtx) {
				continue
			}
			switch r := ratios[i]; {
			case r.W < 0: // original
				if p.src != nil {
					p.setRatio(shape.Ratio{W: p.srcW, H: p.srcH}, true)
				}
			default:
				p.setRatio(r, false)
			}
		}
	}
	if p.ratioField.edited(gtx) {
		r, err := shape.ParseRatio(p.ratioField.Text())
		p.ratioInvalid = err != nil
		if err == nil && (!p.st.spec.Ratio.Valid() || math.Abs(r.Aspect()-p.st.spec.Aspect()) > 1e-9 || p.st.useOriginal) {
			p.change("ratio-field")
			p.st.spec.Ratio = r
			p.st.useOriginal = false
		}
	}
	if p.flipBtn.Clicked(gtx) {
		p.change("")
		a := p.st.spec.Aspect()
		r := p.st.spec.Ratio
		if r.Valid() {
			r = shape.Ratio{W: r.H, H: r.W}
		} else {
			r = shape.Ratio{W: 1, H: math.Round(a*1000) / 1000}
		}
		p.st.spec.Ratio = r
		p.st.useOriginal = false
		p.syncRatioField()
	}

	// Position.
	img, fr := p.imgSize(), p.frameSize()
	ready := p.src != nil && fr.W > 0
	if p.zoom.Update(gtx) && ready {
		p.change("zoom")
		z := sliderZoomMin * math.Pow(sliderZoomMax/sliderZoomMin, float64(p.zoom.Value))
		p.st.view = p.st.view.SetZoom(z)
	} else if v, ok := p.zoom.typed(gtx); ok && ready {
		p.change("zoom-field")
		p.st.view = p.st.view.SetZoom(v / 100)
	} else if !p.zoom.Dragging() {
		t := math.Log(p.st.view.Zoom/sliderZoomMin) / math.Log(sliderZoomMax/sliderZoomMin)
		p.zoom.Value = float32(math.Min(math.Max(t, 0), 1))
	}
	if p.rot.Update(gtx) && ready {
		p.change("rotate")
		deg := math.Round((float64(p.rot.Value)*360-180)*2) / 2
		p.st.view.Rotation = crop.NormDeg(deg)
	} else if v, ok := p.rot.typed(gtx); ok && ready {
		p.change("rotate-field")
		p.st.view.Rotation = crop.NormDeg(v)
	} else if !p.rot.Dragging() {
		p.rot.Value = float32((p.st.view.Rotation + 180) / 360)
	}
	if ready {
		if p.rotL.Clicked(gtx) {
			p.change("")
			p.st.view = p.st.view.RotateAt(geom.Pt{X: fr.W / 2, Y: fr.H / 2}, -90, img, fr)
		}
		if p.rotR.Clicked(gtx) {
			p.change("")
			p.st.view = p.st.view.RotateAt(geom.Pt{X: fr.W / 2, Y: fr.H / 2}, 90, img, fr)
		}
		if p.fillBtn.Clicked(gtx) {
			p.change("")
			p.st.view = p.st.view.Fill(img, fr)
		}
		if p.fitBtn.Clicked(gtx) {
			p.change("")
			p.st.view = p.st.view.Fit(img, fr)
		}
	}

	// Output size.
	if p.wField.edited(gtx) {
		if v, err := strconv.Atoi(p.wField.Text()); err == nil && v > 0 {
			p.outW = min(v, maxOutputSide)
			p.native.Value = false
		}
	}
	if p.hField.edited(gtx) {
		if v, err := strconv.Atoi(p.hField.Text()); err == nil && v > 0 {
			p.outW = min(int(math.Round(float64(v)*p.st.spec.Aspect())), maxOutputSide)
			p.native.Value = false
		}
	}
	p.native.Update(gtx)
	w, h := p.outputSize()
	if !gtx.Focused(&p.wField.Editor) && p.wField.Text() != strconv.Itoa(w) {
		p.wField.SetText(strconv.Itoa(w))
	}
	if !gtx.Focused(&p.hField.Editor) && p.hField.Text() != strconv.Itoa(h) {
		p.hField.SetText(strconv.Itoa(h))
	}
	p.wField.Filter, p.hField.Filter = "0123456789", "0123456789"
	p.wField.MaxLen, p.hField.MaxLen = 5, 5
}

func (p *cropPage) shapeGrid(gtx C) D {
	th := p.a.th
	return grid(gtx, len(shapeKinds), 4, 6, func(gtx C, i int) D {
		k := shapeKinds[i]
		selected := k == p.st.spec.Kind
		b := &p.shapeBtns[i]
		return b.Clickable.Layout(gtx, func(gtx C) D {
			bg := colControl
			fg := colText
			if selected {
				bg, fg = colAccent, rgb(0xffffff)
			} else if b.Hovered() {
				bg = colHover
			}
			w := gtx.Constraints.Min.X
			h := gtx.Dp(58)
			fillRRect(gtx.Ops, image.Rect(0, 0, w, h), gtx.Dp(5), bg)
			// Icon.
			sp := shape.Default(k)
			if p.specs[k].Kind == k {
				sp = p.specs[k]
			}
			if selected {
				sp = p.st.spec
			}
			box := geom.FitRect(geom.Rect{X: 0, Y: float64(gtx.Dp(8)), W: float64(w), H: float64(gtx.Dp(26))}, sp.Aspect())
			path := sp.Path(box.W, box.H).Transform(geom.Translate(box.X, box.Y))
			paint.FillShape(gtx.Ops, fg, clip.Outline{Path: pathSpec(gtx.Ops, path)}.Op())
			// Label.
			l := label(th, 11, fg, k.Label())
			l.Alignment = text.Middle
			off := gtx.Dp(38)
			gtx.Constraints = layout.Exact(image.Pt(w, h-off))
			stack := op.Offset(image.Pt(0, off)).Push(gtx.Ops)
			l.Layout(gtx)
			stack.Pop()
			return D{Size: image.Pt(w, h)}
		})
	})
}

func (p *cropPage) shapeParams(gtx C) []layout.Widget {
	th := p.a.th
	sp := &p.st.spec
	switch sp.Kind {
	case shape.RoundedRect:
		if p.corner.Update(gtx) {
			p.change("corner")
			sp.Corner = float64(p.corner.Value) * 0.5
		} else if !p.corner.Dragging() {
			p.corner.Value = float32(sp.Corner / 0.5)
		}
		return []layout.Widget{func(gtx C) D {
			d, _ := p.corner.layout(gtx, th, "Corner radius", fmt.Sprintf("%.0f %%", sp.Corner/0.5*100))
			return d
		}}
	case shape.Triangle:
		dirs := []shape.Direction{shape.Up, shape.Down, shape.Left, shape.Right}
		sel := 0
		for i, d := range dirs {
			if d == sp.Apex {
				sel = i
			}
		}
		return []layout.Widget{func(gtx C) D {
			d, clicked := p.apex.layout(gtx, th, []string{"Up", "Down", "Left", "Right"}, sel, 4)
			if clicked >= 0 && dirs[clicked] != sp.Apex {
				p.change("")
				sp.Apex = dirs[clicked]
			}
			return d
		}}
	case shape.Polygon, shape.Star:
		title := "Corners"
		if sp.Kind == shape.Star {
			title = "Points"
		}
		if p.sides.Update(gtx) {
			n := 3 + int(math.Round(float64(p.sides.Value)*9))
			if n != sp.Sides {
				p.change("sides")
				sp.Sides = n
			}
		} else if !p.sides.Dragging() {
			p.sides.Value = float32(sp.Sides-3) / 9
		}
		ws := []layout.Widget{func(gtx C) D {
			d, _ := p.sides.layout(gtx, th, title, strconv.Itoa(sp.Sides))
			return d
		}}
		if sp.Kind == shape.Star {
			if p.inner.Update(gtx) {
				p.change("inner")
				sp.Inner = 0.1 + float64(p.inner.Value)*0.8
			} else if !p.inner.Dragging() {
				p.inner.Value = float32((sp.Inner - 0.1) / 0.8)
			}
			ws = append(ws, func(gtx C) D {
				d, _ := p.inner.layout(gtx, th, "Inner radius", fmt.Sprintf("%.0f %%", sp.Inner*100))
				return d
			})
		}
		return ws
	}
	return nil
}

func (p *cropPage) outputSection(gtx C) D {
	th := p.a.th
	w, h := p.outputSize()
	fmtSel := 0
	if p.jpeg {
		fmtSel = 1
	}
	ws := []layout.Widget{
		func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D { return p.wField.layout(gtx, th, "width", "px", false) }),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: 8, Right: 8}.Layout(gtx, label(th, 13, colMuted, "×").Layout)
				}),
				layout.Flexed(1, func(gtx C) D { return p.hField.layout(gtx, th, "height", "px", false) }),
			)
		},
		func(gtx C) D {
			return checkbox(gtx, th, &p.native, "Native resolution (1 source pixel = 1 output pixel)")
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
			d, _ := p.quality.layout(gtx, th, "JPG quality", strconv.Itoa(p.qualityValue()))
			return d
		})
	}
	ws = append(ws,
		func(gtx C) D { return boldLabel(th, 12, colMuted, "BACKGROUND").Layout(gtx) },
		func(gtx C) D {
			d, _ := p.bg.layout(gtx, th)
			return d
		},
	)
	info := fmt.Sprintf("Result: %d × %d px", w, h)
	if p.jpeg && p.bg.Transparent {
		info += " · JPG has no transparency, white is used"
	} else if p.bg.Transparent {
		info += " · transparent outside the shape"
	}
	ws = append(ws, hint(th, info))
	return section(gtx, th, "OUTPUT", ws...)
}

func (p *cropPage) handleCanvasInput(gtx C) {
	img, fr := p.imgSize(), p.frameSize()
	if p.src == nil || fr.W <= 0 {
		return
	}
	for {
		ev, ok := gtx.Event(
			pointer.Filter{
				Target:  &p.tag,
				Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
				ScrollY: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32},
			},
			key.Filter{Focus: &p.tag, Name: key.NameLeftArrow, Optional: key.ModShift},
			key.Filter{Focus: &p.tag, Name: key.NameRightArrow, Optional: key.ModShift},
			key.Filter{Focus: &p.tag, Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Focus: &p.tag, Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Focus: &p.tag, Name: "+"},
			key.Filter{Focus: &p.tag, Name: "-"},
			key.Filter{Focus: &p.tag, Name: "0"},
			key.Filter{Focus: &p.tag, Name: "F"},
			key.Filter{Focus: &p.tag, Name: "R", Optional: key.ModShift},
		)
		if !ok {
			break
		}
		// The panel was laid out before this event; draw another frame.
		gtx.Execute(op.InvalidateCmd{})
		switch e := ev.(type) {
		case pointer.Event:
			pos := gpt(e.Position).Sub(p.frame.Min())
			switch e.Kind {
			case pointer.Press:
				if p.dragging || p.rotating {
					break
				}
				switch {
				case e.Buttons.Contain(pointer.ButtonPrimary):
					gtx.Execute(key.FocusCmd{Tag: &p.tag})
					p.change("")
					p.dragging = true
					p.lastPos = e.Position
				case e.Buttons.Contain(pointer.ButtonSecondary):
					gtx.Execute(key.FocusCmd{Tag: &p.tag})
					p.change("")
					p.rotating = true
					p.rotAccum = p.st.view.Rotation
					p.lastAngle = angleAround(pos, fr)
				}
			case pointer.Drag:
				switch {
				case p.dragging:
					d := e.Position.Sub(p.lastPos)
					p.lastPos = e.Position
					p.st.view = p.st.view.Pan(float64(d.X), float64(d.Y), img, fr)
				case p.rotating:
					a := angleAround(pos, fr)
					p.rotAccum += crop.NormDeg(a - p.lastAngle)
					p.lastAngle = a
					target := p.rotAccum
					if e.Modifiers.Contain(key.ModShift) {
						target = math.Round(target/15) * 15
					}
					deg := crop.NormDeg(target - p.st.view.Rotation)
					p.st.view = p.st.view.RotateAt(geom.Pt{X: fr.W / 2, Y: fr.H / 2}, deg, img, fr)
				}
			case pointer.Release, pointer.Cancel:
				p.dragging, p.rotating = false, false
			case pointer.Scroll:
				steps := float64(e.Scroll.Y) / 120
				if e.Modifiers.Contain(key.ModShortcut) {
					p.change("rotate")
					p.st.view = p.st.view.RotateAt(geom.Pt{X: fr.W / 2, Y: fr.H / 2}, steps*2, img, fr)
				} else {
					p.change("zoom")
					p.st.view = p.st.view.ZoomAt(pos, math.Pow(1.15, -steps), img, fr)
				}
			}
		case key.Event:
			if e.State != key.Press {
				continue
			}
			step := 1.0
			if e.Modifiers.Contain(key.ModShift) {
				step = 10
			}
			switch e.Name {
			case key.NameLeftArrow:
				p.change("nudge")
				p.st.view = p.st.view.Pan(-step, 0, img, fr)
			case key.NameRightArrow:
				p.change("nudge")
				p.st.view = p.st.view.Pan(step, 0, img, fr)
			case key.NameUpArrow:
				p.change("nudge")
				p.st.view = p.st.view.Pan(0, -step, img, fr)
			case key.NameDownArrow:
				p.change("nudge")
				p.st.view = p.st.view.Pan(0, step, img, fr)
			case "+":
				p.change("zoom")
				p.st.view = p.st.view.SetZoom(p.st.view.Zoom * 1.1)
			case "-":
				p.change("zoom")
				p.st.view = p.st.view.SetZoom(p.st.view.Zoom / 1.1)
			case "0":
				p.change("")
				p.st.view = p.st.view.Fit(img, fr)
			case "F":
				p.change("")
				p.st.view = p.st.view.Fill(img, fr)
			case "R":
				deg := 90.0
				if e.Modifiers.Contain(key.ModShift) {
					deg = -90
				}
				p.change("")
				p.st.view = p.st.view.RotateAt(geom.Pt{X: fr.W / 2, Y: fr.H / 2}, deg, img, fr)
			}
		}
	}
}

func (p *cropPage) canvas(gtx C) D {
	size := gtx.Constraints.Max
	if p.src == nil {
		return emptyState(gtx, p.a.th, &p.emptyOpenBtn, icOpen, "Open an image to start", "Open image…", "or press Ctrl+O")
	}
	pad := float64(gtx.Dp(40))
	area := geom.Rect{W: float64(size.X), H: float64(size.Y)}.Inset(pad)
	p.frame = geom.FitRect(area, p.st.spec.Aspect())
	p.handleCanvasInput(gtx)

	fr := p.frameSize()
	a := p.st.view.ImageToFrame(p.imgSize(), fr).Then(geom.Translate(p.frame.X, p.frame.Y))

	// The whole image, dimmed.
	opac := paint.PushOpacity(gtx.Ops, 0.28)
	drawImage(gtx.Ops, p.preview, p.srcW, p.srcH, a)
	opac.Pop()

	// Inside the shape: background and the image at full brightness.
	path := p.st.spec.Path(fr.W, fr.H).Transform(geom.Translate(p.frame.X, p.frame.Y))
	cl := clip.Outline{Path: pathSpec(gtx.Ops, path)}.Op().Push(gtx.Ops)
	if bg := p.bg.Value(); bg.A == 0 && !p.jpeg {
		checkerboard(gtx, pxRect(p.frame))
	} else {
		if bg.A == 0 {
			bg = rgb(0xffffff)
		}
		fillRect(gtx.Ops, pxRect(p.frame), bg)
	}
	drawImage(gtx.Ops, p.preview, p.srcW, p.srcH, a)
	if p.dragging || p.rotating {
		thirds(gtx, p.frame)
	}
	cl.Pop()

	// Outline.
	if p.st.spec.Kind != shape.Rect {
		r := pxRect(p.frame)
		paint.FillShape(gtx.Ops, withAlpha(rgb(0xffffff), 0x30), clip.Stroke{Path: clip.Rect(r).Path(), Width: 1}.Op())
	}
	paint.FillShape(gtx.Ops, withAlpha(rgb(0x000000), 0x80), clip.Stroke{Path: pathSpec(gtx.Ops, path), Width: float32(gtx.Dp(3))}.Op())
	paint.FillShape(gtx.Ops, withAlpha(rgb(0xffffff), 0xe0), clip.Stroke{Path: pathSpec(gtx.Ops, path), Width: float32(gtx.Dp(1.2))}.Op())

	// Input area.
	ar := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.tag)
	switch {
	case p.dragging:
		pointer.CursorGrabbing.Add(gtx.Ops)
	case p.rotating:
		pointer.CursorCrosshair.Add(gtx.Ops)
	default:
		pointer.CursorGrab.Add(gtx.Ops)
	}
	ar.Pop()
	return D{Size: size}
}

// angleAround returns the angle in degrees (clockwise) of frame point p
// around the frame center.
func angleAround(p geom.Pt, fr crop.Size) float64 {
	return angleDeg(p, geom.Pt{X: fr.W / 2, Y: fr.H / 2})
}

// angleDeg returns the angle in degrees (clockwise on screen) of p around c.
func angleDeg(p, c geom.Pt) float64 {
	return math.Atan2(p.Y-c.Y, p.X-c.X) * 180 / math.Pi
}

// thirds draws rule-of-thirds guides inside r.
func thirds(gtx C, r geom.Rect) {
	c := withAlpha(rgb(0xffffff), 0x60)
	w := float64(gtx.Dp(1))
	for i := 1; i <= 2; i++ {
		x := r.X + r.W*float64(i)/3
		y := r.Y + r.H*float64(i)/3
		fillRect(gtx.Ops, pxRect(geom.Rect{X: x, Y: r.Y, W: w, H: r.H}), c)
		fillRect(gtx.Ops, pxRect(geom.Rect{X: r.X, Y: y, W: r.W, H: w}), c)
	}
}

// emptyState shows a centered call to action.
func emptyState(gtx C, th *material.Theme, btn *button, icon *widget.Icon, title, action, sub string) D {
	size := gtx.Constraints.Max
	layout.Center.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min = image.Point{}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				s := gtx.Dp(56)
				gtx.Constraints = layout.Exact(image.Pt(s, s))
				return icon.Layout(gtx, colBorder)
			}),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(label(th, 16, colText, title).Layout),
			layout.Rigid(layout.Spacer{Height: 16}.Layout),
			layout.Rigid(func(gtx C) D { return btn.layout(gtx, th, nil, action, btnPrimary, false) }),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(label(th, 12, colMuted, sub).Layout),
		)
	})
	return D{Size: size}
}
