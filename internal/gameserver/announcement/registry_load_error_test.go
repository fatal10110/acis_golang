package announcement

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
)

// brokenStore fails every Load once broken is set.
type brokenStore struct {
	fakeStore
	broken bool
}

func (s *brokenStore) Load() ([]admin.Announcement, error) {
	if s.broken {
		return nil, errors.New("malformed")
	}
	return s.fakeStore.Load()
}

// A load that fails leaves the announcements held before, and their
// schedules, as they were.
func TestFailedLoadKeepsHeldAnnouncements(t *testing.T) {
	store := &brokenStore{fakeStore: fakeStore{list: []admin.Announcement{{Message: "login"}, auto("auto", 5, 10, 0)}}}
	r, clock, rec := newTestRegistry(t, store)
	if err := r.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	store.broken = true
	if err := r.Load(); err == nil {
		t.Fatal("Load() of a broken file succeeded, want an error")
	}
	if got := r.List(); len(got) != 2 || got[0].Message != "login" || got[1].Message != "auto" {
		t.Fatalf("List() after a failed load = %+v, want both announcements", got)
	}
	clock.Advance(15 * time.Second)
	assertTimes(t, rec, "auto", 5, 15)
}
