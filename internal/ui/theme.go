package ui

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

// Palette of the dark UI.
var (
	colWindow   = rgb(0x17181b)
	colPanel    = rgb(0x212226)
	colControl  = rgb(0x2c2e33)
	colHover    = rgb(0x383a41)
	colBorder   = rgb(0x3a3c43)
	colText     = rgb(0xe8e9ec)
	colMuted    = rgb(0x9a9ca3)
	colAccent   = rgb(0x5b8def)
	colCanvas   = rgb(0x101113)
	colError    = rgb(0xff6b6b)
	colChecker  = rgb(0x3a3a3a)
	colChecker2 = rgb(0x2a2a2a)
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         colPanel,
		Fg:         colText,
		ContrastBg: colAccent,
		ContrastFg: rgb(0xffffff),
	}
	th.TextSize = 14
	return th
}

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

var (
	icOpen     = mustIcon(icons.FileFolderOpen)
	icAdd      = mustIcon(icons.ContentAdd)
	icExport   = mustIcon(icons.FileFileDownload)
	icRotRight = mustIcon(icons.ImageRotateRight)
	icRotLeft  = mustIcon(icons.ImageRotateLeft)
	icClose    = mustIcon(icons.NavigationClose)
	icSwap     = mustIcon(icons.ActionSwapHoriz)
	icFit      = mustIcon(icons.ImageCropFree)
	icFill     = mustIcon(icons.ImageCropSquare)
	icMove     = mustIcon(icons.ActionOpenWith)
	icUndo     = mustIcon(icons.ContentUndo)
	icRedo     = mustIcon(icons.ContentRedo)
	icRelayout = mustIcon(icons.ActionAutorenew)
	icReset    = mustIcon(icons.NavigationRefresh)
	icClear    = mustIcon(icons.ActionDelete)
	icCrop     = mustIcon(icons.ImageCrop)
	icCollage  = mustIcon(icons.ActionDashboard)
)

// fillRect paints r with c.
func fillRect(ops *op.Ops, r image.Rectangle, c color.NRGBA) {
	paint.FillShape(ops, c, clip.Rect(r).Op())
}

// fillRRect paints a rounded rectangle.
func fillRRect(ops *op.Ops, r image.Rectangle, radius int, c color.NRGBA) {
	paint.FillShape(ops, c, clip.UniformRRect(r, radius).Op(ops))
}

// label is a text label with an explicit color and size.
func label(th *material.Theme, size unit.Sp, c color.NRGBA, txt string) material.LabelStyle {
	l := material.Label(th, size, txt)
	l.Color = c
	return l
}

func boldLabel(th *material.Theme, size unit.Sp, c color.NRGBA, txt string) material.LabelStyle {
	l := label(th, size, c, txt)
	l.Font.Weight = font.SemiBold
	return l
}

// section lays out a titled group of controls.
func section(gtx C, th *material.Theme, title string, children ...layout.Widget) D {
	items := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Bottom: 8}.Layout(gtx, boldLabel(th, 12, colMuted, title).Layout)
		}),
	}
	for i, w := range children {
		w := w
		top := unit.Dp(8)
		if i == 0 {
			top = 0
		}
		items = append(items, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: top}.Layout(gtx, w)
		}))
	}
	return layout.Inset{Top: 14, Bottom: 14, Left: 16, Right: 16}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
	})
}

// separator draws a thin horizontal line.
func separator(gtx C) D {
	h := gtx.Dp(1)
	fillRect(gtx.Ops, image.Rect(0, 0, gtx.Constraints.Max.X, h), colBorder)
	return D{Size: image.Pt(gtx.Constraints.Max.X, h)}
}

// button is a flat button with optional icon.
type button struct {
	widget.Clickable
}

type buttonKind int

const (
	btnNormal buttonKind = iota
	btnPrimary
	btnSelected
)

