package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The last batch 3 maker (#214): the maker of Sailren, booted through the
// batch's catalog file, whose npc DB info hook reads the saved X of
// Sailren's spawn_data row (#4028).

const (
	sailrenID     = 21771
	sailrenRow    = "sailren"
	sailrenLairX  = -113091
	sailrenMakers = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="sailren_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rune20_mb2017_04m1" territory="sailren_field" maximumNpcs="2">
		<ai type="sailren_maker"/>
		<npc id="21771" pos="-113091;20;30;0" total="1" respawn="60sec" dbName="sailren"/>
	</npcmaker>
	<npcmaker name="rune16_npc2017_01m1" territory="sailren_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
	<npcmaker name="rune16_npc2017_13m1" territory="sailren_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
</list>`
)

func bootSailren(t *testing.T, makers *makerEventLog, heard *heardLog) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithNPCs(batch1Templates(sailrenID)),
		gameservertest.WithNpcSpawns(batch1Table(t, sailrenMakers)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(batch3Makers(makers.catalog())),
		heard.option(),
	)
	srv.Settle(t)
	return srv
}

// killSailren kills Sailren and lets his corpse decay.
func killSailren(t *testing.T, srv *gameservertest.Server, sailren *npc.Hostile) {
	t.Helper()
	onQueueOf(t, sailren, func() {
		if !sailren.Die(nil, nil) {
			t.Error("Sailren did not die")
		}
	})
	onQueueOf(t, sailren, func() { sailren.Decay(srv.State, srv.NpcSpawns.RespawnHook(sailren.ObjectID())) })
	srv.Settle(t)
}

// SailrenMaker: Sailren's point is at the lair's X. A row never
// initialized saves no X, which is not the lair's, so the start runs 11047 on rune16_npc2017_01m1 and
// rune16_npc2017_13m1 before Sailren appears. Maker script event 11042
// sends him 11042 with 1 and 0. Dying, his corpse's decay runs 11045 on
// rune16_npc2017_01m1 and comes back after his spawn's respawn delay.
// After a restart with his row saved alive at the lair's X, he appears
// with no 11047; once the row is saved dead with its respawn ahead, the
// start runs no 11047 either.
func TestSailrenMakerReadsItsRow(t *testing.T) {
	makers := &makerEventLog{}
	heard := &heardLog{ids: []int32{sailrenID}}
	srv := bootSailren(t, makers, heard)
	mw := &makerWorld{srv: srv}
	makers.expect(t, "first boot", "rune16_npc2017_01m1:11047(0,0)", "rune16_npc2017_13m1:11047(0,0)")
	if got := len(mw.inWorld(sailrenID)); got != 1 {
		t.Fatalf("first boot: Sailren in the world = %d, want 1", got)
	}

	if !srv.NpcSpawns.MakerEvent("rune20_mb2017_04m1", "11042", 0, 0) {
		t.Fatal("npcmaker rune20_mb2017_04m1 not found by name")
	}
	srv.Settle(t)
	heard.expect(t, "11042", heardEvent{npcID: sailrenID, event: 11042, arg1: 1})
	makers.expect(t, "11042")

	killSailren(t, srv, mw.inWorld(sailrenID)[0])
	makers.expect(t, "Sailren died", "rune16_npc2017_01m1:11045(0,0)")
	// The slot key of a database-tracked spawn is its database name; the
	// harness never fires a respawn timer, so it is brought due by hand.
	if !srv.NpcRespawns.Tracked(sailrenRow) {
		t.Fatal("Sailren's death armed no respawn")
	}
	srv.NpcRespawns.Cancel(sailrenRow)
	srv.NpcSpawns.Respawn(sailrenRow)
	srv.Settle(t)
	if got := len(mw.inWorld(sailrenID)); got != 1 {
		t.Fatalf("after the respawn: Sailren in the world = %d, want 1", got)
	}
	makers.expect(t, "Sailren back")

	srv.Shutdown(t)
	if row := rowState(t, srv, sailrenRow); row.Status != spawn.StatusAlive || row.Location.X != sailrenLairX {
		t.Fatalf("saved row = status %d at X %d, want alive at %d", row.Status, row.Location.X, sailrenLairX)
	}

	srv = bootSailren(t, makers, heard)
	mw = &makerWorld{srv: srv}
	makers.expect(t, "restart at the lair's X")
	sailren := mw.inWorld(sailrenID)[0]
	if x, _, _ := sailren.Position(); x != sailrenLairX {
		t.Fatalf("restart: Sailren at X %d, want his row's %d", x, sailrenLairX)
	}

	killSailren(t, srv, sailren)
	makers.expect(t, "Sailren died again", "rune16_npc2017_01m1:11045(0,0)")
	srv.Shutdown(t)
	if row := rowState(t, srv, sailrenRow); row.Status != spawn.StatusDead {
		t.Fatalf("saved row status = %d, want dead", row.Status)
	}

	srv = bootSailren(t, makers, heard)
	makers.expect(t, "restart with the row dead")
	if got := len((&makerWorld{srv: srv}).inWorld(sailrenID)); got != 0 {
		t.Fatalf("restart with the row dead: Sailren in the world = %d, want 0", got)
	}
}
