package main

import (
	"errors"
	"log"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"
)

const (
	lineupSize = 5
	lineupZone = "America/Los_Angeles"
)

var (
	errNotInLineup   = errors.New("member not in today's lineup")
	errAlreadyPicked = errors.New("already picked today")
)

// Lineup is one user's daily set of members and the one they picked, if any.
type Lineup struct {
	Day       string   `json:"day"`
	MemberIDs []string `json:"member_ids"`
	PickedID  string   `json:"picked_id,omitempty"`
}

// Match is a pair of users who picked each other. Users is sorted so each pair is stored once.
type Match struct {
	Users     [2]string `json:"users"`
	CreatedAt time.Time `json:"created_at"`
}

// Member is another user as shown in a lineup or match list.
type Member struct {
	ID      string
	Profile Profile
}

// today returns the current lineup day as YYYY-MM-DD in lineupZone.
func (s *Store) today() string {
	return s.now().In(s.loc).Format(time.DateOnly)
}

// TodayLineup returns userID's lineup for today, generating and saving it on the first call of the day.
func (s *Store) TodayLineup(userID string) (Lineup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := s.today()
	old, had := s.data.Lineups[userID]
	if had && old.Day == day {
		return old, nil
	}
	l := Lineup{Day: day, MemberIDs: s.pickCandidates(userID)}
	s.data.Lineups[userID] = l
	if err := s.save(); err != nil {
		if had {
			s.data.Lineups[userID] = old
		} else {
			delete(s.data.Lineups, userID)
		}
		return Lineup{}, err
	}
	return l, nil
}

// pickCandidates returns up to lineupSize random users with a display name, excluding userID, its picks, and its matches.
func (s *Store) pickCandidates(userID string) []string {
	excluded := map[string]bool{userID: true}
	for _, id := range s.data.Picks[userID] {
		excluded[id] = true
	}
	for _, id := range s.matchIDs(userID) {
		excluded[id] = true
	}
	var ids []string
	for _, u := range s.data.Users {
		if u.Profile.DisplayName != "" && !excluded[u.ID] {
			ids = append(ids, u.ID)
		}
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids[:min(len(ids), lineupSize)]
}

// matchIDs returns the ids of users matched with userID, oldest match first.
func (s *Store) matchIDs(userID string) []string {
	var ids []string
	for _, m := range s.data.Matches {
		switch userID {
		case m.Users[0]:
			ids = append(ids, m.Users[1])
		case m.Users[1]:
			ids = append(ids, m.Users[0])
		}
	}
	return ids
}

// Pick records userID's pick of memberID from today's lineup and creates a match when the pick is mutual.
func (s *Store) Pick(userID, memberID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.data.Lineups[userID]
	if !ok || l.Day != s.today() || !slices.Contains(l.MemberIDs, memberID) {
		return errNotInLineup
	}
	if l.PickedID != "" {
		return errAlreadyPicked
	}
	oldPicks, oldMatches := s.data.Picks[userID], s.data.Matches
	picked := l
	picked.PickedID = memberID
	s.data.Lineups[userID] = picked
	s.data.Picks[userID] = append(slices.Clip(oldPicks), memberID)
	if slices.Contains(s.data.Picks[memberID], userID) {
		s.addMatch(userID, memberID)
	}
	if err := s.save(); err != nil {
		s.data.Lineups[userID] = l
		s.data.Picks[userID] = oldPicks
		s.data.Matches = oldMatches
		return err
	}
	return nil
}

// addMatch stores the pair once, whatever order the ids come in.
func (s *Store) addMatch(a, b string) {
	pair := [2]string{min(a, b), max(a, b)}
	for _, m := range s.data.Matches {
		if m.Users == pair {
			return
		}
	}
	s.data.Matches = append(slices.Clip(s.data.Matches), Match{Users: pair, CreatedAt: s.now().UTC()})
}

// Members returns the profiles for ids, skipping ids with no user.
func (s *Store) Members(ids []string) []Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.members(ids)
}

func (s *Store) members(ids []string) []Member {
	var out []Member
	for _, id := range ids {
		if u := s.userByID(id); u != nil {
			out = append(out, Member{ID: u.ID, Profile: u.Profile})
		}
	}
	return out
}

// Matches returns the members matched with userID, oldest match first.
func (s *Store) Matches(userID string) []Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.members(s.matchIDs(userID))
}

// suspect is a lineup member on /home. CanPick is false once today's pick is made.
type suspect struct {
	Member
	CanPick bool
}

type homeView struct {
	User    User
	Lineup  []suspect
	Picked  *suspect
	Matches []Member
	Error   string
}

// homeData builds the /home view for u from today's lineup and u's matches.
func (s *server) homeData(u User) (homeView, error) {
	l, err := s.store.TodayLineup(u.ID)
	if err != nil {
		return homeView{}, err
	}
	v := homeView{User: u, Matches: s.store.Matches(u.ID)}
	if l.PickedID == "" {
		for _, m := range s.store.Members(l.MemberIDs) {
			v.Lineup = append(v.Lineup, suspect{Member: m, CanPick: true})
		}
		return v, nil
	}
	if p := s.store.Members([]string{l.PickedID}); len(p) == 1 {
		v.Picked = &suspect{Member: p[0]}
	}
	return v, nil
}

func (s *server) renderHome(w http.ResponseWriter, status int, u User, msg string) {
	v, err := s.homeData(u)
	if err != nil {
		log.Printf("lineup: %v", err)
		http.Error(w, "could not load the lineup", http.StatusInternalServerError)
		return
	}
	v.Error = msg
	render(w, status, "home.html", v)
}

func (s *server) pick(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if u.Profile.DisplayName == "" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	err := s.store.Pick(u.ID, r.PostFormValue("member"))
	switch {
	case errors.Is(err, errNotInLineup):
		s.renderHome(w, http.StatusBadRequest, u, "That member is not in today's lineup.")
	case errors.Is(err, errAlreadyPicked):
		s.renderHome(w, http.StatusConflict, u, "You already picked someone today. A new lineup comes at midnight Pacific.")
	case err != nil:
		log.Printf("pick: %v", err)
		s.renderHome(w, http.StatusInternalServerError, u, "Could not save the pick. Try again.")
	default:
		http.Redirect(w, r, "/home", http.StatusSeeOther)
	}
}
