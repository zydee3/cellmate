package main

import (
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	_ "time/tzdata"
)

//go:embed web
var webFS embed.FS

var pages = template.Must(template.ParseFS(webFS, "web/*.html"))

const sessionCookie = "session"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dataFile := os.Getenv("DATA_FILE")
	if dataFile == "" {
		dataFile = "data/cellmate.json"
	}
	store, err := openStore(dataFile)
	if err != nil {
		log.Fatalf("open %s: %v", dataFile, err)
	}
	addr := ":" + port
	log.Printf("listening on %s, data in %s", addr, dataFile)
	log.Fatal(http.ListenAndServe(addr, newHandler(store)))
}

type server struct {
	store *Store
}

type formData struct {
	Email string
	Error string
}

func newHandler(store *Store) http.Handler {
	s := &server{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.guestOnly("landing.html"))
	mux.HandleFunc("GET /login", s.guestOnly("login.html"))
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("GET /signup", s.guestOnly("signup.html"))
	mux.HandleFunc("POST /signup", s.signup)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /home", s.home)
	mux.HandleFunc("GET /profile", s.editProfile)
	mux.HandleFunc("POST /profile", s.saveProfile)
	mux.HandleFunc("GET /photos/{id}", s.photo)
	mux.HandleFunc("POST /lineup/pick", s.pick)
	mux.HandleFunc("GET /static/style.css", style)
	return mux
}

func render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// currentUser returns the logged-in user, or false when the request has no valid session.
func (s *server) currentUser(r *http.Request) (User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return User{}, false
	}
	u, err := s.store.UserForToken(c.Value)
	return u, err == nil
}

// guestOnly renders a page for visitors and sends logged-in users to /home.
func (s *server) guestOnly(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.currentUser(r); ok {
			http.Redirect(w, r, "/home", http.StatusSeeOther)
			return
		}
		render(w, http.StatusOK, name, formData{})
	}
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if u.Profile.DisplayName == "" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	s.renderHome(w, http.StatusOK, u, "")
}

func (s *server) signup(w http.ResponseWriter, r *http.Request) {
	email := normalizeEmail(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	fail := func(status int, msg string) {
		render(w, status, "signup.html", formData{Email: email, Error: msg})
	}
	switch {
	case !strings.Contains(email, "@"):
		fail(http.StatusBadRequest, "Enter a valid email address.")
		return
	case len(password) < 8:
		fail(http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	case r.PostFormValue("consent") == "":
		fail(http.StatusBadRequest, "You must confirm the age and legal statement to create an account.")
		return
	}
	u, err := s.store.CreateUser(email, password)
	if errors.Is(err, errEmailTaken) {
		fail(http.StatusConflict, "An account with this email already exists. Log in instead.")
		return
	}
	if err != nil {
		log.Printf("create user: %v", err)
		fail(http.StatusInternalServerError, "Could not create the account. Try again.")
		return
	}
	s.startSession(w, r, u.ID)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	email := normalizeEmail(r.PostFormValue("email"))
	u, err := s.store.Authenticate(email, r.PostFormValue("password"))
	if err != nil {
		render(w, http.StatusUnauthorized, "login.html", formData{Email: email, Error: "Wrong email or password."})
		return
	}
	s.startSession(w, r, u.ID)
}

// startSession sets the session cookie and redirects to /home.
func (s *server) startSession(w http.ResponseWriter, r *http.Request, userID string) {
	token, err := s.store.CreateSession(userID)
	if err != nil {
		log.Printf("create session: %v", err)
		http.Error(w, "could not start a session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

// isHTTPS covers direct TLS and TLS ended at the ingress proxy.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.store.DeleteSession(c.Value); err != nil {
			log.Printf("delete session: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func style(w http.ResponseWriter, r *http.Request) {
	b, err := webFS.ReadFile("web/style.css")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Write(b)
}
