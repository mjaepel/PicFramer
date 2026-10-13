// PicFramer crops pictures into shapes and arranges them into collages.
package main

// Embeds icon, manifest and version info (winres/winres.json) into the .exe.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64

import (
	"os"

	"github.com/mjaepel/picframer/internal/ui"
)

func main() {
	ui.Run(os.Args[1:])
}