// layout draws the button filling the available width if expand is set.
func (b *button) layout(gtx C, th *material.Theme, icon *widget.Icon, txt string, kind buttonKind, expand bool) D {
	bg := colControl
	fg := colText
	switch kind {
	case btnPrimary, btnSelected:
		bg = colAccent
		fg = rgb(0xffffff)
	}
	if b.Hovered() {
		if kind == btnNormal {
			bg = colHover
		} else {
			bg = lighten(bg, 18)
		}
	}
	if !gtx.Enabled() {
		bg = withAlpha(bg, 0x60)
		fg = withAlpha(fg, 0x70)
	}
	return b.Clickable.Layout(gtx, func(gtx C) D {
		if !expand {
			gtx.Constraints.Min.X = 0
		}
		m := op.Record(gtx.Ops)
		dims := layout.Inset{Top: 7, Bottom: 7, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			var items []layout.FlexChild
			if icon != nil {
				items = append(items, layout.Rigid(func(gtx C) D {
					sz := gtx.Dp(18)
					gtx.Constraints = layout.Exact(image.Pt(sz, sz))
					return icon.Layout(gtx, fg)
				}))
				if txt != "" {
					items = append(items, layout.Rigid(layout.Spacer{Width: 6}.Layout))
				}
			}
			if txt != "" {
				items = append(items, layout.Rigid(func(gtx C) D {
					l := label(th, 13, fg, txt)
					l.MaxLines = 1
					return l.Layout(gtx)
				}))
			}
			gtx.Constraints.Min.X = max(gtx.Constraints.Min.X, 0)
			return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceSides}.Layout(gtx, items...)
		})
		call := m.Stop()
		fillRRect(gtx.Ops, image.Rectangle{Max: dims.Size}, gtx.Dp(5), bg)
		call.Add(gtx.Ops)
		return dims
	})
}

func lighten(c color.NRGBA, d uint8) color.NRGBA {
	add := func(v uint8) uint8 {
		if int(v)+int(d) > 255 {
			return 255
		}
		return v + d
	}
	return color.NRGBA{add(c.R), add(c.G), add(c.B), c.A}
}

// iconButton is a small square icon-only button.
func (b *button) icon(gtx C, icon *widget.Icon, size unit.Dp, bg color.NRGBA) D {
	if b.Hovered() {
		bg = lighten(bg, 28)
	}
	return b.Clickable.Layout(gtx, func(gtx C) D {
		pad := gtx.Dp(6)
		s := gtx.Dp(size)
		sz := image.Pt(s+2*pad, s+2*pad)
		fillRRect(gtx.Ops, image.Rectangle{Max: sz}, gtx.Dp(5), bg)
		off := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
		gtx.Constraints = layout.Exact(image.Pt(s, s))
		fg := colText
		if !gtx.Enabled() {
			fg = withAlpha(fg, 0x60)
		}
		icon.Layout(gtx, fg)
		off.Pop()
		return D{Size: sz}
	})
}

// grid lays out n equally wide cells per row.
func grid(gtx C, n, cols int, gap unit.Dp, cell func(gtx C, i int) D) D {
	var rows []layout.FlexChild
	for r := 0; r*cols < n; r++ {
		r := r
		if r > 0 {
			rows = append(rows, layout.Rigid(layout.Spacer{Height: gap}.Layout))
		}
		rows = append(rows, layout.Rigid(func(gtx C) D {
			var items []layout.FlexChild
			for c := 0; c < cols; c++ {
				i := r*cols + c
				if c > 0 {
					items = append(items, layout.Rigid(layout.Spacer{Width: gap}.Layout))
				}
				items = append(items, layout.Flexed(1, func(gtx C) D {
					if i >= n {
						return D{Size: image.Pt(gtx.Constraints.Min.X, 0)}
					}
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return cell(gtx, i)
				}))
			}
			return layout.Flex{}.Layout(gtx, items...)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// choice is a group of mutually exclusive buttons.
type choice struct {
	btns []button
}

// layout draws the options in a grid and returns the index of a clicked
// option, or -1.
func (c *choice) layout(gtx C, th *material.Theme, labels []string, selected, cols int) (D, int) {
	if len(c.btns) != len(labels) {
		c.btns = make([]button, len(labels))
	}
	clicked := -1
	for i := range c.btns {
		if c.btns[i].Clicked(gtx) {
			clicked = i
		}
	}
	dims := grid(gtx, len(labels), cols, 6, func(gtx C, i int) D {
		kind := btnNormal
		if i == selected {
			kind = btnSelected
		}
		return c.btns[i].layout(gtx, th, nil, labels[i], kind, true)
	})
	return dims, clicked
}

// field is a bordered single-line text input.
type field struct {
	widget.Editor
}

func (f *field) layout(gtx C, th *material.Theme, hint, suffix string, invalid bool) D {
	f.SingleLine = true
	f.Submit = true
	border := colBorder
	if gtx.Focused(&f.Editor) {
		border = colAccent
	}
	if invalid {
		border = colError
	}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(1).Layout(gtx, func(gtx C) D {
		mm := op.Record(gtx.Ops)
		d := layout.Inset{Top: 7, Bottom: 7, Left: 9, Right: 9}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					e := material.Editor(th, &f.Editor, hint)
					e.TextSize = 13
					e.HintColor = colMuted
					e.SelectionColor = withAlpha(colAccent, 0x60)
					return e.Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					if suffix == "" {
						return D{}
					}
					return layout.Inset{Left: 4}.Layout(gtx, label(th, 12, colMuted, suffix).Layout)
				}),
			)
		})
		c := mm.Stop()
		fillRRect(gtx.Ops, image.Rectangle{Max: d.Size}, gtx.Dp(4), colControl)
		c.Add(gtx.Ops)
		return d
	})
	c := m.Stop()
	fillRRect(gtx.Ops, image.Rectangle{Max: dims.Size}, gtx.Dp(5), border)
	c.Add(gtx.Ops)
	return dims
}

