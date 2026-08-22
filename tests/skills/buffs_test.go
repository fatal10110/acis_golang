package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBuffIconPersistsUntilExpiry casts a self-buff and walks its visible
// lifetime: the icon appears in AbnormalStatusUpdate with the definition's
// remaining duration, and the production effect sweep retires it on time,
// clearing the icon list.
func TestBuffIconPersistsUntilExpiry(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 4, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
				Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 2, Icon: true}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 4, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(4, false, false))
	readCastStartFrames(t, c, objID, 4, 1, 500, 60_000, objID)
	icons := readStatusUpdateSkippingAbnormal(t, c, objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
	found := false
	for _, e := range icons {
		if e.SkillID == 4 && int32(e.Level) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("AbnormalStatusUpdate icons after buff cast = %+v, want skill 4", icons)
	}
	drainUntilQuiet(t, c)

	time.Sleep(2200 * time.Millisecond)
	srv.TickEffects()
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1HasWornOff, 4, 1)
	if entries := readAbnormalStatusUpdateEntries(t, c); len(entries) != 0 {
		t.Fatalf("AbnormalStatusUpdate entries after expiry = %+v, want none", entries)
	}
	drainUntilQuiet(t, c)
}

// TestDebuffThatFailsToLandSendsAttackFailed verifies a debuff with no land
// chance still plays the cast (ack, use message, launch report) but answers
// with the attack-failed message and applies nothing.
func TestDebuffThatFailsToLandSendsAttackFailed(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
				BaseLandRate: 0, IgnoreResists: true,
				Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(5, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 5, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageAttackFailed)
	drainUntilQuiet(t, c)
}

// TestStunBlocksCastingAndMovement lands a stun through a self-cast skill
// and verifies both blanket locks the reference applies to a CC'd caster:
// further casts get ActionFailed only — no reason message — and walk
// requests are released with ActionFailed too.
func TestStunBlocksCastingAndMovement(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 20, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 0, StaticHitTime: true,
				SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
				BaseLandRate: 100, IgnoreResists: true,
				Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 30}},
			},
			{
				ID: 21, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 0, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 20, 1)
	seedKnownSkill(t, srv, objID, 21, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(20, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "stun cast ack")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 20, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "stun launch")
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(21, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "cast while stunned")
	drainUntilQuiet(t, c)

	c.Send(encodeMoveBackwardToLocation(200, 200, 30))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "move while stunned")
	drainUntilQuiet(t, c)
}
