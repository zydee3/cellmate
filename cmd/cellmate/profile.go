package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	maxNameRunes = 40
	maxTextRunes = 500
	// Room for the text fields and multipart framing on top of the photo.
	maxProfileBody = maxPhotoBytes + 1<<20
)

var errPhotoTooBig = errors.New("photo too big")

type profileView struct {
	Profile Profile
	Error   string
}

func (s *server) editProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	render(w, http.StatusOK, "profile.html", profileView{Profile: u.Profile})
}

// saveProfile updates only the session user's profile. Any id in the form is ignored.
func (s *server) saveProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBody)
	p := Profile{PhotoID: u.Profile.PhotoID}
	fail := func(status int, msg string) {
		render(w, status, "profile.html", profileView{Profile: p, Error: msg})
	}
	photo, status, msg := parseProfileForm(r, &p)
	if msg != "" {
		fail(status, msg)
		return
	}
	if err := s.storeProfile(u.ID, p, photo); err != nil {
		log.Printf("save profile: %v", err)
		fail(http.StatusInternalServerError, "Could not save the profile. Try again.")
		return
	}
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

// parseProfileForm fills the text fields of p and returns the photo bytes, or an HTTP status and message on bad input.
func parseProfileForm(r *http.Request, p *Profile) ([]byte, int, string) {
	err := r.ParseMultipartForm(1 << 20)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return nil, http.StatusRequestEntityTooLarge, "Photo must be 5 MB or smaller."
	}
	if err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return nil, http.StatusBadRequest, "Could not read the form. Try again."
	}
	p.DisplayName = strings.TrimSpace(r.PostFormValue("display_name"))
	p.Bio = strings.TrimSpace(r.PostFormValue("bio"))
	p.Convictions = strings.TrimSpace(r.PostFormValue("convictions"))
	if msg := validateProfile(*p); msg != "" {
		return nil, http.StatusBadRequest, msg
	}
	photo, err := readPhoto(r)
	if errors.Is(err, errPhotoTooBig) {
		return nil, http.StatusRequestEntityTooLarge, "Photo must be 5 MB or smaller."
	}
	if err != nil {
		return nil, http.StatusBadRequest, "Could not read the photo. Try again."
	}
	if photo != nil {
		photo, err = cleanPhoto(photo)
		if err != nil {
			return nil, http.StatusBadRequest, "Photo must be a JPEG or PNG."
		}
	}
	return photo, 0, ""
}

// validateProfile returns a message for the first invalid text field, or "" when all are valid.
func validateProfile(p Profile) string {
	switch {
	case p.DisplayName == "":
		return "Display name is required."
	case utf8.RuneCountInString(p.DisplayName) > maxNameRunes:
		return "Display name must be 40 characters or fewer."
	case utf8.RuneCountInString(p.Bio) > maxTextRunes:
		return "Bio must be 500 characters or fewer."
	case utf8.RuneCountInString(p.Convictions) > maxTextRunes:
		return "Conviction history must be 500 characters or fewer."
	}
	return ""
}

// readPhoto returns the uploaded photo bytes, or nil when no file was chosen.
func readPhoto(r *http.Request) ([]byte, error) {
	f, _, err := r.FormFile("photo")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPhotoBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPhotoBytes {
		return nil, errPhotoTooBig
	}
	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

// storeProfile saves a new photo if given, updates the profile, and removes the replaced photo.
func (s *server) storeProfile(userID string, p Profile, photo []byte) error {
	if photo != nil {
		id, err := s.store.SavePhoto(photo)
		if err != nil {
			return err
		}
		p.PhotoID = id
	}
	old, err := s.store.UpdateProfile(userID, p)
	if err != nil {
		if photo != nil {
			s.store.DeletePhoto(p.PhotoID)
		}
		return err
	}
	if old.PhotoID != p.PhotoID {
		if err := s.store.DeletePhoto(old.PhotoID); err != nil {
			log.Printf("delete old photo: %v", err)
		}
	}
	return nil
}
