package npcs

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// //unspawnall and //respawnall retire the npcmakers in use: a timer one
// armed before runs nothing, and its tickers stop, so neither command
// leaves a spawn the old makers bring back on top of the new ones (#4838).

const (
	retireBossID  = 21996
	retireTickID  = 21997
	retireRowName = "retire_boss_row"
	retireList    = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="retire_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="retire_boss" territory="retire_field" maximumNpcs="1">
		<ai type="default_use_db_maker"/>
		<npc id="21996" total="1" pos="40;20;30;0" respawn="60sec" dbName="retire_boss_row"/>
	</npcmaker>
	<npcmaker name="retire_tick" territory="retire_field" maximumNpcs="1">
		<ai type="retire_tick_probe"/>
		<npc id="21997" total="1" pos="60;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// retireTickProbe is a fixture maker type: it starts its spawn and a ten
// second ticker it counts the ticks of, and answers its NPC leaving by
// spawning one again 30 seconds on.
type retireTickProbe struct{ ticks atomic.Int32 }

func (p *retireTickProbe) catalog() script.MakerCatalog {
	return script.MakerCatalog{"retire_tick_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				e.Group.Every(10*time.Second, func() { p.ticks.Add(1) })
				e.Group.Spawns()[0].Spawn()
			},
			OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
				ms := e.Spawn
				e.Group.After(30*time.Second, ms.Spawn)
			},
		}}
	}}
}

func bootRetire(t *testing.T) (*folkWorld, *retireTickProbe) {
	t.Helper()
	probe := &retireTickProbe{}
	makers := batch1Makers()
	for typ, ctor := range probe.catalog() {
		makers[typ] = ctor
	}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(retireBossID, retireTickID)),
		gameservertest.WithNpcSpawns(batch1Table(t, retireList)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers),
	)
	return w, probe
}

func retireCount(t *testing.T, mw *makerWorld, step string, bosses, ticks int) {
	t.Helper()
	if got := [2]int{len(mw.inWorld(retireBossID)), len(mw.inWorld(retireTickID))}; got != [2]int{bosses, ticks} {
		t.Fatalf("%s: NPCs in the world (boss, tick) = %v, want [%d %d]", step, got, bosses, ticks)
	}
}

// respawnAll is //respawnall over the spawn list in use.
func respawnAll(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	srv.NpcSpawns.DespawnAll()
	srv.Settle(t)
	srv.NpcSpawns.RespawnAll(srv.NpcSpawns.Spawns())
	srv.Settle(t)
}

// After //respawnall the database-tracked boss killed before it, and the
// NPC whose maker armed a spawn for it, are each back once: the old
// makers' timers bring nothing more. The old makers' tickers stop, however
// many times it runs.
func TestRespawnAllRetiresTheOldMakersTimers(t *testing.T) {
	t.Parallel()
	w, probe := bootRetire(t)
	mw := &makerWorld{srv: w.srv}
	retireCount(t, mw, "boot", 1, 1)

	mw.inWorld(retireBossID)[0].DeleteMe()
	mw.inWorld(retireTickID)[0].DeleteMe()
	w.srv.Settle(t)
	retireCount(t, mw, "both left", 0, 0)

	respawnAll(t, w.srv)
	retireCount(t, mw, "//respawnall", 1, 1)
	w.srv.Advance(t, 61*time.Second)
	retireCount(t, mw, "past the old respawn delays", 1, 1)

	respawnAll(t, w.srv)
	respawnAll(t, w.srv)
	retireCount(t, mw, "two more //respawnall", 1, 1)
	probe.ticks.Store(0)
	w.srv.Advance(t, 10*time.Second)
	if got := probe.ticks.Load(); got != 1 {
		t.Fatalf("ticks in one period after four //respawnall = %d, want the current maker's 1", got)
	}
	if row, ok := w.srv.NpcSpawns.Spawns().State(retireRowName); !ok || row.Status != spawn.StatusAlive {
		t.Fatalf("boss row = %+v, %v; want alive", row, ok)
	}
}

// After //unspawnall a spawn the makers armed before it brings nothing
// back, and their tickers stop.
func TestUnspawnAllRetiresTheMakersTimers(t *testing.T) {
	t.Parallel()
	w, probe := bootRetire(t)
	mw := &makerWorld{srv: w.srv}
	retireCount(t, mw, "boot", 1, 1)

	mw.inWorld(retireTickID)[0].DeleteMe()
	w.srv.Settle(t)
	w.srv.NpcSpawns.DespawnAll()
	w.srv.Settle(t)
	retireCount(t, mw, "//unspawnall", 0, 0)
	probe.ticks.Store(0)
	w.srv.Advance(t, 31*time.Second)
	retireCount(t, mw, "past the armed spawn", 0, 0)
	if got := probe.ticks.Load(); got != 0 {
		t.Fatalf("ticks after //unspawnall = %d, want 0", got)
	}
}
