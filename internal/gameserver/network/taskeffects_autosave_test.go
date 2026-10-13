package network

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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
	db, chars := autosaveCharacters(t, 42)
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := queuedTestLive(t, 42)
	sim.RunOwned(live.Queue(), live.markDetaching)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())

	sim.RunOwned(live.Queue(), func() { effects.Save(live) })

	if got := readAutosaveRow(t, db, 42).online; got != 0 {
		t.Fatalf("online after autosave of a detaching session = %d, want 0 (no save)", got)
	}
}

// TestAutosaveSaveRunsForAttachedSession is the control case: a session
// that has not started detaching still gets its periodic autosave write.
func TestAutosaveSaveRunsForAttachedSession(t *testing.T) {
	db, chars := autosaveCharacters(t, 43)
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := queuedTestLive(t, 43)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())

	sim.RunOwned(live.Queue(), func() { effects.Save(live) })

	if got := readAutosaveRow(t, db, 43).online; got != 1 {
		t.Fatalf("online after autosave of an attached session = %d, want 1 (saved)", got)
	}
}

// TestAutosaveSavePersistsPosition guards against #1814: the periodic
// autosave wrote only the 14-column stat update and never x/y/z/heading,
// so a crash rolled every online player back to its login position. The
// reference's periodic autosave (GameClient.java store()) persists position
// in the same write.
func TestAutosaveSavePersistsPosition(t *testing.T) {
	db, chars := autosaveCharacters(t, 44)
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	live := queuedTestLive(t, 44)
	live.Character.SetLastKnownPosition(location.Location{X: 100, Y: 200, Z: 300}, 12345)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())

	sim.RunOwned(live.Queue(), func() { effects.Save(live) })

	row := readAutosaveRow(t, db, 44)
	if row.location != (location.Location{X: 100, Y: 200, Z: 300}) || row.heading != 12345 {
		t.Fatalf("autosave saved position = %+v heading %d, want (100,200,300) heading 12345", row.location, row.heading)
	}
}

// TestAutosaveSaveDoesNotOutraceDetachOfflineWrite guards against #1948: an
// autosave write already in flight when detachLivePlayer runs must not land
// after detach's own offline write and leave online stuck at 1 for a
// character that fully logged out.
//
// The store holds the autosave write on its persistence lane (a slow write),
// detachLivePlayer then runs to completion, and only then is the autosave
// write released. Detach's saves share the owner's lane, so they wait behind
// the held write and its offline write lands last. A detach that wrote
// directly would mark the row offline while the autosave write was still
// held, and the released write would then leave it online.
func TestAutosaveSaveDoesNotOutraceDetachOfflineWrite(t *testing.T) {
	db, store := autosaveCharacters(t, 45)
	if err := store.SetOnline(context.Background(), 45, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var held atomic.Bool
	chars := heldCharacterStore{CharacterStore: store, hold: func(id int32) {
		if id == 45 && held.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}}
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)
	worker := persist.New(zerolog.Nop())
	defer worker.Close(context.Background())

	live := queuedTestLive(t, 45)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, worker, zerolog.Nop())
	sim.RunOwned(live.Queue(), func() { effects.Save(live) })
	<-entered // the autosave write is in flight on the lane

	link := &GameClientLink{roster: roster, log: zerolog.Nop(), persist: worker}
	sim.RunOwned(live.Queue(), func() { link.detachLivePlayer(live) })
	if got := readAutosaveRow(t, db, 45).online; got != 1 {
		t.Fatalf("online while the autosave write was held = %d, want 1 (detach's writes queued behind it)", got)
	}

	close(release)
	if err := worker.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := readAutosaveRow(t, db, 45).online; got != 0 {
		t.Fatalf("online after detach = %d, want 0 (detach's offline write landed last)", got)
	}
}

// heldCharacterStore is the real character store whose Save first runs
// hold, so a test can keep one write in flight as a slow database would.
type heldCharacterStore struct {
	*gamesql.CharacterStore
	hold func(id int32)
}

func (s heldCharacterStore) Save(ctx context.Context, st player.SaveState) error {
	s.hold(st.ID)
	return s.CharacterStore.Save(ctx, st)
}

// autosaveCharacters is the character store of t's database with an
// offline row for each of ids.
func autosaveCharacters(t *testing.T, ids ...int32) (*sql.DB, *gamesql.CharacterStore) {
	t.Helper()
	db := sqltest.SharedDB(t)
	chars := gamesql.NewCharacterStore(db)
	for _, id := range ids {
		seedSelectableSQLCharacterWithID(t, chars, id, fmt.Sprintf("Saved%d", id))
	}
	return db, chars
}

// autosaveRow is what an autosave writes to a characters row that these
// tests read back.
type autosaveRow struct {
	online   int
	location location.Location
	heading  int
}

func readAutosaveRow(t *testing.T, db *sql.DB, id int32) autosaveRow {
	t.Helper()
	var row autosaveRow
	err := db.QueryRowContext(context.Background(), "SELECT online, x, y, z, heading FROM characters WHERE obj_Id = ?", id).
		Scan(&row.online, &row.location.X, &row.location.Y, &row.location.Z, &row.heading)
	if err != nil {
		t.Fatalf("read character %d: %v", id, err)
	}
	return row
}

// queuedTestLive builds an attached player on its own queue, where the
// autosave tick runs Save and detach runs.
func queuedTestLive(t *testing.T, id int32) *livePlayer {
	t.Helper()
	ch := &player.Character{ID: id}
	creatureLive, err := creature.NewLive(location.Location{}, 0, testGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	creatureLive.SetQueue(idleQueue())
	ch.Live = creatureLive
	return &livePlayer{Character: ch, log: zerolog.Nop()}
}

// TestAutosaveSaveSkipsANewerSessionOfTheSameCharacter: a Save posted to a
// session's queue before its detach closed it can run after the character
// has entered the world again on a new queue. It must not save the newer
// session from the old queue.
func TestAutosaveSaveSkipsANewerSessionOfTheSameCharacter(t *testing.T) {
	db, chars := autosaveCharacters(t, 46)
	state := world.New()
	roster := gamemanager.NewRoster(chars, nil, nil, nil, nil, nil, nil, gamemanager.DefaultDeleteAfter, time.Now)

	stale := queuedTestLive(t, 46)
	state.AddPlayer(queuedTestLive(t, 46))

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, nil, nil, nil, zerolog.Nop())
	sim.RunOwned(stale.Queue(), func() { effects.Save(stale) })

	if got := readAutosaveRow(t, db, 46).online; got != 0 {
		t.Fatalf("online after autosave from a stale session's queue = %d, want 0 (no save)", got)
	}
}
