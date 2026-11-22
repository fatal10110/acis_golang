package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	shotServitorSkill   = 1112
	shotServitorNpcID   = 12601
	shotNoTemplateSkill = 1113
)

// chargeBothSpiritshots equips the held sword and charges both spiritshot
// kinds on it.
func chargeBothSpiritshots(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID, sword int32) {
	t.Helper()
	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	drainUntilQuiet(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.SetChargedShot(item.ShotSpirit, true)
		pc.SetChargedShot(item.ShotBlessedSpirit, true)
	})
}

// spiritshotCharges reads the player's plain and blessed spiritshot charges.
func spiritshotCharges(t *testing.T, srv *gameservertest.Server, objID int32) (sps, bsps bool) {
	t.Helper()
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		sps, bsps = pc.SpiritshotCharged(), pc.BlessedSpiritshotCharged()
	})
	return sps, bsps
}

// TestCubicSummonSpendsSpiritshot summons a cubic with both spiritshot kinds
// charged: L2SkillSummon.useSkill (L2SkillSummon.java:179) ends by spending
// the blessed spiritshot when one is charged, so the blessed charge is gone
// and the plain one stays.
func TestCubicSummonSpendsSpiritshot(t *testing.T) {
	t.Parallel()
	defs := cubicSummonSkills(900*time.Second, 900*time.Second)
	for i := range defs {
		defs[i].StaticReuse = false
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, summonStormCubicSkill, 1)
	sword := srv.GiveItem(t, objID, approachSwordID, 1)
	startInWorld(t, c)
	chargeBothSpiritshots(t, srv, c, objID, sword)

	summonStormCubic(t, srv, c, objID)
	if sps, bsps := spiritshotCharges(t, srv, objID); !sps || bsps {
		t.Fatalf("after the cubic summon spiritshot/blessed = %v/%v, want true/false (blessed spent)", sps, bsps)
	}
}

// TestServitorSummonSpendsSpiritshotOnlyWhenSpawned casts a servitor summon
// whose template is missing, then one that spawns. The missing template
// returns before the spend (L2SkillSummon.java:146-151), so both charges
// stay; the spawned servitor spends the blessed one
// (L2SkillSummon.java:179). A second summon while the servitor is out is
// refused and spends nothing.
func TestServitorSummonSpendsSpiritshotOnlyWhenSpawned(t *testing.T) {
	t.Parallel()
	servitor := modelskill.Definition{
		ID:            shotServitorSkill,
		Level:         1,
		Activation:    modelskill.ActivationActive,
		Target:        modelskill.TargetSelf,
		SkillType:     "SUMMON",
		StaticHitTime: true,
		HitTime:       0,
		ReuseDelay:    0,
	}.WithExtra(modelskill.Extra{
		NpcID:               shotServitorNpcID,
		SummonTotalLifeTime: 1_200_000,
	})
	missing := servitor
	extra := *missing.Extra()
	missing.ID, extra.NpcID = shotNoTemplateSkill, shotServitorNpcID+1
	missing = missing.WithExtra(extra)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
			ID: shotServitorNpcID, TemplateID: shotServitorNpcID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
			HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
			CollisionRadius: 8, CollisionHeight: 20,
		}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{servitor, missing})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, shotServitorSkill, 1)
	seedKnownSkill(t, srv, objID, shotNoTemplateSkill, 1)
	sword := srv.GiveItem(t, objID, approachSwordID, 1)
	startInWorld(t, c)
	chargeBothSpiritshots(t, srv, c, objID, sword)

	c.Send(encodeRequestMagicSkillUse(shotNoTemplateSkill, false, false))
	srv.Advance(t, time.Second)
	if readFrameLog(c).index(func(f []byte) bool { return f[0] == serverpackets.OpcodeMagicSkillLaunched }) < 0 {
		t.Fatal("the missing-template summon never launched")
	}
	if _, ok := srv.State.Summon(objID); ok {
		t.Fatal("a summon with no template spawned a servitor")
	}
	if sps, bsps := spiritshotCharges(t, srv, objID); !sps || !bsps {
		t.Fatalf("after the missing-template summon spiritshot/blessed = %v/%v, want both kept", sps, bsps)
	}

	c.Send(encodeRequestMagicSkillUse(shotServitorSkill, false, false))
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		_, ok := srv.State.Summon(objID)
		return ok
	})
	drainUntilQuiet(t, c)
	if sps, bsps := spiritshotCharges(t, srv, objID); !sps || bsps {
		t.Fatalf("after the servitor summon spiritshot/blessed = %v/%v, want true/false (blessed spent)", sps, bsps)
	}

	c.Send(encodeRequestMagicSkillUse(shotServitorSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	if sps, _ := spiritshotCharges(t, srv, objID); !sps {
		t.Fatal("a summon refused with a servitor out spent the plain spiritshot")
	}
}
