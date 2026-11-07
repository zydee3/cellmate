package main

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type upload struct {
	name string
	data []byte
}

func postProfile(t *testing.T, h http.Handler, c *http.Cookie, fields map[string]string, photo *upload) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if photo != nil {
		fw, err := mw.CreateFormFile("photo", photo.name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(photo.data)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/profile", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c != nil {
		req.AddCookie(c)
	}
	return send(h, req)
}

func setName(t *testing.T, h http.Handler, c *http.Cookie, name string) {
	t.Helper()
	assertRedirect(t, postProfile(t, h, c, map[string]string{"display_name": name}, nil), "/home")
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func userFor(t *testing.T, store *Store, c *http.Cookie) User {
	t.Helper()
	u, err := store.UserForToken(c.Value)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestHomeWithoutNameRedirectsToProfile(t *testing.T) {
	h := newHandler(newTestStore(t))
	c := signUp(t, h, "ann@example.com")
	assertRedirect(t, getWith(h, "/home", c), "/profile")
	rec := getWith(h, "/profile", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `name="display_name"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	assertRedirect(t, getWith(h, "/profile", nil), "/login")
}

func TestSaveValidProfile(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "ben@example.com")
	fields := map[string]string{"display_name": " Ben ", "bio": "Likes <b>dogs</b>", "convictions": "Grand theft auto, 2019"}
	assertRedirect(t, postProfile(t, h, c, fields, &upload{"me.txt", pngBytes(t)}), "/home")

	p := userFor(t, store, c).Profile
	if p.DisplayName != "Ben" || p.Bio != "Likes <b>dogs</b>" || p.Convictions != "Grand theft auto, 2019" || !photoIDPattern.MatchString(p.PhotoID) {
		t.Fatalf("bad profile %+v", p)
	}
	if _, err := os.Stat(store.PhotoPath(p.PhotoID)); err != nil {
		t.Fatal(err)
	}
	home := getWith(h, "/home", c).Body.String()
	for _, want := range []string{"Ben", "Likes &lt;b&gt;dogs&lt;/b&gt;", "Grand theft auto, 2019", `src="/photos/` + p.PhotoID, `href="/profile"`, `action="/logout"`} {
		if !strings.Contains(home, want) {
			t.Errorf("home missing %s", want)
		}
	}
	photo := getWith(h, "/photos/"+p.PhotoID, c)
	if photo.Code != http.StatusOK || photo.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("photo status %d type %q", photo.Code, photo.Header().Get("Content-Type"))
	}
	edit := getWith(h, "/profile", c).Body.String()
	if !strings.Contains(edit, `value="Ben"`) || !strings.Contains(edit, "Grand theft auto, 2019") {
		t.Errorf("edit form missing current values: %s", edit)
	}
}

func TestReplacingPhotoDeletesOld(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "cal@example.com")
	postProfile(t, h, c, map[string]string{"display_name": "Cal"}, &upload{"a.png", pngBytes(t)})
	first := userFor(t, store, c).Profile.PhotoID
	postProfile(t, h, c, map[string]string{"display_name": "Cal"}, &upload{"b.png", pngBytes(t)})
	second := userFor(t, store, c).Profile.PhotoID
	if first == second {
		t.Fatal("photo id not replaced")
	}
	if _, err := os.Stat(store.PhotoPath(first)); !os.IsNotExist(err) {
		t.Fatalf("old photo still on disk: %v", err)
	}
	setName(t, h, c, "Cal")
	if userFor(t, store, c).Profile.PhotoID != second {
		t.Fatal("saving without a file dropped the photo")
	}
}

