package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"

	"github.com/mjaepel/picframer/internal/geom"
)

func f32pt(p geom.Pt) f32.Point { return f32.Pt(float32(p.X), float32(p.Y)) }

func toAffine(a geom.Aff) f32.Affine2D {
	return f32.NewAffine2D(float32(a[0]), float32(a[1]), float32(a[2]), float32(a[3]), float32(a[4]), float32(a[5]))
}

func gpt(p f32.Point) geom.Pt { return geom.Pt{X: float64(p.X), Y: float64(p.Y)} }

func pxRect(r geom.Rect) image.Rectangle {
	return image.Rect(int(r.X+0.5), int(r.Y+0.5), int(r.X+r.W+0.5), int(r.Y+r.H+0.5))
}

// pathSpec converts a geometry path to a Gio clip path.
func pathSpec(ops *op.Ops, p geom.Path) clip.PathSpec {
	var cp clip.Path
	cp.Begin(ops)
	for _, s := range p {
		switch s.Kind {
		case geom.MoveTo:
			cp.MoveTo(f32pt(s.P[0]))
		case geom.LineTo:
			cp.LineTo(f32pt(s.P[0]))
		case geom.QuadTo:
			cp.QuadTo(f32pt(s.P[0]), f32pt(s.P[1]))
		case geom.CubeTo:
			cp.CubeTo(f32pt(s.P[0]), f32pt(s.P[1]), f32pt(s.P[2]))
		case geom.Close:
			cp.Close()
		}
	}
	return cp.End()
}

// drawImage paints img (shown at size srcSize source pixels) transformed by
// a, which maps source pixel coordinates to the current coordinates.
func drawImage(ops *op.Ops, img paint.ImageOp, srcW, srcH float64, a geom.Aff) {
	sz := img.Size()
	if sz.X == 0 || sz.Y == 0 {
		return
	}
	// The preview texture may be smaller than the source image.
	a = geom.Scale(srcW/float64(sz.X), srcH/float64(sz.Y)).Then(a)
	t := op.Affine(toAffine(a)).Push(ops)
	cl := clip.Rect{Max: sz}.Push(ops)
	img.Add(ops)
	paint.PaintOp{}.Add(ops)
	cl.Pop()
	t.Pop()
}

// checkerboard fills r with a transparency checkerboard.
func checkerboard(gtx C, r image.Rectangle) {
	fillRect(gtx.Ops, r, colChecker2)
	s := gtx.Dp(8)
	var p clip.Path
	p.Begin(gtx.Ops)
	for y := r.Min.Y; y < r.Max.Y; y += s {
		for x := r.Min.X + ((y-r.Min.Y)/s%2)*s; x < r.Max.X; x += 2 * s {
			x1, y1 := min(x+s, r.Max.X), min(y+s, r.Max.Y)
			p.MoveTo(f32.Pt(float32(x), float32(y)))
			p.LineTo(f32.Pt(float32(x1), float32(y)))
			p.LineTo(f32.Pt(float32(x1), float32(y1)))
			p.LineTo(f32.Pt(float32(x), float32(y1)))
			p.Close()
		}
	}
	paint.FillShape(gtx.Ops, colChecker, clip.Outline{Path: p.End()}.Op())
}

// history is an undo/redo stack of snapshots. Pushes with the same key in
// quick succession are coalesced (e.g. many wheel steps form one undo step).
type history[T any] struct {
	undo, redo []T
	lastKey    string
	lastTime   time.Time
}

const coalesceWindow = 800 * time.Millisecond

// push records cur as the state before a change.
func (h *history[T]) push(cur T, key string) {
	now := time.Now()
	if key != "" && key == h.lastKey && now.Sub(h.lastTime) < coalesceWindow {
		h.lastTime = now
		return
	}
	h.lastKey, h.lastTime = key, now
	h.undo = append(h.undo, cur)
	if len(h.undo) > 200 {
		h.undo = h.undo[1:]
	}
	h.redo = nil
}

