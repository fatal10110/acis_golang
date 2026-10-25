package ai

import (
	"encoding/binary"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// sendAwaySkillID is a fixture one-target TELEPORT skill no NPC's
	// table lists.
	sendAwaySkillID = 9302
	// callerFolkID is a civilian NPC fixture that casts nothing of its own.
	callerFolkID = 30920
)

// An NPC a script makes use a skill on a player, a hostile one and a
// civilian one alike, shows the players around it one MagicSkillUse naming
// it as the caster and the player as the target, with the skill's hit
// time and no reuse delay, and nothing else of a cast: no
// MagicSkillLaunched. The skill's effect, a teleport, lands on the player,
// though no NPC's table lists the skill. The call runs from a hook on the
// NPC's own queue, or on another one, as an attacked hook runs on the
// attacker's.
func TestScriptNPCCallSkillLandsOnItsTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		civilian, onPlayer bool
	}{
		{"hostile", false, false},
		{"civilian", true, false},
		{"civilian from the target's queue", true, true},
		{"hostile from the target's queue", false, true},
	} {
		civilian := tc.civilian
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dest := location.Location{X: 1000, Y: 2000, Z: -3000}
			def := modelskill.Definition{
				ID: sendAwaySkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				SkillType: "TELEPORT", HitTime: 1500, TeleCoords: &dest,
			}
			db := sqltest.SharedDB(t)
			table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, def})
			skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))
			w := bootWarriorsWith(t, []gameservertest.Option{
				gameservertest.WithSkills(skills), gameservertest.WithNpcSpawnSkills(skills),
			}, warriorTemplate(warriorID, nil), gameservertest.FolkTemplate("Folk", callerFolkID))
			var caster attackable.Combatant
			var queue *sim.Queue
			if civilian {
				n, err := w.srv.NpcSpawns.AddSpawn(callerFolkID, w.at.X+100, w.at.Y, w.at.Z, 0, false, 0)
				if err != nil {
					t.Fatalf("spawn %d: %v", callerFolkID, err)
				}
				w.srv.Settle(t)
				w.srv.ReadQueued(t, w.srv.Client)
				f := n.(*npc.Folk)
				caster, queue = f, f.Queue()
			} else {
				h := w.spawn(t, warriorID, 100, 0)
				caster, queue = h, h.Queue()
			}
			if tc.onPlayer {
				queue = w.srv.PlayerQueue(t, w.player.ObjectID())
			}

			done := make(chan struct{})
			if !queue.Post(func() {
				defer close(done)
				script.NPCOf(caster).CallSkill(script.PlayerOf(w.player), modelskill.Ref{ID: sendAwaySkillID, Level: 1})
			}) {
				t.Fatal("the NPC's queue is closed")
			}
			<-done
			w.srv.Settle(t)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			casts := framesOf(frames, serverpackets.OpcodeMagicSkillUse, caster.ObjectID())
			if len(casts) != 1 {
				t.Fatalf("MagicSkillUse frames = %d, want 1", len(casts))
			}
			field := func(i int) int32 { return int32(binary.LittleEndian.Uint32(casts[0][1+4*i:])) }
			if target, id, level, hitTime, reuse := field(1), field(2), field(3), field(4), field(5); target != w.player.ObjectID() || id != sendAwaySkillID || level != 1 || hitTime != 1500 || reuse != 0 {
				t.Fatalf("MagicSkillUse target %d skill %d-%d hit time %d reuse %d, want %d %d-1 1500 0", target, id, level, hitTime, reuse, w.player.ObjectID(), sendAwaySkillID)
			}
			if launched := framesOf(frames, serverpackets.OpcodeMagicSkillLaunched, caster.ObjectID()); len(launched) != 0 {
				t.Fatalf("MagicSkillLaunched frames = %d, want none", len(launched))
			}
			x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
			if abs(x-dest.X) > 20 || abs(y-dest.Y) > 20 {
				t.Fatalf("character at %d,%d, want within 20 of the skill's %d,%d", x, y, dest.X, dest.Y)
			}
		})
	}
}
