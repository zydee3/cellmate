package main

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

const (
	maxPhotoBytes = 5 << 20
	// Caps decoded size so a tiny compressed file cannot expand to gigabytes.
	maxPhotoSide = 8000
	jpegQuality  = 90
)

var errInvalidPhoto = errors.New("invalid image")

// Photo ids are 32 hex chars from randomString(16). Checking the shape keeps ids from naming paths outside photoDir.
var photoIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// isAllowedPhoto reports whether data starts with JPEG or PNG magic bytes.
func isAllowedPhoto(data []byte) bool {
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png":
		return true
	}
	return false
}

// cleanPhoto re-encodes a JPEG or PNG so EXIF and other metadata, including GPS, are dropped.
func cleanPhoto(data []byte) ([]byte, error) {
	if !isAllowedPhoto(data) {
		return nil, errInvalidPhoto
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width > maxPhotoSide || cfg.Height > maxPhotoSide {
		return nil, errInvalidPhoto
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errInvalidPhoto
	}
	var out bytes.Buffer
	if format == "jpeg" {
		img = applyOrientation(img, jpegOrientation(data))
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality})
	} else {
		err = png.Encode(&out, img)
	}
	return out.Bytes(), err
}

// SavePhoto writes data under a new random id and returns the id.
func (s *Store) SavePhoto(data []byte) (string, error) {
	id, err := randomString(16)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.photoDir, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return id, os.Rename(f.Name(), s.PhotoPath(id))
}

// PhotoPath returns the file path for a photo id. The caller checks the id shape.
func (s *Store) PhotoPath(id string) string {
	return filepath.Join(s.photoDir, id)
}

// DeletePhoto removes a photo file. An empty or missing id is not an error.
func (s *Store) DeletePhoto(id string) error {
	if id == "" {
		return nil
	}
	err := os.Remove(s.PhotoPath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *server) photo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(r); !ok {
		http.Error(w, "log in to see photos", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if !photoIDPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(s.store.PhotoPath(id))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}
