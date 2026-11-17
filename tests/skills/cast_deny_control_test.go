package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestCastRequestRefusedUnderControlEffects pins RequestMagicSkillUse's
// out-of-control gate for the crowd-control effects that hold a caster
// (RequestMagicSkillUse: Creature.isOutOfControl answers ActionFailed alone
// before the skill is looked up): no cast packets, no MP spent, no cast in
// flight.
func TestCastRequestRefusedUnderControlEffects(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Stun", "Sleep", "Paralyze", "Fear"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootCastDenyAI(t)
			c := srv.Client
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				e, err := effect.New(effect.Skill{ID: 2, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: name, Time: 30})
				if err != nil {
					t.Errorf("effect.New(%s): %v", name, err)
					return
				}
				e.Effector, e.Effected = pc, pc
				pc.EffectList().Add(e)
			})
			drainUntilQuiet(t, c)
			mpBefore := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "refused cast")
			if queued := c.ReadQueued(); len(queued) != 0 {
				t.Fatalf("refused cast extra opcodes = %x, want none", frameOpcodes(queued))
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
				t.Fatalf("MP after refused cast = %d, want unchanged %d", got, mpBefore)
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("refused cast is in flight")
			}
		})
	}
}
