package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Greater CP Potion (5592) and its skill, CP Gauge Potion level 2: a
// self COMBATPOINTHEAL of power 200.
const (
	greaterCPPotionID    int32 = 5592
	cpGaugePotionSkillID int32 = 2166
	cpGaugePotionLevel         = 2
	cpGaugePotionPower         = 200
)

func cpGaugePotionSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(cpGaugePotionSkillID), Level: cpGaugePotionLevel, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "COMBATPOINTHEAL", Power: cpGaugePotionPower, Potion: true,
		ReuseDelay: 50, MagicLevel: 1,
	}
}

func greaterCPPotionTemplates() gameservertest.Option {
	return gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: greaterCPPotionID, Name: "Greater CP Potion", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Depositable: true,
		EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 500, SharedReuseGroup: -1},
		AttachedSkills: []item.SkillRef{{ID: cpGaugePotionSkillID, Level: cpGaugePotionLevel}},
	})))
}

// TestCPPotionThenRealDamageLeaveExactVitals drinks a Greater CP Potion
// from empty CP and then takes a survivable REAL_DAMAGE cast: the potion
// restores exactly what was missing (capped at its power) and says so, and
// the real damage takes exactly its power off HP and leaves the restored CP
// alone. Both handlers change vitals by a delta under the player's lock
// (#5085).
func TestCPPotionThenRealDamageLeaveExactVitals(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{cpGaugePotionSkill()})),
		greaterCPPotionTemplates(),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, greaterCPPotionID, 2)
	startInWorld(t, c)
	caster, aiCtl := srv.SpawnCastingHostileNPC(t, realDamageCasterTemplate(100), modelskill.NewTable([]modelskill.Definition{realDamageProbeSkill()}))
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetCP(0) })
	drainUntilQuiet(t, c)

	maxCP := srv.PlayerMaxCP(t, objID)
	want := min(maxCP, cpGaugePotionPower)
	c.Send(encodeUseItem(potion))
	statuses, msg := selfStatusesThenMessage(t, c, objID, int32(serverpackets.SystemMessageS1CPWillBeRestored), 1)
	assertRestoredNumber(t, msg, int32(want))
	if cp := srv.PlayerCurrentCP(t, objID); cp != want {
		t.Fatalf("CP after the potion = %d, want %d", cp, want)
	}
	assertCasterStatus(t, srv, statuses[0], objID, srv.PlayerCurrentHP(t, objID), srv.PlayerCurrentMP(t, objID))
	drainUntilQuiet(t, c)

	hp := srv.PlayerCurrentHP(t, objID)
	if hp <= realDamageProbeHP {
		t.Fatalf("setup: player HP %d leaves no room for a survivable hit", hp)
	}
	if !caster.Queue().Post(func() {
		aiCtl.Cast(worldCombatant(t, srv, objID), modelskill.Ref{ID: realDamageProbe, Level: 1})
	}) {
		t.Fatal("post npc cast: queue closed")
	}
	srv.AdvanceUntil(t, "the player losing HP", func() bool { return srv.PlayerCurrentHP(t, objID) < hp })
	if got := srv.PlayerCurrentHP(t, objID); got != hp-realDamageProbeHP {
		t.Fatalf("HP after real damage = %d, want %d", got, hp-realDamageProbeHP)
	}
	if got := srv.PlayerCurrentCP(t, objID); got != want {
		t.Fatalf("CP after real damage = %d, want the restored %d", got, want)
	}
	assertCasterStatus(t, srv, readUntil(t, c, serverpackets.OpcodeStatusUpdate), objID, hp-realDamageProbeHP, srv.PlayerCurrentMP(t, objID))
}
