package skills

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// fixtureMonsterID is the template id of the harness's plain fixture
// monsters.
const fixtureMonsterID = 100

// withWarriorBehavior binds WarriorBehavior, the ordinary fighting
// monster's behavior, to the fixture monster id: with nothing to do it
// wanders, or follows its live master; attacked, it wants to attack back
// and its party assists. Tests of the AI built-ins those hooks drive
// (wander, follow, attack, assist) boot with it, as an NPC no behavior
// binds does none of them.
func withWarriorBehavior() gameservertest.Option {
	const path = "ai.FixtureMonster"
	return gameservertest.WithNPCScripts(
		map[int32]script.NPCKind{fixtureMonsterID: script.KindHostile},
		[]script.Listing{{Path: path}},
		script.Catalog{path: func() script.Script {
			s := monsterbehavior.WarriorBehavior()
			s.NPCs = []int32{fixtureMonsterID}
			return s
		}},
	)
}
