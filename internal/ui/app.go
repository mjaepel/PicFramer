// Package ui implements the PicFramer desktop user interface with Gio.
package ui

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/mjaepel/picframer/internal/imgio"
)

// page is one of the two modes (crop, collage).
type page interface {
	panel(gtx C) []layout.Widget
	canvas(gtx C) D
	open()
	export()
	undo()
	redo()
	canUndo() bool
	canRedo() bool
}

// App is the main window.
type App struct {
	win        *app.Window
	th         *material.Theme
	dlg        dialogs
	dialogBusy atomic.Bool
	tasks      chan func()

	mode     int
	tabs     [2]button
	undoBtn  button
	redoBtn  button
	panelLst widget.List

	crop    *cropPage
	collage *collagePage

	status    string
	statusErr bool
}

// Run opens the main window and blocks until it is closed. One image path
// in args opens it for cropping; several start a collage.
func Run(args []string) {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("PicFramer"),
			app.Size(unit.Dp(1280), unit.Dp(820)),
			app.MinSize(unit.Dp(900), unit.Dp(560)),
		)
		a := newApp(w)
		a.openArgs(args)
		if err := a.loop(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func newApp(w *app.Window) *App {
	a := &App{
		win:   w,
		th:    newTheme(),
		tasks: make(chan func(), 64),
	}
	a.panelLst.Axis = layout.Vertical
	a.crop = newCropPage(a)
	a.collage = newCollagePage(a)
	a.status = "Open an image to crop it, or switch to Collage."
	return a
}

func (a *App) loop() error {
	var ops op.Ops
	for {
		e := a.win.Event()
		a.dlg.listen(e)
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			a.runTasks()
			gtx := app.NewContext(&ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// post schedules f to run on the UI goroutine. It is safe to call from any
// goroutine.
func (a *App) post(f func()) {
	a.tasks <- f
	a.win.Invalidate()
}

func (a *App) runTasks() {
	for {
		select {
		case f := <-a.tasks:
			f()
		default:
			return
		}
	}
}

func (a *App) setStatus(msg string) {
	a.status, a.statusErr = msg, false
}

func (a *App) setError(err error) {
	a.status, a.statusErr = "Error: "+err.Error(), true
}

func (a *App) page() page {
	if a.mode == 1 {
		return a.collage
	}
	return a.crop
}

// chooseImages shows an open dialog (on a background goroutine) and calls
// done on the UI goroutine with the chosen paths.
func (a *App) chooseImages(multi bool, done func(paths []string)) {
	if !a.dialogBusy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.dialogBusy.Store(false)
		if paths := a.dlg.openImages(imgio.Extensions, multi); len(paths) > 0 {
			a.post(func() { done(paths) })
		}
	}()
}

// saveFile shows a save dialog proposing name (on a background goroutine)
// and calls write with the created file. The file is closed afterwards.
func (a *App) saveFile(name string, write func(f *os.File) error) {
	if !a.dialogBusy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		path := a.dlg.saveFile(name)
		a.dialogBusy.Store(false)
		if path == "" {
			return
		}
		a.post(func() { a.setStatus("Exporting " + filepath.Base(path) + " …") })
		start := time.Now()
		f, err := os.Create(path)
		if err == nil {
			err = write(f)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				os.Remove(path)
			}
		}
		a.post(func() {
			if err != nil {
				a.setError(err)
				return
			}
			a.setStatus("Exported " + path + " (" + time.Since(start).Round(10*time.Millisecond).String() + ")")
		})
	}()
}

func baseName(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

func (a *App) shortcuts(gtx C) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "O", Required: key.ModShortcut},
			key.Filter{Name: "E", Required: key.ModShortcut},
			key.Filter{Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Name: "Y", Required: key.ModShortcut},
			key.Filter{Name: "1", Required: key.ModShortcut},
			key.Filter{Name: "2", Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		p := a.page()
		switch e.Name {
		case "O":
			p.open()
		case "E":
			p.export()
		case "Z":
			if e.Modifiers.Contain(key.ModShift) {
				p.redo()
			} else {
				p.undo()
			}
		case "Y":
			p.redo()
		case "1":
			a.mode = 0
		case "2":
			a.mode = 1
		}
	}
}

