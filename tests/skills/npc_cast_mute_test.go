package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// sawNPCCancel reads c's frames until the stream goes quiet and reports
// whether any was a MagicSkillCanceled for hostile.
func sawNPCCancel(c *testsupport.ScriptedClient, hostile *npc.Hostile) bool {
	canceled := false
	for {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return canceled
		}
		if frame[0] == serverpackets.OpcodeMagicSkillCanceled && wireReader(frame[1:]).ReadInt32() == hostile.ObjectID() {
			canceled = true
		}
	}
}

// TestCastStoppingEffectsStopNPCCast lands Mute, PhysicalMute and
// SilenceMagicPhysical on a monster mid-way through a desire-driven cast.
// EffectMute.onStart stops a magic cast only, EffectPhysicalMute.onStart a
// physical one only, and EffectSilenceMagicPhysical.onStart any cast. A
// stopped cast reaches observers as MagicSkillCanceled
// (CreatureCast.stop) and drops the CAST desire that drove it
// (NpcCast.notifyCastFinishToAI); a cast left alone keeps running with its
// desire.
func TestCastStoppingEffectsStopNPCCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		effect  string
		magic   bool
		stopped bool
	}{
		{effect: "Mute", magic: true, stopped: true},
		{effect: "Mute", magic: false},
		{effect: "PhysicalMute", magic: false, stopped: true},
		{effect: "PhysicalMute", magic: true},
		{effect: "SilenceMagicPhysical", magic: true, stopped: true},
		{effect: "SilenceMagicPhysical", magic: false, stopped: true},
	} {
		kind := "physical"
		if tt.magic {
			kind = "magic"
		}
		t.Run(tt.effect+" on "+kind+" cast", func(t *testing.T) {
			t.Parallel()
			srv, hostile := bootNPCDesireCasterWith(t, tt.magic)
			c := srv.Client

			thinkOnNPCQueue(t, hostile)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "NPC MagicSkillUse")
			if !hasCastDesire(hostile) {
				t.Fatal("CAST desire gone mid-cast, want it held until the cast ends")
			}

			e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: tt.effect, Time: 30})
			if err != nil {
				t.Fatalf("effect.New(%s): %v", tt.effect, err)
			}
			e.Effector, e.Effected = hostile, hostile
			onNPCQueue(t, hostile, func() { hostile.EffectList().Add(e) })

			if got := sawNPCCancel(c, hostile); got != tt.stopped {
				t.Fatalf("monster MagicSkillCanceled = %v, want %v", got, tt.stopped)
			}
			if got := hostile.CastingNow(); got == tt.stopped {
				t.Fatalf("monster CastingNow() = %v after %s, want %v", got, tt.effect, !tt.stopped)
			}
			if got := hasCastDesire(hostile); got == tt.stopped {
				t.Fatalf("CAST desire held = %v after %s, want %v", got, tt.effect, !tt.stopped)
			}
		})
	}
}

// TestMutedNPCDropsBlockedCastDesire lands Mute, PhysicalMute and
// SilenceMagicPhysical on a monster before it thinks. The think first drops
// every CAST desire the monster cannot cast right now
// (NpcAI.thinkAttack's removeIf on meetsHpMpDisabledConditions): Mute
// blocks a magic skill, PhysicalMute a physical one, SilenceMagicPhysical
// both. A dropped desire starts no cast; a skill the effect leaves alone
// casts as usual.
func TestMutedNPCDropsBlockedCastDesire(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		effect  string
		magic   bool
		blocked bool
	}{
		{effect: "Mute", magic: true, blocked: true},
		{effect: "Mute", magic: false},
		{effect: "PhysicalMute", magic: false, blocked: true},
		{effect: "PhysicalMute", magic: true},
		{effect: "SilenceMagicPhysical", magic: true, blocked: true},
		{effect: "SilenceMagicPhysical", magic: false, blocked: true},
	} {
		kind := "physical"
		if tt.magic {
			kind = "magic"
		}
		t.Run(tt.effect+" before "+kind+" cast", func(t *testing.T) {
			t.Parallel()
			srv, hostile := bootNPCDesireCasterWith(t, tt.magic)
			c := srv.Client

			e, err := effect.New(effect.Skill{ID: 1064, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: tt.effect, Time: 30})
			if err != nil {
				t.Fatalf("effect.New(%s): %v", tt.effect, err)
			}
			e.Effector, e.Effected = hostile, hostile
			onNPCQueue(t, hostile, func() { hostile.EffectList().Add(e) })
			drainUntilQuiet(t, c)

			thinkOnNPCQueue(t, hostile)
			cast := false
			for {
				frame := c.ReadWithTimeout(300 * time.Millisecond)
				if frame == nil {
					break
				}
				if frame[0] == serverpackets.OpcodeMagicSkillUse {
					cast = true
				}
			}
			if cast == tt.blocked {
				t.Fatalf("monster MagicSkillUse under %s = %v, want %v", tt.effect, cast, !tt.blocked)
			}
			if got := hasCastDesire(hostile); got == tt.blocked {
				t.Fatalf("CAST desire held under %s = %v, want %v", tt.effect, got, !tt.blocked)
			}
		})
	}
}
