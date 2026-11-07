package main

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// namedUsers signs up n users with display names and returns their cookies and ids.
func namedUsers(t *testing.T, store *Store, h http.Handler, n int) ([]*http.Cookie, []string) {
	t.Helper()
	var cookies []*http.Cookie
	var ids []string
	for i := range n {
		c := signUp(t, h, fmt.Sprintf("user%d@example.com", i))
		setName(t, h, c, fmt.Sprintf("User%d", i))
		cookies = append(cookies, c)
		ids = append(ids, userFor(t, store, c).ID)
	}
	return cookies, ids
}

func setClock(t *testing.T, store *Store, at string) {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return ts }
}

func pickVia(h http.Handler, c *http.Cookie, memberID string) int {
	return post(h, "/lineup/pick", url.Values{"member": {memberID}}, c).Code
}

func TestLineupExcludesSelfPickedMatchedAndUnnamed(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	_, ids := namedUsers(t, store, h, 9)
	unnamed := userFor(t, store, signUp(t, h, "noname@example.com")).ID
	me, picked, matched := ids[0], ids[1], ids[2]
	store.data.Picks[me] = []string{picked}
	store.data.Matches = []Match{{Users: [2]string{min(me, matched), max(me, matched)}}}

	start := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for day := range 30 {
		store.now = func() time.Time { return start.AddDate(0, 0, day) }
		l, err := store.TodayLineup(me)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.MemberIDs) != lineupSize {
			t.Fatalf("day %d: %d members, want %d", day, len(l.MemberIDs), lineupSize)
		}
		for _, bad := range []string{me, picked, matched, unnamed} {
			if slices.Contains(l.MemberIDs, bad) {
				t.Fatalf("day %d: lineup has excluded user %s", day, bad)
			}
		}
	}
}

func TestLineupShowsAllWhenFewCandidates(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	cookies, ids := namedUsers(t, store, h, 4)
	signUp(t, h, "noname@example.com")
	l, err := store.TodayLineup(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(l.MemberIDs)
	want := slices.Sorted(slices.Values(ids[1:]))
	if !slices.Equal(l.MemberIDs, want) {
		t.Fatalf("lineup %v, want %v", l.MemberIDs, want)
	}
	if home := getWith(h, "/home", cookies[0]).Body.String(); strings.Count(home, `class="suspect"`) != 3 {
		t.Fatalf("want 3 suspects on /home, got %d", strings.Count(home, `class="suspect"`))
	}
}

func TestLineupEmptyState(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	cookies, _ := namedUsers(t, store, h, 1)
	signUp(t, h, "noname@example.com")
	home := getWith(h, "/home", cookies[0]).Body.String()
	if !strings.Contains(home, "No one is in your lineup today") || strings.Contains(home, `class="suspect"`) {
		t.Fatalf("no empty state: %s", home)
	}
}

func TestLineupStableWithinDay(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	_, ids := namedUsers(t, store, h, 10)
	setClock(t, store, "2026-11-06T09:00:00-08:00")
	first, err := store.TodayLineup(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	setClock(t, store, "2026-11-06T23:59:00-08:00")
	reloaded, err := openStore(store.path)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.now = store.now
	for _, s := range []*Store{store, reloaded} {
		again, err := s.TodayLineup(ids[0])
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(first.MemberIDs, again.MemberIDs) {
			t.Fatalf("lineup changed within the day: %v then %v", first.MemberIDs, again.MemberIDs)
		}
	}
}

func TestLineupRollsOverAtPacificMidnight(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	cookies, ids := namedUsers(t, store, h, 10)
	// 07:59 UTC on Nov 7 is still Nov 6 in Los Angeles (PST, UTC-8).
	setClock(t, store, "2026-11-07T07:59:00Z")
	l1, err := store.TodayLineup(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if l1.Day != "2026-11-06" {
		t.Fatalf("day %s, want 2026-11-06", l1.Day)
	}
	if code := pickVia(h, cookies[0], l1.MemberIDs[0]); code != http.StatusSeeOther {
		t.Fatalf("pick status %d", code)
	}
	setClock(t, store, "2026-11-07T08:01:00Z")
	l2, err := store.TodayLineup(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if l2.Day != "2026-11-07" || l2.PickedID != "" || slices.Contains(l2.MemberIDs, l1.MemberIDs[0]) {
		t.Fatalf("bad new lineup %+v", l2)
	}
	if code := pickVia(h, cookies[0], l2.MemberIDs[0]); code != http.StatusSeeOther {
		t.Fatalf("pick on new day status %d", code)
	}
}

func TestSecondPickSameDayRejected(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	cookies, ids := namedUsers(t, store, h, 4)
	l, err := store.TodayLineup(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if code := pickVia(h, cookies[0], l.MemberIDs[0]); code != http.StatusSeeOther {
		t.Fatalf("first pick status %d", code)
	}
	if code := pickVia(h, cookies[0], l.MemberIDs[1]); code != http.StatusConflict {
		t.Fatalf("second pick status %d, want 409", code)
	}
	if got := store.data.Picks[ids[0]]; len(got) != 1 || got[0] != l.MemberIDs[0] {
		t.Fatalf("picks %v", got)
	}
	home := getWith(h, "/home", cookies[0]).Body.String()
	if !strings.Contains(home, "You picked") || !strings.Contains(home, "midnight Pacific") || strings.Contains(home, `action="/lineup/pick"`) {
		t.Fatalf("home after pick: %s", home)
	}
}

func TestPickOutsideLineupRejected(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	cookies, ids := namedUsers(t, store, h, 8)
	l, err := store.TodayLineup(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	outside := ""
	for _, id := range ids[1:] {
		if !slices.Contains(l.MemberIDs, id) {
			outside = id
		}
	}
	for _, id := range []string{outside, ids[0], "nope", ""} {
		if code := pickVia(h, cookies[0], id); code != http.StatusBadRequest {
			t.Fatalf("pick %q status %d, want 400", id, code)
		}
	}
	if len(store.data.Picks[ids[0]]) != 0 {
		t.Fatalf("rejected pick was stored: %v", store.data.Picks[ids[0]])
	}
	assertRedirect(t, post(h, "/lineup/pick", url.Values{"member": {l.MemberIDs[0]}}, nil), "/login")
}

func TestMutualPickCreatesOneMatch(t *testing.T) {
	store := newTestStore(t)
	h := newHandler(store)
	cookies, ids := namedUsers(t, store, h, 2)
	getWith(h, "/home", cookies[0])
	getWith(h, "/home", cookies[1])
	if code := pickVia(h, cookies[0], ids[1]); code != http.StatusSeeOther {
		t.Fatalf("pick status %d", code)
	}
	if len(store.data.Matches) != 0 || len(store.Matches(ids[0])) != 0 || len(store.Matches(ids[1])) != 0 {
		t.Fatal("one-sided pick created a match")
	}
	if home := getWith(h, "/home", cookies[1]).Body.String(); strings.Contains(home, `class="match-name"`) {
		t.Fatal("one-sided pick shows a match")
	}
	if code := pickVia(h, cookies[1], ids[0]); code != http.StatusSeeOther {
		t.Fatalf("pick back status %d", code)
	}
	if len(store.data.Matches) != 1 {
		t.Fatalf("matches %v, want 1", store.data.Matches)
	}
	for i, other := range []string{"User1", "User0"} {
		home := getWith(h, "/home", cookies[i]).Body.String()
		if !strings.Contains(home, `<span class="match-name">`+other+`</span>`) {
			t.Fatalf("user %d home missing match %s", i, other)
		}
	}
}
