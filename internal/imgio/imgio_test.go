package imgio

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// withOrientation inserts an EXIF APP1 segment with the given orientation
// right after the SOI marker of a JPEG.
func withOrientation(jpg []byte, o uint16, bo binary.ByteOrder) []byte {
	var tiff bytes.Buffer
	if bo == binary.LittleEndian {
		tiff.WriteString("II")
	} else {
		tiff.WriteString("MM")
	}
	b := make([]byte, 2)
	bo.PutUint16(b, 42)
	tiff.Write(b)
	b4 := make([]byte, 4)
	bo.PutUint32(b4, 8)
	tiff.Write(b4)
	bo.PutUint16(b, 1) // one entry
	tiff.Write(b)
	entry := make([]byte, 12)
	bo.PutUint16(entry[0:], 0x0112)
	bo.PutUint16(entry[2:], 3) // SHORT
	bo.PutUint32(entry[4:], 1)
	bo.PutUint16(entry[8:], o)
	tiff.Write(entry)
	tiff.Write([]byte{0, 0, 0, 0}) // next IFD

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)

	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

func testJPEG(t *testing.T) []byte {
	// 40x20: left half black, right half white.
	m := image.NewGray(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 20; x < 40; x++ {
			m.SetGray(x, y, color.Gray{255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, m, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOrientationParse(t *testing.T) {
	jpg := testJPEG(t)
	if o := jpegOrientation(jpg); o != 1 {
		t.Errorf("plain jpeg orientation %d", o)
	}
	for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		if o := jpegOrientation(withOrientation(jpg, 6, bo)); o != 6 {
			t.Errorf("%v: orientation %d, want 6", bo, o)
		}
	}
	if o := jpegOrientation([]byte("not a jpeg")); o != 1 {
		t.Errorf("garbage orientation %d", o)
	}
}

func TestDecodeRotates(t *testing.T) {
	data := withOrientation(testJPEG(t), 6, binary.BigEndian)
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("size %v, want 20x40", b.Size())
	}
	// Rotated 90° clockwise: the black left half ends up on top.
	top := color.GrayModel.Convert(img.At(10, 5)).(color.Gray).Y
	bottom := color.GrayModel.Convert(img.At(10, 35)).(color.Gray).Y
	if top > 40 || bottom < 215 {
		t.Errorf("top %d bottom %d, want dark top and light bottom", top, bottom)
	}
}

func TestThumbnail(t *testing.T) {
	m := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	th := Thumbnail(m, 100)
	if s := th.Bounds().Size(); s.X != 100 || s.Y != 50 {
		t.Errorf("thumbnail size %v", s)
	}
	if Thumbnail(m, 2000) != image.Image(m) {
		t.Error("small image should be returned unchanged")
	}
}