func (a *App) layout(gtx C) {
	a.shortcuts(gtx)
	for i := range a.tabs {
		if a.tabs[i].Clicked(gtx) {
			a.mode = i
		}
	}
	p := a.page()
	if a.undoBtn.Clicked(gtx) {
		p.undo()
	}
	if a.redoBtn.Clicked(gtx) {
		p.redo()
	}

	fillRect(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, colWindow)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.topBar),
		layout.Rigid(separator),
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					w := gtx.Dp(316)
					gtx.Constraints = layout.Exact(image.Pt(w, gtx.Constraints.Max.Y))
					fillRect(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, colPanel)
					items := p.panel(gtx)
					l := material.List(a.th, &a.panelLst)
					l.AnchorStrategy = material.Overlay
					return l.Layout(gtx, len(items), func(gtx C, i int) D {
						if i > 0 {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(separator),
								layout.Rigid(items[i]),
							)
						}
						return items[i](gtx)
					})
				}),
				layout.Rigid(func(gtx C) D {
					w := gtx.Dp(1)
					fillRect(gtx.Ops, image.Rect(0, 0, w, gtx.Constraints.Max.Y), colBorder)
					return D{Size: image.Pt(w, gtx.Constraints.Max.Y)}
				}),
				layout.Flexed(1, func(gtx C) D {
					gtx.Constraints.Min = gtx.Constraints.Max
					defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
					fillRect(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, colCanvas)
					return p.canvas(gtx)
				}),
			)
		}),
		layout.Rigid(separator),
		layout.Rigid(a.statusBar),
	)
}

func (a *App) topBar(gtx C) D {
	p := a.page()
	return layout.Inset{Top: 8, Bottom: 8, Left: 16, Right: 12}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(boldLabel(a.th, 18, colText, "PicFramer").Layout),
			layout.Rigid(layout.Spacer{Width: 28}.Layout),
			layout.Rigid(func(gtx C) D {
				kind := func(i int) buttonKind {
					if a.mode == i {
						return btnSelected
					}
					return btnNormal
				}
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return a.tabs[0].layout(gtx, a.th, icCrop, "Crop", kind(0), false) }),
					layout.Rigid(layout.Spacer{Width: 6}.Layout),
					layout.Rigid(func(gtx C) D { return a.tabs[1].layout(gtx, a.th, icCollage, "Collage", kind(1), false) }),
				)
			}),
			layout.Flexed(1, func(gtx C) D { return D{Size: image.Pt(gtx.Constraints.Min.X, 0)} }),
			layout.Rigid(func(gtx C) D {
				if !p.canUndo() {
					gtx = gtx.Disabled()
				}
				return a.undoBtn.icon(gtx, icUndo, 18, colControl)
			}),
			layout.Rigid(layout.Spacer{Width: 6}.Layout),
			layout.Rigid(func(gtx C) D {
				if !p.canRedo() {
					gtx = gtx.Disabled()
				}
				return a.redoBtn.icon(gtx, icRedo, 18, colControl)
			}),
		)
	})
}

func (a *App) statusBar(gtx C) D {
	c := colMuted
	if a.statusErr {
		c = colError
	}
	return layout.Inset{Top: 5, Bottom: 5, Left: 16, Right: 16}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		l := label(a.th, 12, c, a.status)
		l.MaxLines = 1
		return l.Layout(gtx)
	})
}

func (a *App) openArgs(args []string) {
	switch len(args) {
	case 0:
	case 1:
		a.crop.load(args[0])
	default:
		a.mode = 1
		a.collage.addPaths(args)
	}
}
