package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestRecallLeavesSevenSignsDungeon pins L2SkillTeleport's dungeon exit:
// every player the recall sends away, by coordinates or by recall type,
// leaves its Seven Signs dungeon before it moves; a target the recall
// skips keeps its membership.
func TestRecallLeavesSevenSignsDungeon(t *testing.T) {
	for _, def := range []modelskill.Definition{
		{RecallType: modelskill.RecallTown},
		{TeleCoords: &location.Location{X: 100, Y: 200, Z: 300}},
	} {
		caster := newRecallPlayer(1)
		moved := newRecallPlayer(2)
		jailed := newRecallPlayer(3)
		jailed.jailed = true
		for _, p := range []*recallPlayer{caster, moved, jailed} {
			p.in7sDungeon = true
		}
		recall(caster, def, caster, moved, jailed)
		for _, p := range []*recallPlayer{caster, moved} {
			if p.in7sDungeon || !p.leftDungeonFirst || !p.moved() {
				t.Fatalf("telecoords %v: player %d in dungeon %v, left before moving %v, moved %v; want out of the dungeon before moving",
					def.TeleCoords != nil, p.objectID, p.in7sDungeon, p.leftDungeonFirst, p.moved())
			}
		}
		if !jailed.in7sDungeon || jailed.moved() {
			t.Fatalf("telecoords %v: skipped jailed target in dungeon %v, moved %v; want it left in place", def.TeleCoords != nil, jailed.in7sDungeon, jailed.moved())
		}
	}
}