func TestProfileValidationErrors(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "dot@example.com")
	cases := []struct {
		name   string
		fields map[string]string
		photo  *upload
		status int
		msg    string
	}{
		{"name required", map[string]string{"display_name": "   ", "bio": "kept"}, nil, http.StatusBadRequest, "Display name is required."},
		{"name too long", map[string]string{"display_name": strings.Repeat("a", 41)}, nil, http.StatusBadRequest, "Display name must be 40"},
		{"bio too long", map[string]string{"display_name": "Dot", "bio": strings.Repeat("é", 501)}, nil, http.StatusBadRequest, "Bio must be 500"},
		{"convictions too long", map[string]string{"display_name": "Dot", "convictions": strings.Repeat("x", 501)}, nil, http.StatusBadRequest, "Conviction history must be 500"},
		{"not an image", map[string]string{"display_name": "Dot"}, &upload{"evil.png", []byte("<html><script>alert(1)</script></html>")}, http.StatusBadRequest, "JPEG or PNG"},
		{"gif rejected", map[string]string{"display_name": "Dot"}, &upload{"a.jpg", []byte("GIF89a\x01\x00\x01\x00")}, http.StatusBadRequest, "JPEG or PNG"},
		{"over 5 MB", map[string]string{"display_name": "Dot"}, &upload{"big.png", append(pngBytes(t), make([]byte, maxPhotoBytes)...)}, http.StatusRequestEntityTooLarge, "5 MB or smaller"},
		{"far over 5 MB", map[string]string{"display_name": "Dot"}, &upload{"huge.png", make([]byte, maxProfileBody+1)}, http.StatusRequestEntityTooLarge, "5 MB or smaller"},
	}
	for _, tc := range cases {
		rec := postProfile(t, h, c, tc.fields, tc.photo)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.msg) || !strings.Contains(rec.Body.String(), `name="display_name"`) {
			t.Errorf("%s: status %d body %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	if p := userFor(t, store, c).Profile; p != (Profile{}) {
		t.Fatalf("invalid input saved: %+v", p)
	}
	entries, _ := os.ReadDir(store.photoDir)
	if len(entries) != 0 {
		t.Fatalf("rejected uploads left files: %v", entries)
	}
	if body := postProfile(t, h, c, map[string]string{"display_name": "", "bio": "kept"}, nil).Body.String(); !strings.Contains(body, ">kept</textarea>") {
		t.Error("form did not re-show submitted bio")
	}
}

func TestPhotoRequiresLogin(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "eli@example.com")
	postProfile(t, h, c, map[string]string{"display_name": "Eli"}, &upload{"a.png", pngBytes(t)})
	id := userFor(t, store, c).Profile.PhotoID
	if rec := getWith(h, "/photos/"+id, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("logged out: status %d", rec.Code)
	}
	other := signUp(t, h, "fin@example.com")
	if rec := getWith(h, "/photos/"+id, other); rec.Code != http.StatusOK {
		t.Fatalf("other member: status %d", rec.Code)
	}
	for _, bad := range []string{"..%2Fcellmate.json", "nothex", strings.Repeat("0", 32)} {
		if rec := getWith(h, "/photos/"+bad, c); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", bad, rec.Code)
		}
	}
}

func TestCannotEditAnotherUsersProfile(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	victim := signUp(t, h, "gil@example.com")
	setName(t, h, victim, "Gil")
	attacker := signUp(t, h, "hex@example.com")
	victimID := userFor(t, store, victim).ID
	fields := map[string]string{"display_name": "Hacked", "id": victimID, "user_id": victimID}
	assertRedirect(t, postProfile(t, h, attacker, fields, nil), "/home")
	if got := userFor(t, store, victim).Profile.DisplayName; got != "Gil" {
		t.Fatalf("victim profile changed to %q", got)
	}
	if got := userFor(t, store, attacker).Profile.DisplayName; got != "Hacked" {
		t.Fatalf("attacker profile %q", got)
	}
	assertRedirect(t, postProfile(t, h, nil, fields, nil), "/login")
	if got := userFor(t, store, victim).Profile.DisplayName; got != "Gil" {
		t.Fatalf("logged-out post changed victim to %q", got)
	}
}
