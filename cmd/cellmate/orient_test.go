package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// exifSegment builds an APP1 segment with one Orientation entry in the given byte order.
func exifSegment(order binary.AppendByteOrder, tag string, o uint16) []byte {
	t := []byte(tag)
	t = order.AppendUint16(t, 42)
	t = order.AppendUint32(t, 8)
	t = order.AppendUint16(t, 1)
	t = order.AppendUint16(t, exifOrientationTag)
	t = order.AppendUint16(t, tiffTypeShort)
	t = order.AppendUint32(t, 1)
	t = order.AppendUint16(t, o)
	t = append(t, 0, 0)
	t = order.AppendUint32(t, 0)
	payload := append([]byte("Exif\x00\x00"), t...)
	n := len(payload) + 2
	return append([]byte{0xFF, 0xE1, byte(n >> 8), byte(n)}, payload...)
}

func withSegment(t *testing.T, img image.Image, seg []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	return append(append(raw[:2:2], seg...), raw[2:]...)
}

func TestJPEGOrientationByteOrders(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for _, tc := range []struct {
		order binary.AppendByteOrder
		tag   string
	}{{binary.LittleEndian, "II"}, {binary.BigEndian, "MM"}} {
		for o := uint16(1); o <= 8; o++ {
			got := jpegOrientation(withSegment(t, img, exifSegment(tc.order, tc.tag, o)))
			if got != int(o) {
				t.Fatalf("%s orientation %d: got %d", tc.tag, o, got)
			}
		}
	}
}

func TestJPEGOrientationMalformed(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	good := exifSegment(binary.BigEndian, "MM", 6)
	cases := map[string][]byte{
		"empty":       nil,
		"not jpeg":    []byte("hello world"),
		"soi only":    {0xFF, 0xD8},
		"bad value":   withSegment(t, img, exifSegment(binary.BigEndian, "MM", 9)),
		"zero value":  withSegment(t, img, exifSegment(binary.LittleEndian, "II", 0)),
		"no exif":     withSegment(t, img, nil),
		"bad order":   withSegment(t, img, exifSegment(binary.BigEndian, "XX", 6)),
		"huge length": append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF}, good[4:]...),
	}
	for cut := 3; cut < len(good); cut++ {
		cases["truncated"+string(rune('A'+cut%26))+string(rune('a'+cut/26))] = append([]byte{0xFF, 0xD8}, good[:cut]...)
	}
	for name, data := range cases {
		if got := jpegOrientation(data); got != 1 {
			t.Fatalf("%s: got %d want 1", name, got)
		}
	}
}

func TestJPEGOrientationBadIFDOffset(t *testing.T) {
	seg := exifSegment(binary.BigEndian, "MM", 6)
	binary.BigEndian.PutUint32(seg[4+6+4:], 0xFFFFFFF0)
	if got := jpegOrientation(append([]byte{0xFF, 0xD8}, seg...)); got != 1 {
		t.Fatalf("got %d", got)
	}
}

// Source 2x3 (w x h), row-major ids:
//
//	1 2
//	3 4
//	5 6
func orientFixture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	for i := 0; i < 6; i++ {
		img.SetNRGBA(i%2, i/2, color.NRGBA{R: uint8(i + 1), A: 255})
	}
	return img
}

func layout(img image.Image) [][]uint8 {
	b := img.Bounds()
	rows := make([][]uint8, b.Dy())
	for y := range rows {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			rows[y] = append(rows[y], c.R)
		}
	}
	return rows
}

func TestApplyOrientation(t *testing.T) {
	want := map[int][][]uint8{
		1: {{1, 2}, {3, 4}, {5, 6}},
		2: {{2, 1}, {4, 3}, {6, 5}},
		3: {{6, 5}, {4, 3}, {2, 1}},
		4: {{5, 6}, {3, 4}, {1, 2}},
		5: {{1, 3, 5}, {2, 4, 6}},
		6: {{5, 3, 1}, {6, 4, 2}},
		7: {{6, 4, 2}, {5, 3, 1}},
		8: {{2, 4, 6}, {1, 3, 5}},
	}
	for o, rows := range want {
		got := layout(applyOrientation(orientFixture(), o))
		if len(got) != len(rows) {
			t.Fatalf("orientation %d: got %v want %v", o, got, rows)
		}
		for y := range rows {
			if !bytes.Equal(got[y], rows[y]) {
				t.Fatalf("orientation %d: got %v want %v", o, got, rows)
			}
		}
	}
}

func TestCleanPhotoAppliesOrientation(t *testing.T) {
	in := withSegment(t, image.NewRGBA(image.Rect(0, 0, 16, 8)), exifSegment(binary.BigEndian, "MM", 6))
	out, err := cleanPhoto(in)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 8 || cfg.Height != 16 {
		t.Fatalf("size %dx%d err %v", cfg.Width, cfg.Height, err)
	}
	if bytes.Contains(out, []byte("Exif")) || bytes.Contains(out, []byte{0xFF, 0xE1}) {
		t.Fatal("output still has Exif")
	}
}
