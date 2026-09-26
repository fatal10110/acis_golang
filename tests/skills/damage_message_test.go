package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPhysicalSkillReportsDamageToTheCaster hits the fixture monster with a
// PDAM skill: the caster reads the damage message carrying the HP the
// monster lost.
func TestPhysicalSkillReportsDamageToTheCaster(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 42, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "PDAM", Power: 1,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	full := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(42, false, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, hostile.ObjectID())
	srv.AdvanceUntil(t, "PDAM hit", func() bool { return hostile.HP() < float64(full) })
	if hostile.Dead() {
		t.Fatal("monster died; the hit must leave it alive to compare the damage")
	}
	lost := int32(float64(full) - hostile.HP())

	r := findSystemMessage(t, c, int32(serverpackets.SystemMessageYouDidS1Dmg))
	if params, typ, amount := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); params != 1 || typ != serverpackets.SystemMessageParamNumber || amount != lost {
		t.Fatalf("damage message = params %d type %d amount %d, want one number %d", params, typ, amount, lost)
	}
	drainUntilQuiet(t, c)
}
