package clan

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: castle gate and trap upgrades (Castle.java:558-600, 880-935,
// CastleManager.java:85-110, DoorStatus.java:72-86, Siege.midVictory):
//   - a door's maximum HP is its template HP times its upgrade ratio;
//   - Castle.upgradeDoor, on one of the castle's own doors only, sets the
//     ratio and fills the door's HP to the new maximum (setMaxHp, which
//     broadcasts), storing the ratio in castle_doorupgrade when asked;
//   - Castle.setTrapUpgrade stores the level of a control tower's trap in
//     castle_trapupgrade when asked and keeps it on the tower;
//   - the boot applies every stored trap level, then every stored gate
//     ratio, without storing them again;
//   - midVictory takes every gate's ratio back to 1 without touching its HP,
//     deletes the castle's stored gate upgrades, takes every trap back to
//     level 0 and deletes the castle's stored trap upgrades, before the
//     gates stand again at half their (now plain) maximum HP.

// gludioOtherDoorID is a door id that is none of Gludio's gates.
const gludioOtherDoorID = 19210099

// gateStatus is the DoorStatusUpdate of a closed door of maxHP with hp
// left.
func gateStatus(objectID int32, doorID int, hp, maxHP, showHP int32) gateFrame {
	damage := max(0, min(6, 6-int32(math.Ceil(float64(hp)/float64(maxHP)*6))))
	return gateFrame{
		opcode: serverpackets.OpcodeDoorStatusUpdate, objectID: objectID, closed: 1, damage: damage, showHP: showHP,
		doorID: int32(doorID), maxHP: maxHP, hp: hp,
	}
}

// storedUpgrades returns the castle_doorupgrade rows as doorId/hp/castleId
// and the castle_trapupgrade rows as castleId/towerIndex/level triples.
func storedUpgrades(t *testing.T, w *clanWorld) (doors, traps [][3]int) {
	t.Helper()
	w.srv.FlushPersistence(t)
	read := func(query string) [][3]int {
		rows, err := w.srv.DB.QueryContext(context.Background(), query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		defer rows.Close()
		var out [][3]int
		for rows.Next() {
			var r [3]int
			if err := rows.Scan(&r[0], &r[1], &r[2]); err != nil {
				t.Fatalf("%s: %v", query, err)
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return out
	}
	return read(`SELECT doorId, hp, castleId FROM castle_doorupgrade ORDER BY doorId`),
		read(`SELECT castleId, towerIndex, level FROM castle_trapupgrade ORDER BY castleId, towerIndex`)
}

func expectRows(t *testing.T, what string, got, want [][3]int) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s rows = %v, want %v", what, got, want)
	}
}

// TestCastleUpgradesRestoredThenRemovedAtMidVictory boots Gludio with its
// gate upgraded threefold and its first trap tower at level 2, plus rows
// the boot leaves out (a door that is not Gludio's gate, a tower index
// Gludio has not) and rows of Dion. The gate stands at 1800 HP and shows
// it to a hit during the siege. Taking Gludio mid-siege takes the gate
// back to its plain 600 maximum and stands it at half of that, removes
// Gludio's trap levels and deletes Gludio's stored rows, Dion's staying.
func TestCastleUpgradesRestoredThenRemovedAtMidVictory(t *testing.T) {
	t.Parallel()
	w := gateWorldSeeded(t, []string{
		`INSERT INTO castle_doorupgrade (doorId, hp, castleId) VALUES (19210001, 3, 1), (19210099, 2, 1), (19210003, 2, 2)`,
		`INSERT INTO castle_trapupgrade (castleId, towerIndex, level) VALUES (1, 3, 2), (1, 200, 3), (2, 3, 1)`,
	})
	s := gludioSiegeOf(t, w)
	gate, wall := gateObject(t, w, gludioGateID), gateObject(t, w, gludioWallID)
	gludio, dion := s.Castle(), mustCastle(t, w, 2)
	if gate.HPUpgradeRatio() != 3 || gate.MaxHP() != 3*gateDoorHP || gate.HP() != 3*gateDoorHP {
		t.Fatalf("restored gate ratio %d, max HP %d, HP %d; want 3, %d, %d", gate.HPUpgradeRatio(), gate.MaxHP(), gate.HP(), 3*gateDoorHP, 3*gateDoorHP)
	}
	if wall.HPUpgradeRatio() != 1 || wall.MaxHP() != gateDoorHP {
		t.Fatalf("wall upgraded by Dion's row: ratio %d, max HP %d", wall.HPUpgradeRatio(), wall.MaxHP())
	}
	if gludio.TrapUpgradeLevel(3) != 2 || gludio.TrapUpgradeLevel(200) != 0 || dion.TrapUpgradeLevel(3) != 1 {
		t.Fatalf("restored trap levels Gludio[3]=%d Gludio[200]=%d Dion[3]=%d, want 2, 0, 1",
			gludio.TrapUpgradeLevel(3), gludio.TrapUpgradeLevel(200), dion.TrapUpgradeLevel(3))
	}

	s.Start()
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	expectGateFrames(t, "upgraded gate hit", gateFrames(t, drainFrames(t, w.leader), gate.ObjectID()),
		gateStatus(gate.ObjectID(), gludioGateID, 3*gateDoorHP-100, 3*gateDoorHP, 1))
	drainFrames(t, w.member)

	knights, ok := w.srv.Clans.Table().Get(knightsClanID)
	if !ok {
		t.Fatal("no Knights clan")
	}
	if _, ok := w.srv.Castles.SetOwner(gludio, knights); !ok {
		t.Fatal("Knights did not take Gludio")
	}
	s.MidVictory()
	frames := drainFrames(t, w.leader)
	expectGateFrames(t, "gate at mid-victory", gateFrames(t, frames, gate.ObjectID()),
		gateStatus(gate.ObjectID(), gludioGateID, gateDoorHP/2, gateDoorHP, 1))
	expectGateFrames(t, "wall at mid-victory", gateFrames(t, frames, wall.ObjectID()),
		gateStatus(wall.ObjectID(), gludioWallID, gateDoorHP/2, gateDoorHP, 1))
	if gate.HPUpgradeRatio() != 1 || gate.MaxHP() != gateDoorHP {
		t.Fatalf("gate after mid-victory: ratio %d, max HP %d", gate.HPUpgradeRatio(), gate.MaxHP())
	}
	if gludio.TrapUpgradeLevel(3) != 0 || dion.TrapUpgradeLevel(3) != 1 {
		t.Fatalf("trap levels after mid-victory Gludio[3]=%d Dion[3]=%d, want 0, 1", gludio.TrapUpgradeLevel(3), dion.TrapUpgradeLevel(3))
	}
	doors, traps := storedUpgrades(t, w)
	expectRows(t, "castle_doorupgrade", doors, [][3]int{{gludioWallID, 2, 2}})
	expectRows(t, "castle_trapupgrade", traps, [][3]int{{2, 3, 1}})
}

// TestCastleUpgradeStoredAndKeptOverDoorReload upgrades Gludio's gate
// twofold and its first trap tower to level 3, both stored: the gate's
// observers see it at its new 1200 HP maximum, full. A door that is not
// Gludio's gate and a tower index Gludio has not change and store
// nothing. A door reload builds the gate upgraded still.
func TestCastleUpgradeStoredAndKeptOverDoorReload(t *testing.T) {
	t.Parallel()
	w := gateWorld(t)
	gludio := gludioSiegeOf(t, w).Castle()
	gate := gateObject(t, w, gludioGateID)

	if !gludio.UpgradeDoor(gludioGateID, 2, true) {
		t.Fatal("Gludio could not upgrade its gate")
	}
	got := gateFrames(t, drainFrames(t, w.leader), gate.ObjectID())
	expectGateFrames(t, "upgraded gate", got, gateFrame{
		opcode: serverpackets.OpcodeDoorStatusUpdate, objectID: gate.ObjectID(), closed: 1,
		doorID: gludioGateID, maxHP: 2 * gateDoorHP, hp: 2 * gateDoorHP,
	})
	if gludio.UpgradeDoor(gludioOtherDoorID, 2, true) {
		t.Fatal("Gludio upgraded a door that is not its gate")
	}
	if !gludio.SetTrapUpgrade(3, 3, true) || gludio.TrapUpgradeLevel(3) != 3 {
		t.Fatalf("Gludio trap tower 3 at level %d, want 3", gludio.TrapUpgradeLevel(3))
	}
	if gludio.SetTrapUpgrade(200, 3, true) {
		t.Fatal("Gludio upgraded the trap of a tower it has not")
	}
	doors, traps := storedUpgrades(t, w)
	expectRows(t, "castle_doorupgrade", doors, [][3]int{{gludioGateID, 2, 1}})
	expectRows(t, "castle_trapupgrade", traps, [][3]int{{1, 3, 3}})

	table, err := door.NewTable(gludioGates())
	if err != nil {
		t.Fatal(err)
	}
	if err := w.srv.WorldObjects.ReloadDoors(table); err != nil {
		t.Fatalf("reload doors: %v", err)
	}
	reloaded := gateObject(t, w, gludioGateID)
	if reloaded == gate || reloaded.HPUpgradeRatio() != 2 || reloaded.MaxHP() != 2*gateDoorHP || reloaded.HP() != 2*gateDoorHP {
		t.Fatalf("reloaded gate ratio %d, max HP %d, HP %d; want 2, %d, %d",
			reloaded.HPUpgradeRatio(), reloaded.MaxHP(), reloaded.HP(), 2*gateDoorHP, 2*gateDoorHP)
	}
}

func mustCastle(t *testing.T, w *clanWorld, id int) *castle.Castle {
	t.Helper()
	c, ok := w.srv.Castles.Get(id)
	if !ok {
		t.Fatalf("no castle %d", id)
	}
	return c
}
