// Package imgio loads and saves images.
package imgio

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"

	// Register additional decoders.
	_ "image/gif"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Extensions lists the file extensions that can be opened.
var Extensions = []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff"}

// Decode reads an image and applies its EXIF orientation.
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("unsupported or damaged image: %w", err)
	}
	return applyOrientation(img, jpegOrientation(data)), nil
}

// Load opens and decodes the image at path.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

// Thumbnail scales img down so that its longer side is at most limit pixels.
// Smaller images are returned unchanged.
func Thumbnail(img image.Image, limit int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= limit && h <= limit {
		return img
	}
	if w >= h {
		h = max(1, h*limit/w)
		w = limit
	} else {
		w = max(1, w*limit/h)
		h = limit
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// Format is an export file format.
type Format string

const (
	PNG  Format = "png"
	JPEG Format = "jpg"
)

// Ext returns the file extension including the dot.
func (f Format) Ext() string { return "." + string(f) }

// FormatFromPath guesses the format from a file name, defaulting to fallback.
func FormatFromPath(path string, fallback Format) Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return PNG
	case ".jpg", ".jpeg":
		return JPEG
	}
	return fallback
}

// Encode writes img in the given format. quality (1..100) applies to JPEG.
func Encode(w io.Writer, img image.Image, f Format, quality int) error {
	switch f {
	case JPEG:
		if quality < 1 || quality > 100 {
			quality = 92
		}
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	default:
		return (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(w, img)
	}
}
