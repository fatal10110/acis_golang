package network

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
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
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())

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
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())

	effects.Save(live)

	if got := chars.saves(43); got != 1 {
		t.Fatalf("roster.Save calls for an attached session = %d, want 1", got)
	}
}

// TestAutosaveSavePersistsPosition guards against #1814: the periodic
// autosave wrote only the 14-column stat update and never x/y/z/heading,
// so a crash rolled every online player back to its login position. The
// reference's periodic autosave (GameClient.java store()) persists position
// in the same write.
func TestAutosaveSavePersistsPosition(t *testing.T) {
	chars := newFakeCharStore()
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := &livePlayer{Character: &player.Character{ID: 44}, log: zerolog.Nop()}
	live.Character.SetLastKnownPosition(location.Location{X: 100, Y: 200, Z: 300}, 12345)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())

	effects.Save(live)

	pos := chars.savedPosition(t, 44)
	if pos.location != (location.Location{X: 100, Y: 200, Z: 300}) || pos.heading != 12345 {
		t.Fatalf("autosave saved position = %+v heading %d, want (100,200,300) heading 12345", pos.location, pos.heading)
	}
}

// TestAutosaveSaveDoesNotOutraceDetachOfflineWrite guards against #1948: an
// autosave write already in flight when detachLivePlayer runs must not land
// after detach's own offline write and leave online stuck at 1 for a
// character that fully logged out.
//
// A hook inside the fake char store's Save blocks the autosave write on its
// persistence lane (a slow write), detachLivePlayer then runs to completion,
// and only then is the autosave write released. Detach's saves share the
// owner's lane, so they wait behind the blocked write and its offline write
// is the last one recorded. A detach that wrote directly would record
// "offline" while the autosave write was still blocked.
func TestAutosaveSaveDoesNotOutraceDetachOfflineWrite(t *testing.T) {
	chars := newFakeCharStore()
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)
	worker := persist.New(zerolog.Nop())
	defer worker.Close(context.Background())

	live := &livePlayer{Character: &player.Character{ID: 45}, log: zerolog.Nop()}
	state.AddPlayer(live)

	entered := make(chan struct{})
	release := make(chan struct{})
	var blocked atomic.Bool
	chars.saveHook = func(id int32) {
		if id == 45 && blocked.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, worker, zerolog.Nop())
	effects.Save(live)
	<-entered // the autosave write is in flight on the lane

	link := &GameClientLink{roster: roster, log: zerolog.Nop(), persist: worker}
	link.detachLivePlayer(live)
	if seq := chars.onlineSequence(45); len(seq) != 0 {
		t.Fatalf("online-status writes recorded while the autosave write was blocked = %v, want none", seq)
	}

	close(release)
	if err := worker.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	seq := chars.onlineSequence(45)
	if len(seq) != 3 || seq[0] != "online" || seq[1] != "online" || seq[2] != "offline" {
		t.Fatalf("online-status write sequence = %v, want [online online offline]", seq)
	}
}
