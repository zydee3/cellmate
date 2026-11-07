package main

import (
	"encoding/binary"
	"image"
	"image/color"
)

const (
	exifOrientationTag = 0x0112
	tiffTypeShort      = 3
	ifdEntrySize       = 12
)

var exifHeader = []byte("Exif\x00\x00")

// jpegOrientation returns the EXIF Orientation (1 to 8) of a JPEG, or 1 when absent or malformed.
func jpegOrientation(data []byte) int {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	pos := 2
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			return 1
		}
		marker := data[pos+1]
		if marker == 0xFF {
			pos++
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			pos += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(data[pos+2:]))
		end := pos + 2 + size
		if size < 2 || end > len(data) {
			return 1
		}
		seg := data[pos+4 : end]
		if marker == 0xE1 && len(seg) > len(exifHeader) && string(seg[:len(exifHeader)]) == string(exifHeader) {
			return tiffOrientation(seg[len(exifHeader):])
		}
		pos = end
	}
	return 1
}

// tiffOrientation reads tag 0x0112 from IFD0 of a TIFF block, checking every offset against len(t).
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(t[4:]))
	if ifd < 8 || ifd > len(t)-2 {
		return 1
	}
	count := int(order.Uint16(t[ifd:]))
	for i := 0; i < count; i++ {
		e := ifd + 2 + i*ifdEntrySize
		if e+ifdEntrySize > len(t) {
			return 1
		}
		if order.Uint16(t[e:]) != exifOrientationTag {
			continue
		}
		if order.Uint16(t[e+2:]) != tiffTypeShort {
			return 1
		}
		if v := int(order.Uint16(t[e+8:])); v >= 1 && v <= 8 {
			return v
		}
		return 1
	}
	return 1
}

// applyOrientation returns img upright for EXIF orientation o. Orientation 1 or an unknown value returns img unchanged.
func applyOrientation(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := orientSource(o, x, y, w, h)
			out.Set(x, y, color.NRGBAModel.Convert(img.At(b.Min.X+sx, b.Min.Y+sy)))
		}
	}
	return out
}

// orientSource maps a destination pixel to its source pixel for orientation o on a w by h source.
func orientSource(o, x, y, w, h int) (int, int) {
	switch o {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return y, h - 1 - x
	case 7:
		return w - 1 - y, h - 1 - x
	default:
		return w - 1 - y, x
	}
}