// slider is a labelled slider mapping widget.Float's 0..1 to a value.
type slider struct {
	widget.Float
}

// layout shows title and value text above the slider. It returns true if
// the user changed the value; the new position is then in s.Value.
func (s *slider) layout(gtx C, th *material.Theme, title, value string) (D, bool) {
	changed := s.Update(gtx)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, label(th, 13, colText, title).Layout),
				layout.Rigid(label(th, 13, colMuted, value).Layout),
			)
		}),
		layout.Rigid(func(gtx C) D {
			sl := material.Slider(th, &s.Float)
			sl.Color = colAccent
			sl.FingerSize = 20
			return sl.Layout(gtx)
		}),
	)
	return dims, changed
}

// numSlider is a slider whose value can also be typed into a field.
type numSlider struct {
	widget.Float
	input field
}

// typed drains the input field and returns the number the user entered,
// if it is valid.
func (s *numSlider) typed(gtx C) (float64, bool) {
	if !s.input.edited(gtx) {
		return 0, false
	}
	t := strings.TrimSpace(strings.ReplaceAll(s.input.Text(), ",", "."))
	v, err := strconv.ParseFloat(t, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// layout shows the title, an input field holding value (with prec decimals
// and unit suffix) and the slider. Call Update and typed before it.
func (s *numSlider) layout(gtx C, th *material.Theme, title string, value float64, prec int, suffix string) D {
	txt := strconv.FormatFloat(value, 'f', prec, 64)
	if !gtx.Focused(&s.input.Editor) && s.input.Text() != txt {
		s.input.SetText(txt)
	}
	s.input.Filter = "0123456789.,-"
	s.input.MaxLen = 9
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, label(th, 13, colText, title).Layout),
				layout.Rigid(func(gtx C) D {
					w := gtx.Dp(96)
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
					return s.input.layout(gtx, th, "", suffix, false)
				}),
			)
		}),
		layout.Rigid(func(gtx C) D {
			sl := material.Slider(th, &s.Float)
			sl.Color = colAccent
			sl.FingerSize = 20
			return sl.Layout(gtx)
		}),
	)
}

// checkbox wraps material.CheckBox with the UI's colors.
func checkbox(gtx C, th *material.Theme, b *widget.Bool, txt string) D {
	cb := material.CheckBox(th, b, txt)
	cb.Color = colText
	cb.IconColor = colAccent
	cb.TextSize = 13
	cb.Size = 20
	return cb.Layout(gtx)
}

// hint is muted help text.
func hint(th *material.Theme, txt string) layout.Widget {
	return label(th, 12, colMuted, txt).Layout
}

// edited drains the editor's events and reports whether the user changed
// or submitted the text. Programmatic SetText calls also emit change events;
// they are ignored because the field is not focused then.
func (f *field) edited(gtx C) bool {
	changed := false
	for {
		ev, ok := f.Update(gtx)
		if !ok {
			return changed
		}
		switch ev.(type) {
		case widget.ChangeEvent, widget.SubmitEvent:
			if gtx.Focused(&f.Editor) {
				changed = true
			}
		}
	}
}
