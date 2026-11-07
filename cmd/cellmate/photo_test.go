package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// jpegWithEXIF returns a JPEG with an APP1 Exif segment holding a GPS marker after SOI.
func jpegWithEXIF(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	payload := []byte("Exif\x00\x00GPSLatitude")
	n := len(payload) + 2
	seg := append([]byte{0xFF, 0xE1, byte(n >> 8), byte(n)}, payload...)
	raw := b.Bytes()
	return append(append(raw[:2:2], seg...), raw[2:]...)
}

func TestUploadStripsEXIF(t *testing.T) {
	in := jpegWithEXIF(t)
	if !bytes.Contains(in, []byte("GPSLatitude")) {
		t.Fatal("fixture lacks GPS marker")
	}
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "exif@example.com")
	rec := postProfile(t, h, c, map[string]string{"display_name": "Ex"}, &upload{"a.jpg", in})
	assertRedirect(t, rec, "/home")
	out := getWith(h, "/photos/"+userFor(t, store, c).Profile.PhotoID, c)
	if out.Code != http.StatusOK || out.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("status %d type %q", out.Code, out.Header().Get("Content-Type"))
	}
	body := out.Body.Bytes()
	if bytes.Contains(body, []byte("Exif")) || bytes.Contains(body, []byte("GPSLatitude")) || bytes.Contains(body, []byte{0xFF, 0xE1}) {
		t.Fatal("saved photo still has EXIF")
	}
	if _, err := jpeg.Decode(bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
}

func TestCleanPhotoPNGRoundTrip(t *testing.T) {
	out, err := cleanPhoto(pngBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil || img.Bounds().Dx() != 2 {
		t.Fatalf("decode: %v", err)
	}
}

func TestCleanPhotoRejectsOversizedDimensions(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, maxPhotoSide+1, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanPhoto(b.Bytes()); err != errInvalidPhoto {
		t.Fatalf("got %v", err)
	}
}

func TestCleanPhotoRejectsCorruptJPEG(t *testing.T) {
	in := jpegWithEXIF(t)
	in = in[:len(in)/2]
	if _, err := cleanPhoto(in); err != errInvalidPhoto {
		t.Fatalf("got %v", err)
	}
	h := newHandler(newTestStore(t))
	c := signUp(t, h, "bad@example.com")
	rec := postProfile(t, h, c, map[string]string{"display_name": "Bad"}, &upload{"a.jpg", in})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "JPEG or PNG") {
		t.Fatalf("status %d", rec.Code)
	}
}
