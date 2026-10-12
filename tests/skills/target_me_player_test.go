package skills

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestTargetMeFromPlayerCaster pins the TargetMe effect for a player caster
// on a player victim (EffectTargetMe.onStart): a victim already targeting
// the caster attacks it; any other victim only selects the caster. Aggression
// (28-1, AGGDAMAGE, TargetMe) applies its effects without a landing roll, so
// both branches are deterministic; the victim carries karma so the cast is
// allowed without force.
func TestTargetMeFromPlayerCaster(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		victimTargetsMe bool
	}{
		{"victim already targeting the caster attacks it", true},
		{"victim targeting nothing only selects the caster", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := shippedSkill(t, 28, 1)
			srv := bootTargetConditionCaster(t, def)
			c, casterID := srv.Client, srv.SoleObjectID(t)
			victim := srv.SeedCharacterFor(t, "victim", "Victim", 5, 0)
			// AGGDAMAGE is no damage skill, so even a forced cast needs a
			// victim with karma or a PvP flag.
			if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = ? WHERE obj_Id = ?", 240, victim.ID); err != nil {
				t.Fatalf("set karma: %v", err)
			}
			vc := srv.DialClient(t, "victim", 1)
			startInWorldAmongPlayers(t, vc)
			startInWorldAmongPlayers(t, c)
			drainUntilQuiet(t, vc)
			drainUntilQuiet(t, c)
			if tc.victimTargetsMe {
				selectTarget(t, vc, casterID)
			}
			selectTarget(t, c, victim.ID)

			c.Send(encodeRequestMagicSkillUse(28, false, false))
			srv.Advance(t, 3*time.Second)
			frames := collectUntilQuiet(t, vc)

			attacked, selected := false, false
			for _, frame := range frames {
				switch frame[0] {
				case serverpackets.OpcodeAttack, serverpackets.OpcodeMoveToPawn:
					r := wireReader(frame[1:])
					if r.ReadInt32() == victim.ID && r.ReadInt32() == casterID {
						attacked = true
					}
				case serverpackets.OpcodeMyTargetSelected:
					if wireReader(frame[1:]).ReadInt32() == casterID {
						selected = true
					}
				}
			}
			if tc.victimTargetsMe {
				if !attacked {
					t.Fatalf("victim already targeting the caster did not attack it; victim frames %x", opcodes(frames))
				}
				return
			}
			if attacked {
				t.Fatal("victim not targeting the caster attacked it")
			}
			if !selected {
				t.Fatalf("victim not targeting the caster was not handed the caster as its target; victim frames %x", opcodes(frames))
			}
		})
	}
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}
