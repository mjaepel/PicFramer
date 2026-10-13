//go:build windows

package ui

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows"
)

// Native file dialogs (comdlg32). Unlike gioui.org/x/explorer they are owned
// by the main window, so they stay in front of it and block it while open.

var (
	comdlg32         = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileW = comdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileW = comdlg32.NewProc("GetSaveFileNameW")
)

const (
	ofnOverwritePrompt  = 0x00000002
	ofnHideReadOnly     = 0x00000004
	ofnNoChangeDir      = 0x00000008
	ofnAllowMultiSelect = 0x00000200
	ofnPathMustExist    = 0x00000800
	ofnFileMustExist    = 0x00001000
	ofnExplorer         = 0x00080000

	pathBufLen = 1 << 16
)

type openFileName struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      uintptr
	DwReserved      uint32
	FlagsEx         uint32
}

type dialogs struct {
	owner atomic.Uintptr
}

// listen records the main window handle.
func (d *dialogs) listen(e event.Event) {
	if v, ok := e.(app.Win32ViewEvent); ok {
		d.owner.Store(v.HWND)
	}
}

// filter builds a double-NUL terminated comdlg32 filter string from
// (description, pattern) pairs.
func filter(pairs ...string) *uint16 {
	var b []uint16
	for _, s := range pairs {
		b = append(b, windows.StringToUTF16(s)...) // includes the NUL
	}
	b = append(b, 0)
	return &b[0]
}

func imageFilter(exts []string) *uint16 {
	pats := make([]string, len(exts))
	for i, e := range exts {
		pats[i] = "*" + e
	}
	p := strings.Join(pats, ";")
	return filter("Images ("+p+")", p, "All files (*.*)", "*.*")
}

// run calls a comdlg32 function on a locked, COM-initialized thread.
func run(proc *windows.LazyProc, o *openFileName) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(o)))
	return r != 0
}

// openImages shows an open dialog and returns the chosen paths (nil if
// cancelled). It blocks; call it from a goroutine.
func (d *dialogs) openImages(exts []string, multi bool) []string {
	buf := make([]uint16, pathBufLen)
	title, _ := windows.UTF16PtrFromString("Open pictures")
	o := openFileName{
		Owner:   d.owner.Load(),
		Filter:  imageFilter(exts),
		File:    &buf[0],
		MaxFile: uint32(len(buf)),
		Title:   title,
		Flags:   ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir,
	}
	if multi {
		o.Flags |= ofnAllowMultiSelect
	}
	o.StructSize = uint32(unsafe.Sizeof(o))
	if !run(procGetOpenFileW, &o) {
		return nil
	}
	// The buffer holds either one full path, or a directory followed by
	// file names, separated by NULs and terminated by a double NUL.
	var parts []string
	start := 0
	for i, c := range buf {
		if c != 0 {
			continue
		}
		if i == start {
			break
		}
		parts = append(parts, windows.UTF16ToString(buf[start:i]))
		start = i + 1
	}
	if len(parts) <= 1 {
		return parts
	}
	paths := make([]string, len(parts)-1)
	for i, name := range parts[1:] {
		paths[i] = filepath.Join(parts[0], name)
	}
	return paths
}

// saveFile shows a save dialog proposing name and returns the chosen path
// ("" if cancelled). The extension of name selects the file type.
func (d *dialogs) saveFile(name string) string {
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	buf := make([]uint16, pathBufLen)
	copy(buf, windows.StringToUTF16(name))
	title, _ := windows.UTF16PtrFromString("Export")
	defExt, _ := windows.UTF16PtrFromString(ext)
	desc := strings.ToUpper(ext) + " image (*." + ext + ")"
	o := openFileName{
		Owner:   d.owner.Load(),
		Filter:  filter(desc, "*."+ext),
		File:    &buf[0],
		MaxFile: uint32(len(buf)),
		Title:   title,
		DefExt:  defExt,
		Flags:   ofnExplorer | ofnOverwritePrompt | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir,
	}
	o.StructSize = uint32(unsafe.Sizeof(o))
	if !run(procGetSaveFileW, &o) {
		return ""
	}
	return windows.UTF16ToString(buf)
}
