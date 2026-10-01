package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMonsterChaseStopsWithinAttackRange pins the NPC chase pawn walk
// (CreatureMove.offensiveFollowTask, CreatureMove.java:563-584, sets
// _pawn/_offset; CreatureMove.updatePosition, :387, ends the walk on its last
// geo leg once within _offset of the pawn, 2D): a monster chasing a player
// who stands still stops at the first step strictly within its attack range
// of the player, not on the player's cell.
func TestMonsterChaseStopsWithinAttackRange(t *testing.T) {
	t.Parallel()
	const attackRange = 40
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	player := liveCombatant(t, srv)

	px, py, pz := player.Position()
	target := location.Location{X: px, Y: py, Z: pz}
	home := location.Location{X: px + 600, Y: py, Z: pz}
	monster := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	monster.Instance.Template.BaseAttackRange = attackRange
	drainUntilQuiet(t, c)

	monster.AddCombatDamageHate(player, 50)
	if !monster.IsMoving() {
		t.Fatal("monster did not start chasing the out-of-range player")
	}
	for i := 0; monster.IsMoving(); i++ {
		if i == 300 {
			t.Fatal("chase still under way after 30 s of position updates")
		}
		srv.TickPositions()
	}

	x, y, z := monster.Position()
	pos := location.Location{X: x, Y: y, Z: z}
	// One position update covers 12 units at the template's run speed 120.
	step := int(monster.Instance.Template.RunSpeed / 10)
	if !pos.In2DRadius(target, attackRange) || pos.In2DRadius(target, attackRange-step) {
		t.Fatalf("chase ended at %+v, %.1f from the player; want the first step within the attack range %d",
			pos, pos.Distance2D(target), attackRange)
	}
	drainUntilQuiet(t, c)
}
