package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	mutedMagicSkillID    int32 = 1011
	mutedPhysicalSkillID int32 = 3
)

func mutedCastSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: modelskill.ID(mutedMagicSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, MPConsume: 5, SkillType: "DUMMY",
			Magic: true,
		},
		{
			ID: modelskill.ID(mutedPhysicalSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, MPConsume: 5, SkillType: "DUMMY",
		},
	}
}

// TestMutedPlayerCastRefusedSilently lands Mute, PhysicalMute and
// SilenceMagicPhysical on a player before it asks for a magic or a physical
// skill. Mute blocks magic skills, PhysicalMute physical ones, and
// SilenceMagicPhysical both (CreatureCast.meetsHpMpDisabledConditions:
// skill.isMagic() && isMuted() || !skill.isMagic() && isPhysicalMuted()).
// The refusal comes from canCast inside PlayerAI.thinkCast, which returns
// without a message or ActionFailed: the client hears nothing, no MP is
// spent and no cast starts. A skill the effect does not block casts.
func TestMutedPlayerCastRefusedSilently(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		effect  string
		magic   bool
		refused bool
	}{
		{effect: "Mute", magic: true, refused: true},
		{effect: "Mute", magic: false},
		{effect: "PhysicalMute", magic: false, refused: true},
		{effect: "PhysicalMute", magic: true},
		{effect: "SilenceMagicPhysical", magic: true, refused: true},
		{effect: "SilenceMagicPhysical", magic: false, refused: true},
	} {
		kind, skillID := "physical", mutedPhysicalSkillID
		if tt.magic {
			kind, skillID = "magic", mutedMagicSkillID
		}
		t.Run(tt.effect+" on "+kind+" skill", func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Muted", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, mutedCastSkills())),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			for _, def := range mutedCastSkills() {
				seedKnownSkill(t, srv, objID, int(def.ID), 1)
			}
			startInWorld(t, c)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				e, err := effect.New(effect.Skill{ID: 1064, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: tt.effect, Time: 30})
				if err != nil {
					t.Errorf("effect.New(%s): %v", tt.effect, err)
					return
				}
				e.Effector, e.Effected = pc, pc
				pc.EffectList().Add(e)
			})
			drainUntilQuiet(t, c)
			mpBefore := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			if !tt.refused {
				opcodes := frameOpcodes(readUntilQuiet(t, c))
				if !slices.Contains(opcodes, serverpackets.OpcodeMagicSkillUse) {
					t.Fatalf("%s cast under %s sent opcodes %x, want MagicSkillUse", kind, tt.effect, opcodes)
				}
				return
			}
			if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
				t.Fatalf("muted cast answered opcode %#x, want silence", frame[0])
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
				t.Fatalf("MP after muted cast = %d, want unchanged %d", got, mpBefore)
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("muted cast is in flight")
			}
		})
	}
}
