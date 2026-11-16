package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	swordSymphony  = modelskill.ID(98)
	plainPdamAura  = modelskill.ID(9342)
	symphonyRadius = 150
)

// pdamAura is a PDAM aura around its caster as wide as Sword Symphony's.
func pdamAura(id modelskill.ID) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetAura,
		Radius: symphonyRadius, Offensive: true, SkillType: "PDAM", Power: 50, HitTime: 500, StaticHitTime: true,
	}
}

// TestSwordSymphonySparesPlayers (Pdam.java:56, EffectFear.DOESNT_AFFECT_PLAYABLE):
// a monster's Sword Symphony aura reaching a player leaves the player
// untouched although the cast resolves on it, while a plain PDAM aura of the
// same reach from the same monster hurts the player.
func TestSwordSymphonySparesPlayers(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	tmpl := realDamageCasterTemplate(100)
	tmpl.PAtk = 500
	caster, aiCtl := srv.SpawnCastingHostileNPC(t, tmpl, modelskill.NewTable([]modelskill.Definition{pdamAura(swordSymphony), pdamAura(plainPdamAura)}))
	drainUntilQuiet(t, srv.Client)
	objID := srv.SoleObjectID(t)
	pc := worldCombatant(t, srv, objID)
	cast := func(id modelskill.ID) {
		t.Helper()
		onNPCQueue(t, caster, func() { aiCtl.Cast(pc, modelskill.Ref{ID: id, Level: 1}) })
	}

	before := srv.PlayerCurrentHP(t, objID)
	cast(swordSymphony)
	srv.Advance(t, 2*time.Second)
	log := readFrameLog(srv.Client)
	if log.index(objectFrame(serverpackets.OpcodeMagicSkillLaunched, caster.ObjectID())) < 0 {
		t.Fatal("the player saw no MagicSkillLaunched of the monster's Sword Symphony")
	}
	if hp := srv.PlayerCurrentHP(t, objID); hp != before {
		t.Fatalf("player HP after Sword Symphony = %d, want unchanged %d", hp, before)
	}

	cast(plainPdamAura)
	srv.AdvanceUntil(t, "the plain PDAM aura hurting the player", func() bool {
		return srv.PlayerCurrentHP(t, objID) < before
	})
}
