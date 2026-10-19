package npcs

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// BenomMaker (#214): Benom's npcmaker follows the siege of the castle its
// CastleID memo names. Its first spawn is the dungeon gatekeeper, its
// second Benom, database-tracked.

const (
	benomGatekeeperID = 21741
	benomID           = 21742
	benomMaker        = "rune03_npc2016_18m1"
	benomSpawnlist    = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="benom_lair" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rune03_npc2016_18m1" territory="benom_lair" maximumNpcs="3">
		<ai type="benom_maker"><set name="CastleID" val="1"/></ai>
		<npc id="21741" total="1" pos="40;20;30;0" respawn="1min"/>
		<npc id="21742" total="1" pos="60;20;30;0" respawn="0sec" dbName="benom_fixture"/>
	</npcmaker>
</list>`
)

// benomCastles is Gludio with lifeTowers placed life control towers.
func benomCastles(t *testing.T, lifeTowers int) *castledata.Table {
	t.Helper()
	var towers []castledata.ControlTower
	for i := range lifeTowers {
		tower, err := castledata.NewControlTower("gludio_life_tower", "LIFE_CONTROL", 400+100*i, 400, gameservertest.SpawnZ, 1000, 100, 100, nil)
		if err != nil {
			t.Fatal(err)
		}
		towers = append(towers, tower)
	}
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
	}, nil, towers, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// bootBenom boots Gludio with lifeTowers placed life control towers,
// Raiders registered to attack and its siege a dozen hours away, and
// Benom's npcmaker on Gludio's siege.
func bootBenom(t *testing.T, lifeTowers int) (*folkWorld, *heardLog, *siege.Siege) {
	t.Helper()
	log := &heardLog{ids: []int32{benomGatekeeperID, benomID}}
	siegeDate := time.Now().Add(12 * time.Hour).UnixMilli()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithCastles(benomCastles(t, lifeTowers)),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedResidenceChat(t, db, residenceSetup{siege: true})
			if _, err := db.ExecContext(context.Background(), "UPDATE castle SET siegeDate = ? WHERE id = ?", siegeDate, taxCastleID); err != nil {
				t.Fatalf("seed siege date: %v", err)
			}
		}),
		gameservertest.WithZones(gludioBattlefield(t)),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			lifeTowerWorking(), spawnMonster(benomGatekeeperID, "Gatekeeper"), spawnMonster(benomID, "Benom"),
		})),
		gameservertest.WithNpcSpawns(batch1Table(t, benomSpawnlist)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(batch3Makers(nil)),
		log.option(),
	)
	gludio, ok := w.srv.Sieges.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	return w, log, gludio
}

// destroyLifeTower drives the castle's placed life control tower i to its
// dead form.
func (w *folkWorld) destroyLifeTower(t *testing.T, gludio *siege.Siege, i int) {
	t.Helper()
	tower, ok := gludio.Castle().Towers()[i].(*npc.Folk)
	if !ok || !tower.TowerLife() {
		t.Fatalf("placed tower %d is no life control tower", i)
	}
	obj, _ := w.srv.State.Object(w.player)
	attacker, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("player is %T, no combatant", obj)
	}
	tower.TakeDamage(tower.MaxHP()*2, attacker)
	drainUntilQuiet(t, w.c)
}

func (w *folkWorld) benomCount(step string, t *testing.T, gatekeepers, benoms int) {
	t.Helper()
	mw := &makerWorld{srv: w.srv}
	if got := [2]int{len(mw.inWorld(benomGatekeeperID)), len(mw.inWorld(benomID))}; got != [2]int{gatekeepers, benoms} {
		t.Fatalf("%s: (gatekeeper, Benom) in the world = %v, want [%d %d]", step, got, gatekeepers, benoms)
	}
}

// At start, with the siege taking registrations, only the gatekeeper
// spawns. The registrations closing bring Benom in. Once the siege runs,
// the tower check runs every 30 seconds: with both life
// control towers standing nothing happens; the first check after one falls
// sends Benom 10100 and the gatekeeper 10101, each with 1, and the check
// stops. The siege's end opens the registrations, which deletes Benom; the
// gatekeeper stays. An NPC leaving arms no respawn.
func TestBenomMakerFollowsTheSiege(t *testing.T) {
	t.Parallel()
	w, log, gludio := bootBenom(t, 2)
	w.benomCount("boot", t, 1, 0)
	drainUntilQuiet(t, w.c)

	w.srv.Advance(t, time.Second)
	w.benomCount("registrations closed", t, 1, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{benomID}) {
		t.Fatalf("registrations closed: NPCs shown = %v, want [%d]", got, benomID)
	}

	gludio.Start()
	drainUntilQuiet(t, w.c)
	w.srv.Advance(t, 30*time.Second)
	log.expect(t, "two towers standing")

	w.destroyLifeTower(t, gludio, 0)
	w.srv.Advance(t, 30*time.Second)
	log.expect(t, "a tower fell",
		heardEvent{npcID: benomID, event: 10100, arg1: 1},
		heardEvent{npcID: benomGatekeeperID, event: 10101, arg1: 1})
	w.srv.Advance(t, 90*time.Second)
	log.expect(t, "the check stopped")

	benom := (&makerWorld{srv: w.srv}).inWorld(benomID)[0]
	gludio.End()
	w.srv.Settle(t)
	w.benomCount("siege ended", t, 1, 0)
	if got := deleted(drainFrames(t, w.c)); !slices.Contains(got, benom.ObjectID()) {
		t.Fatalf("siege ended: deleted objects = %v, want Benom's %d among them", got, benom.ObjectID())
	}

	gatekeeper := (&makerWorld{srv: w.srv}).inWorld(benomGatekeeperID)[0]
	gatekeeper.DeleteMe()
	w.srv.Settle(t)
	if w.srv.NpcRespawns.Tracked(benomMaker + "#0#0") {
		t.Fatal("the gatekeeper leaving armed a respawn")
	}
}

// A tower check that finds the Benom spawn keeping no NPC brings Benom
// back from his row instead and runs on: a row the delete reset to
// uninitialized first comes alive without an NPC, the next check places
// Benom, and the check after that, with the castle's one life control
// tower under two, sends Benom 10100 and stops; the gatekeeper's spawn,
// keeping no NPC either, hears nothing.
func TestBenomMakerTowerCheckLoadsBenom(t *testing.T) {
	t.Parallel()
	w, log, gludio := bootBenom(t, 1)
	w.srv.Advance(t, time.Second)
	w.benomCount("registrations closed", t, 1, 1)
	w.srv.NpcSpawns.MakerEvent(benomMaker, "1000", 0, 0)
	w.srv.Settle(t)
	w.benomCount("1000", t, 0, 0)
	drainUntilQuiet(t, w.c)

	gludio.Start()
	drainUntilQuiet(t, w.c)
	w.srv.Advance(t, 60*time.Second-time.Millisecond)
	w.benomCount("first check", t, 0, 0)
	log.expect(t, "first check")
	w.srv.Advance(t, time.Millisecond)
	w.benomCount("second check", t, 0, 1)
	log.expect(t, "second check")
	w.srv.Advance(t, 30*time.Second)
	log.expect(t, "third check", heardEvent{npcID: benomID, event: 10100, arg1: 1})
	w.srv.Advance(t, 60*time.Second)
	log.expect(t, "the check stopped")
}
