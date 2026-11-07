package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	pbkdf2Iterations = 1000
	os.Exit(m.Run())
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "data", "cellmate.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func send(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	return send(h, httptest.NewRequest(method, path, nil))
}

func post(h http.Handler, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return send(h, req)
}

func getWith(h http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return send(h, req)
}

func signupForm(email, password string) url.Values {
	return url.Values{"email": {email}, "password": {password}, "consent": {"yes"}}
}

func sessionFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie, status %d", rec.Code)
	return nil
}

func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, to string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != to {
		t.Fatalf("want redirect to %s, got %d %q", to, rec.Code, rec.Header().Get("Location"))
	}
}

func signUp(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	rec := post(h, "/signup", signupForm(email, "hunter2hunter2"), nil)
	assertRedirect(t, rec, "/home")
	return sessionFrom(t, rec)
}

func TestLanding(t *testing.T) {
	rec := do(newHandler(newTestStore(t)), http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`href="/login"`, `href="/signup"`, `class="mugshots" aria-hidden="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestLoginPage(t *testing.T) {
	rec := do(newHandler(newTestStore(t)), http.MethodGet, "/login")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`type="email"`, `type="password"`, `href="/signup"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestSignupPage(t *testing.T) {
	rec := do(newHandler(newTestStore(t)), http.MethodGet, "/signup")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `name="consent"`) || !strings.Contains(body, `href="/login"`) {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newHandler(newTestStore(t))
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/"},
		{http.MethodPut, "/login"},
		{http.MethodGet, "/logout"},
	} {
		if got := do(h, c.method, c.path).Code; got != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status %d", c.method, c.path, got)
		}
	}
}

func TestStylesheet(t *testing.T) {
	rec := do(newHandler(newTestStore(t)), http.MethodGet, "/static/style.css")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestSignupCreatesUserAndSession(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "  Alice@Example.com ")
	u, err := store.UserForToken(c.Value)
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "alice@example.com" || u.ConsentAt.IsZero() || strings.Contains(u.PasswordHash, "hunter2") {
		t.Fatalf("bad user %+v", u)
	}
}

func TestSignupDuplicateEmail(t *testing.T) {
	h := newHandler(newTestStore(t))
	signUp(t, h, "bob@example.com")
	rec := post(h, "/signup", signupForm("BOB@example.com", "another-password"), nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSignupRejectsBadInput(t *testing.T) {
	h := newHandler(newTestStore(t))
	noConsent := signupForm("c@example.com", "longenough")
	noConsent.Del("consent")
	cases := map[string]url.Values{
		"missing consent": noConsent,
		"short password":  signupForm("c@example.com", "short"),
		"bad email":       signupForm("not-an-email", "longenough"),
	}
	for name, form := range cases {
		rec := post(h, "/signup", form, nil)
		if rec.Code != http.StatusBadRequest || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: status %d cookies %v", name, rec.Code, rec.Result().Cookies())
		}
	}
}

func TestLoginSetsCookie(t *testing.T) {
	h := newHandler(newTestStore(t))
	signUp(t, h, "dee@example.com")
	rec := post(h, "/login", url.Values{"email": {"DEE@example.com"}, "password": {"hunter2hunter2"}}, nil)
	assertRedirect(t, rec, "/home")
	c := sessionFrom(t, rec)
	setName(t, h, c, "Dee")
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 30*24*3600 || c.Secure {
		t.Fatalf("bad cookie %+v", c)
	}
	if got := getWith(h, "/home", c); got.Code != http.StatusOK {
		t.Fatalf("home status %d", got.Code)
	}
}

func TestLoginCookieSecureBehindProxy(t *testing.T) {
	h := newHandler(newTestStore(t))
	signUp(t, h, "eve@example.com")
	form := url.Values{"email": {"eve@example.com"}, "password": {"hunter2hunter2"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https")
	if c := sessionFrom(t, send(h, req)); !c.Secure {
		t.Fatal("cookie not Secure with X-Forwarded-Proto https")
	}
}

func TestLoginWrongCredentialsGenericError(t *testing.T) {
	h := newHandler(newTestStore(t))
	signUp(t, h, "fay@example.com")
	wrongPassword := post(h, "/login", url.Values{"email": {"fay@example.com"}, "password": {"wrong-password"}}, nil)
	unknownEmail := post(h, "/login", url.Values{"email": {"nobody@example.com"}, "password": {"hunter2hunter2"}}, nil)
	for _, rec := range []*httptest.ResponseRecorder{wrongPassword, unknownEmail} {
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Wrong email or password.") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	}
	if wrongPassword.Body.String() != strings.Replace(unknownEmail.Body.String(), "nobody@example.com", "fay@example.com", 1) {
		t.Error("error pages differ between wrong password and unknown email")
	}
}

func TestHomeRequiresSession(t *testing.T) {
	h := newHandler(newTestStore(t))
	assertRedirect(t, getWith(h, "/home", nil), "/login")
	assertRedirect(t, getWith(h, "/home", &http.Cookie{Name: sessionCookie, Value: "forged"}), "/login")

	c := signUp(t, h, "gus@example.com")
	setName(t, h, c, "Gus")
	rec := getWith(h, "/home", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Logged in as gus@example.com") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/", "/login", "/signup"} {
		assertRedirect(t, getWith(h, path, c), "/home")
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	h := newHandler(newTestStore(t))
	c := signUp(t, h, "hal@example.com")
	rec := post(h, "/logout", nil, c)
	assertRedirect(t, rec, "/")
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("cookie not cleared: %v", cleared)
	}
	assertRedirect(t, getWith(h, "/home", c), "/login")
}

func TestSessionPersistsAcrossReload(t *testing.T) {
	store := newTestStore(t)
	c := signUp(t, newHandler(store), "ivy@example.com")
	setName(t, newHandler(store), c, "Ivy")
	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), c.Value) {
		t.Fatal("raw session token written to disk")
	}
	reloaded, err := openStore(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if rec := getWith(newHandler(reloaded), "/home", c); rec.Code != http.StatusOK {
		t.Fatalf("status %d after reload", rec.Code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	c := signUp(t, h, "jo@example.com")
	store.now = func() time.Time { return time.Now().Add(sessionTTL + time.Minute) }
	assertRedirect(t, getWith(h, "/home", c), "/login")
}
