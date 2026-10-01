package network

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// accessWrite is one SetAccessLevel call a recordingAccessStore saw.
type accessWrite struct {
	objectID int32
	level    int
	title    string
}

// recordingAccessStore records the access levels stored by object id.
type recordingAccessStore struct {
	mu     sync.Mutex
	writes []accessWrite
}

func (s *recordingAccessStore) SetAccessLevel(_ context.Context, objectID int32, level int, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, accessWrite{objectID, level, title})
	return nil
}

func (s *recordingAccessStore) SetAccessLevelByName(context.Context, string, int) (bool, error) {
	return false, nil
}

func (s *recordingAccessStore) recorded() []accessWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]accessWrite(nil), s.writes...)
}

// TestAdminBanOfLeavingPlayerIsStored pins that a character ban or access
// change aimed at a player already leaving the world is still stored: the
// target's queue is closed by then and drops the change posted there, while
// the GM is told the ban went through.
func TestAdminBanOfLeavingPlayerIsStored(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		run        func(*GameClientLink, *livePlayer, string)
		level      int
		title      string
	}{
		{name: "ban player", line: "admin_ban player Leaver", run: (*GameClientLink).adminBan, level: -1, title: "Away"},
		{name: "set access ban", line: "admin_set access Leaver -1", run: (*GameClientLink).adminSet, level: -1, title: "Away"},
		{name: "set access promote", line: "admin_set access Leaver 1", run: (*GameClientLink).adminSet, level: 1, title: "Admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			levels, err := admin.NewData([]admin.AccessLevel{
				{Level: -1, Name: "Banned"},
				{Level: 0, Name: "User", GiveDamage: true},
				{Level: 1, Name: "Admin", IsGM: true, GiveDamage: true},
			}, nil)
			if err != nil {
				t.Fatalf("access levels: %v", err)
			}
			worker := persist.New(zerolog.Nop())
			t.Cleanup(func() {
				if err := worker.Close(context.Background()); err != nil {
					t.Errorf("close persistence worker: %v", err)
				}
			})
			store := &recordingAccessStore{}

			state := world.New()
			gm := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
			gm.Name = "GM"
			target := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
			target.Name = "Leaver"
			target.SetTitle("Away")
			state.Spawn(target, 100, 0, 0, 0)
			state.AddPlayer(target)
			// Detach has begun: the target is still found by name, but its
			// queue refuses new tasks.
			target.Queue().Close()

			l := &GameClientLink{world: state, log: zerolog.Nop(), admin: levels, persist: worker, accessLevels: store}
			tc.run(l, gm, tc.line)
			if err := worker.Flush(context.Background(), target.ObjectID()); err != nil {
				t.Fatalf("flush: %v", err)
			}

			want := accessWrite{objectID: target.ObjectID(), level: tc.level, title: tc.title}
			if got := store.recorded(); len(got) != 1 || got[0] != want {
				t.Fatalf("stored %+v, want [%+v]", got, want)
			}
		})
	}
}
