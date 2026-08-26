package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestAutosaveSaveSkipsDetachingSession guards against the online-status
// race described on the #1744/#1815/#1773/#1743 PR review: TaskEffects.Save
// (the autosave tick's write) must not run against a session whose detach
// has already begun, because detachLivePlayer's own SaveOfflineRecency call
// (online = 0) can complete before autosave's roster.Save call (which now
// also sets online = 1), leaving online stuck at 1 for a character that
// already logged out.
func TestAutosaveSaveSkipsDetachingSession(t *testing.T) {
	chars := newFakeCharStore()
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := &livePlayer{Character: &player.Character{ID: 42}, log: zerolog.Nop()}
	live.detaching = true
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, zerolog.Nop())

	effects.Save(live)

	if got := chars.saves(42); got != 0 {
		t.Fatalf("roster.Save calls for a detaching session = %d, want 0", got)
	}
}

// TestAutosaveSaveRunsForAttachedSession is the control case: a session
// that has not started detaching still gets its periodic autosave write.
func TestAutosaveSaveRunsForAttachedSession(t *testing.T) {
	chars := newFakeCharStore()
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := &livePlayer{Character: &player.Character{ID: 43}, log: zerolog.Nop()}
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, zerolog.Nop())

	effects.Save(live)

	if got := chars.saves(43); got != 1 {
		t.Fatalf("roster.Save calls for an attached session = %d, want 1", got)
	}
}