func (h *history[T]) canUndo() bool { return len(h.undo) > 0 }
func (h *history[T]) canRedo() bool { return len(h.redo) > 0 }

func (h *history[T]) doUndo(cur T) (T, bool) {
	if len(h.undo) == 0 {
		return cur, false
	}
	prev := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, cur)
	h.lastKey = ""
	return prev, true
}

func (h *history[T]) doRedo(cur T) (T, bool) {
	if len(h.redo) == 0 {
		return cur, false
	}
	next := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, cur)
	h.lastKey = ""
	return next, true
}

func (h *history[T]) clear() {
	h.undo, h.redo, h.lastKey = nil, nil, ""
}

// Background color choice shared by both modes.
var swatches = []color.NRGBA{
	rgb(0xffffff), rgb(0xf2efe9), rgb(0xd9d9d9), rgb(0x808080),
	rgb(0x333333), rgb(0x000000),
}

func hexColor(c color.NRGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func parseHex(s string) (color.NRGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return rgb(uint32(v)), true
}

// bgPicker selects "transparent" or a solid color.
type bgPicker struct {
	mode   choice
	swatch [8]button
	hex    field
	// Transparent and Color hold the current selection.
	Transparent bool
	Color       color.NRGBA
	hexInvalid  bool
}

func newBgPicker(transparent bool, c color.NRGBA) *bgPicker {
	b := &bgPicker{Transparent: transparent, Color: c}
	b.hex.SetText(hexColor(c))
	return b
}

// Value returns the background as a color with alpha 0 for transparent.
func (b *bgPicker) Value() color.NRGBA {
	if b.Transparent {
		return color.NRGBA{}
	}
	return b.Color
}

// layout returns true when the selection changed.
func (b *bgPicker) layout(gtx C, th *material.Theme) (D, bool) {
	changed := false
	if b.hex.edited(gtx) {
		if c, ok := parseHex(b.hex.Text()); ok {
			b.hexInvalid = false
			if c != b.Color || b.Transparent {
				b.Color, b.Transparent, changed = c, false, true
			}
		} else {
			b.hexInvalid = true
		}
	}
	for i := range swatches {
		if b.swatch[i].Clicked(gtx) {
			b.Color, b.Transparent, changed = swatches[i], false, true
			b.hex.SetText(hexColor(b.Color))
			b.hexInvalid = false
		}
	}
	sel := 1
	if b.Transparent {
		sel = 0
	}
	var clicked int
	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			var d D
			d, clicked = b.mode.layout(gtx, th, []string{"Transparent", "Color"}, sel, 2)
			return d
		}),
	}
	if !b.Transparent {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx C) D {
				var items []layout.FlexChild
				for i := range swatches {
					i := i
					items = append(items, layout.Rigid(func(gtx C) D {
						return layout.Inset{Right: 6}.Layout(gtx, func(gtx C) D {
							return b.swatch[i].Clickable.Layout(gtx, func(gtx C) D {
								s := gtx.Dp(22)
								bd := colBorder
								if swatches[i] == b.Color {
									bd = colAccent
								}
								fillRRect(gtx.Ops, image.Rect(0, 0, s, s), gtx.Dp(4), bd)
								in := gtx.Dp(2)
								fillRRect(gtx.Ops, image.Rect(in, in, s-in, s-in), gtx.Dp(3), swatches[i])
								return D{Size: image.Pt(s, s)}
							})
						})
					}))
				}
				items = append(items, layout.Flexed(1, func(gtx C) D {
					return b.hex.layout(gtx, th, "#RRGGBB", "", b.hexInvalid)
				}))
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, items...)
			}),
		)
	}
	d := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	switch clicked {
	case 0:
		if !b.Transparent {
			b.Transparent, changed = true, true
		}
	case 1:
		if b.Transparent {
			b.Transparent, changed = false, true
		}
	}
	return d, changed
}
