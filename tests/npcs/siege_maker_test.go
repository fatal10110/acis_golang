package npcs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The maker siege event (#3805): a maker finds a castle by id and registers
// its npcmaker on the castle's siege; each status change of the siege then
// runs the siege event hook of every registered maker, in registration
// order, with the new status, before the change's other world steps (the
// control towers start working after the hook of the siege's start). The
// castle handle reads the siege's status and the castle's standing life
// control towers.

const siegeWatchNpcID = 21510

const siegeWatchSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="watch_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="watch_first" territory="watch_field" maximumNpcs="1">
		<ai type="siege_watch_maker"/>
		<npc id="21510" total="1" pos="40;20;30;100" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="watch_second" territory="watch_field" maximumNpcs="1">
		<ai type="siege_watch_maker"/>
		<npc id="21510" total="1" pos="80;20;30;100" respawn="60sec"/>
	</npcmaker>
</list>`

// siegeWatch is what the fixture makers saw: the status each read at its
// start, and each siege event in the order the hooks ran.
type siegeWatch struct {
	mu      sync.Mutex
	started []script.SiegeStatus
	castle  *script.Castle
	unknown bool // a castle id that names none found nothing
	events  []siegeEvent
}

// siegeEvent is one siege event hook run: the npcmaker it ran for, the
// status it was handed, the status and standing life towers the castle
// handle read inside the hook.
type siegeEvent struct {
	maker      string
	status     script.SiegeStatus
	live       script.SiegeStatus
	lifeTowers int
}

func (w *siegeWatch) makers() script.MakerCatalog {
	return script.MakerCatalog{"siege_watch_maker": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(m *script.Maker, e script.MakerStart) {
				c := m.Castle(taxCastleID)
				w.mu.Lock()
				w.unknown = m.Castle(99) == nil
				w.castle = c
				if c != nil {
					w.started = append(w.started, c.SiegeStatus())
				}
				w.mu.Unlock()
				if c != nil {
					c.AddMakerEvent(m, e.Group)
				}
			},
			OnSiegeEvent: func(_ *script.Maker, e script.MakerSiege) {
				ev := siegeEvent{maker: e.Group.Maker().Name, status: e.Status, live: e.Castle.SiegeStatus(), lifeTowers: e.Castle.AliveLifeTowers()}
				w.mu.Lock()
				w.events = append(w.events, ev)
				w.mu.Unlock()
			},
		}}
	}}
}

func (w *siegeWatch) take() []siegeEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.events
	w.events = nil
	return out
}

// bootSiegeWatch boots Gludio with one placed life control tower, Raiders
// registered to attack, its siege a dozen hours away, and the two fixture
// npcmakers.
func bootSiegeWatch(t *testing.T) (*folkWorld, *siegeWatch) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "watch.xml"), []byte(siegeWatchSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	watch := &siegeWatch{}
	siegeDate := time.Now().Add(12 * time.Hour).UnixMilli()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithCastles(commandCastles(t, true)),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedResidenceChat(t, db, residenceSetup{siege: true})
			if _, err := db.ExecContext(context.Background(), "UPDATE castle SET siegeDate = ? WHERE id = ?", siegeDate, taxCastleID); err != nil {
				t.Fatalf("seed siege date: %v", err)
			}
		}),
		gameservertest.WithZones(gludioBattlefield(t)),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{lifeTowerWorking(), spawnMonster(siegeWatchNpcID, "Watcher")})),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithMakers(watch.makers()),
	)
	return w, watch
}

func TestMakerSiegeEventFollowsTheSiege(t *testing.T) {
	t.Parallel()
	w, watch := bootSiegeWatch(t)
	gludio, ok := w.srv.Sieges.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}

	watch.mu.Lock()
	started, castle, unknown := slices.Clone(watch.started), watch.castle, watch.unknown
	watch.mu.Unlock()
	if want := []script.SiegeStatus{siege.StatusRegistrationOpened, siege.StatusRegistrationOpened}; !slices.Equal(started, want) {
		t.Fatalf("statuses read at start = %v, want %v", started, want)
	}
	if castle == nil || castle.ID() != taxCastleID || !unknown {
		t.Fatalf("castle by id = %v, unknown id found nothing = %v; want Gludio and true", castle, unknown)
	}
	if got := castle.AliveLifeTowers(); got != 1 {
		t.Fatalf("alive life towers before the siege = %d, want the placed one", got)
	}

	both := func(st script.SiegeStatus, towers int) []siegeEvent {
		return []siegeEvent{
			{maker: "watch_first", status: st, live: st, lifeTowers: towers},
			{maker: "watch_second", status: st, live: st, lifeTowers: towers},
		}
	}
	check := func(step string, want []siegeEvent) {
		t.Helper()
		if got := watch.take(); !slices.Equal(got, want) {
			t.Fatalf("%s: siege events = %+v, want %+v", step, got, want)
		}
	}

	// The calendar's step a second after boot closes the registrations: the
	// siege is less than a day away.
	w.srv.Advance(t, time.Second)
	check("registrations closed", both(siege.StatusRegistrationOver, 1))

	// The hooks of the start run before the towers work: under the siege a
	// tower not yet working is not standing.
	gludio.Start()
	drainUntilQuiet(t, w.c)
	check("siege started", both(siege.StatusInProgress, 0))
	_, tower := w.gludioLifeTower(t)
	if got := castle.AliveLifeTowers(); got != 1 {
		t.Fatalf("alive life towers once the siege runs = %d, want the working one", got)
	}

	obj, _ := w.srv.State.Object(w.player)
	attacker, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("player is %T, no combatant", obj)
	}
	tower.TakeDamage(tower.MaxHP()*2, attacker)
	drainUntilQuiet(t, w.c)
	if got := castle.AliveLifeTowers(); got != 0 {
		t.Fatalf("alive life towers once the tower fell = %d, want 0", got)
	}
	check("tower fell", nil)

	// The siege's end opens the next registrations; outside the siege every
	// placed life tower stands.
	gludio.End()
	drainUntilQuiet(t, w.c)
	check("siege ended", both(siege.StatusRegistrationOpened, 1))
}
