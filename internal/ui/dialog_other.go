//go:build !windows

package ui

import "gioui.org/io/event"

// PicFramer targets Windows; on other systems the file dialogs are unavailable
// and images can only be opened via command line arguments.
type dialogs struct{}

func (d *dialogs) listen(event.Event)                 {}
func (d *dialogs) openImages([]string, bool) []string { return nil }
func (d *dialogs) saveFile(string) string             { return "" }
