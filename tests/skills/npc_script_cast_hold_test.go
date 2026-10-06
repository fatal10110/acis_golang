package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScriptHoldCastDesireNeverChasesOutOfReach pins the move-to-target flag
// of a script cast desire: a hold cast queued while the player stands in
// reach, whose player is out of reach by the time the monster thinks,
// leaves the monster standing where it is without casting, where the moving
// form of the same desire walks after the player.
//
// The reach shrinks instead of the player walking off: a walk lets the
// monster's AI tick think while the player is still in reach, and a
// teleport makes the monster forget the player, dropping the desire.
func TestScriptHoldCastDesireNeverChasesOutOfReach(t *testing.T) {
	for _, hold := range []bool{false, true} {
		name := "moving"
		if hold {
			name = "hold"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			objID := srv.SoleObjectID(t)
			player := worldCombatant(t, srv, objID)

			// The monster stands 150 west of the player, inside the probe's
			// 200 range; the probe's range then drops to 10.
			px, py, pz := srv.PlayerPosition(t, objID)
			home := location.Location{X: px - 150, Y: py, Z: pz}
			defs := &laterDefinitions{}
			defs.table.Store(modelskill.NewTable([]modelskill.Definition{probeDefinition(longProbe, 200, 0)}))
			monster, _ := srv.SpawnMovingCastingHostileNPC(t, gameservertest.MovingHostileTemplate("Monster"), home, home, defs)
			drainUntilQuiet(t, c)

			monster.AI().AddCastDesire(player, modelskill.Ref{ID: longProbe, Level: 1}, 1000, true, !hold)
			if len(monster.AI().Desires().Snapshot()) != 1 {
				t.Fatal("cast desire for an in-reach player not queued")
			}

			defs.table.Store(modelskill.NewTable([]modelskill.Definition{probeDefinition(longProbe, 10, 0)}))
			thinkOnNPCQueue(t, monster)
			var intention ai.Intention
			onNPCQueue(t, monster, func() { intention = monster.AI().CurrentIntention() })
			if intention != ai.IntentionCast {
				t.Fatalf("CurrentIntention() after the think = %v, want %v", intention, ai.IntentionCast)
			}

			var moved, cast bool
			for passed := time.Duration(0); passed < time.Second; passed += 100 * time.Millisecond {
				srv.Advance(t, 100*time.Millisecond)
				for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
					if len(frame) < 5 || wireReader(frame[1:]).ReadInt32() != monster.ObjectID() {
						continue
					}
					switch frame[0] {
					case serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeMoveToPawn:
						moved = true
					case serverpackets.OpcodeMagicSkillUse:
						cast = true
					}
				}
			}
			if moved == hold {
				t.Fatalf("monster sent a movement packet = %v, want %v", moved, !hold)
			}
			if !hold {
				return
			}
			if cast {
				t.Fatal("hold cast desire cast at an out-of-reach player")
			}
			if x, y, z := monster.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
				t.Fatalf("monster position = (%d,%d,%d), want %+v", x, y, z, home)
			}
		})
	}
}
