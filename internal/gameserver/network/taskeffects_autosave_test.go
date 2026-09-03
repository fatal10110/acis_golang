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
	effects.SetAutosave(roster, nil, nil, zerolog.Nop())

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
	effects.SetAutosave(roster, nil, nil, zerolog.Nop())

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
	effects.SetAutosave(roster, nil, nil, zerolog.Nop())

	effects.Save(live)

	pos := chars.savedPosition(t, 44)
	if pos.location != (location.Location{X: 100, Y: 200, Z: 300}) || pos.heading != 12345 {
		t.Fatalf("autosave saved position = %+v heading %d, want (100,200,300) heading 12345", pos.location, pos.heading)
	}
}

// TestAutosaveSaveDoesNotOutraceDetachOfflineWrite guards against #1948: the
// detaching flag TestAutosaveSaveSkipsDetachingSession covers is a
// check-then-act read, not atomic with the DB write it guards. If an
// autosave write is already in flight (past the flag check, mid roster.Save)
// when detachLivePlayer runs, the two writers' `online` column writes can
// interleave and leave online stuck at 1 for a character that already fully
// logged out.
//
// This reproduces that interleaving deterministically: a hook inside the
// fake char store's Save blocks the first call in flight (simulating a slow
// write), detachLivePlayer is started concurrently, and only then is the
// blocked autosave write allowed to complete. live.saveMu (added by #1948's
// fix) must serialize the two so detachLivePlayer's own offline write is
// always the last one recorded, regardless of which writer reached the
// store first.
func TestAutosaveSaveDoesNotOutraceDetachOfflineWrite(t *testing.T) {
	chars := newFakeCharStore()
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := &livePlayer{Character: &player.Character{ID: 45}, log: zerolog.Nop()}
	state.AddPlayer(live)

	entered := make(chan struct{})
	release := make(chan struct{})
	// blocked is a lock-free CAS, not a sync.Once: sync.Once's internal
	// mutex stays held (by the blocked first call) until Do's f returns, so
	// a second concurrent Do call would block on that mutex too — silently
	// serializing the two writers and masking the very race this test
	// exists to reproduce. Only the first Save call for id 45 must block;
	// every later call (detachLivePlayer's own, in the pre-fix code with no
	// live.saveMu) must return immediately, uncontended.
	var blocked int32
	chars.saveHook = func(id int32) {
		if id != 45 {
			return
		}
		if atomic.CompareAndSwapInt32(&blocked, 0, 1) {
			close(entered)
			<-release
		}
	}

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, zerolog.Nop())

	autosaveDone := make(chan struct{})
	go func() {
		effects.Save(live)
		close(autosaveDone)
	}()

	<-entered // autosave holds live.saveMu, blocked in chars.Save before its write is recorded

	link := &GameClientLink{roster: roster, log: zerolog.Nop()}
	detachDone := make(chan struct{})
	go func() {
		link.detachLivePlayer(context.Background(), live)
		close(detachDone)
	}()

	// Give detachLivePlayer a window to run its own save sequence to
	// completion before the blocked autosave write is released. With the
	// #1948 fix, detachLivePlayer blocks on live.saveMu for the whole
	// window instead (a no-op wait); without it, this reliably lets
	// detachLivePlayer's unguarded writes land first, reproducing the
	// interleaving deterministically rather than leaving it to scheduler luck.
	time.Sleep(50 * time.Millisecond)
	close(release) // let the blocked autosave write proceed

	<-autosaveDone
	<-detachDone

	seq := chars.onlineSequence(45)
	if len(seq) == 0 || seq[len(seq)-1] != "offline" {
		t.Fatalf("online-status write sequence for a fully detached character = %v, want it to end \"offline\"", seq)
	}
}
