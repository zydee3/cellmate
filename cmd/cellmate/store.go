package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const sessionTTL = 30 * 24 * time.Hour

var (
	errEmailTaken  = errors.New("email already registered")
	errBadLogin    = errors.New("wrong email or password")
	errNoSession   = errors.New("no valid session")
	errUserMissing = errors.New("user not found")
)

// User is a registered account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
	ConsentAt    time.Time `json:"consent_at"`
	Profile      Profile   `json:"profile"`
}

// Profile is what other members see. An empty DisplayName means the user has not set it up yet.
type Profile struct {
	DisplayName string `json:"display_name"`
	PhotoID     string `json:"photo_id,omitempty"`
	Bio         string `json:"bio,omitempty"`
	Convictions string `json:"convictions,omitempty"`
}

// Session maps a hashed token to a user until it expires.
type Session struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type storeData struct {
	Users []User `json:"users"`
	// Keyed by hex SHA-256 of the cookie token, so a leaked file holds no usable tokens.
	Sessions map[string]Session `json:"sessions"`
	// Keyed by user id. Holds only the user's latest lineup.
	Lineups map[string]Lineup `json:"lineups"`
	// Keyed by user id. Every member the user has ever picked.
	Picks   map[string][]string `json:"picks"`
	Matches []Match             `json:"matches"`
}

// Store is the JSON file database for users and sessions.
type Store struct {
	mu       sync.Mutex
	path     string
	photoDir string
	data     storeData
	now      func() time.Time
	// Lineup days roll over at midnight in this zone.
	loc *time.Location
}

// openStore loads the JSON file at path, creating it if missing.
func openStore(path string) (*Store, error) {
	photoDir := filepath.Join(filepath.Dir(path), "photos")
	loc, err := time.LoadLocation(lineupZone)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, photoDir: photoDir, now: time.Now, loc: loc}
	s.data.initMaps()
	if err := os.MkdirAll(photoDir, 0o700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, s.save()
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	s.data.initMaps()
	return s, nil
}

// initMaps fills maps that are missing from a new or older data file.
func (d *storeData) initMaps() {
	if d.Sessions == nil {
		d.Sessions = map[string]Session{}
	}
	if d.Lineups == nil {
		d.Lineups = map[string]Lineup{}
	}
	if d.Picks == nil {
		d.Picks = map[string][]string{}
	}
}

// save writes the data to a temp file and renames it so a crash never leaves a partial file.
func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".cellmate-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Store) userByEmail(email string) *User {
	for i := range s.data.Users {
		if s.data.Users[i].Email == email {
			return &s.data.Users[i]
		}
	}
	return nil
}

func (s *Store) userByID(id string) *User {
	for i := range s.data.Users {
		if s.data.Users[i].ID == id {
			return &s.data.Users[i]
		}
	}
	return nil
}

// CreateUser registers a new account. It returns errEmailTaken for a duplicate email.
func (s *Store) CreateUser(email, password string) (User, error) {
	email = normalizeEmail(email)
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	id, err := randomString(16)
	if err != nil {
		return User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userByEmail(email) != nil {
		return User{}, errEmailTaken
	}
	now := s.now().UTC()
	u := User{ID: id, Email: email, PasswordHash: hash, CreatedAt: now, ConsentAt: now}
	s.data.Users = append(s.data.Users, u)
	if err := s.save(); err != nil {
		s.data.Users = s.data.Users[:len(s.data.Users)-1]
		return User{}, err
	}
	return u, nil
}

// UpdateProfile replaces the profile of userID and returns the previous one.
func (s *Store) UpdateProfile(userID string, p Profile) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.userByID(userID)
	if u == nil {
		return Profile{}, errUserMissing
	}
	old := u.Profile
	u.Profile = p
	if err := s.save(); err != nil {
		u.Profile = old
		return Profile{}, err
	}
	return old, nil
}

// Authenticate returns the user for a correct email and password, or errBadLogin.
func (s *Store) Authenticate(email, password string) (User, error) {
	s.mu.Lock()
	u := s.userByEmail(normalizeEmail(email))
	var found User
	if u != nil {
		found = *u
	}
	s.mu.Unlock()
	if u == nil {
		// Hash anyway so response time does not reveal whether the email exists.
		checkPassword(dummyHash(), password)
		return User{}, errBadLogin
	}
	if !checkPassword(found.PasswordHash, password) {
		return User{}, errBadLogin
	}
	return found, nil
}

// CreateSession stores a new session for userID and returns the raw cookie token.
func (s *Store) CreateSession(userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, sess := range s.data.Sessions {
		if !now.Before(sess.ExpiresAt) {
			delete(s.data.Sessions, k)
		}
	}
	key := tokenKey(token)
	s.data.Sessions[key] = Session{UserID: userID, ExpiresAt: now.Add(sessionTTL).UTC()}
	if err := s.save(); err != nil {
		delete(s.data.Sessions, key)
		return "", err
	}
	return token, nil
}

// UserForToken returns the user owning an unexpired session token.
func (s *Store) UserForToken(token string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.data.Sessions[tokenKey(token)]
	if !ok || !s.now().Before(sess.ExpiresAt) {
		return User{}, errNoSession
	}
	u := s.userByID(sess.UserID)
	if u == nil {
		return User{}, errUserMissing
	}
	return *u, nil
}

// DeleteSession removes the session for token. A missing session is not an error.
func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tokenKey(token)
	if _, ok := s.data.Sessions[key]; !ok {
		return nil
	}
	delete(s.data.Sessions, key)
	return s.save()
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
