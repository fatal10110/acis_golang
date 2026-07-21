package item

import (
	"testing"

	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestUseDrivesShortBuffForHPPotionFamily(t *testing.T) {
	potion := modelskill.Definition{
		ID: 2031, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}}, // 14s
	}
	caster := &fakeCaster{}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if res.Outcome != Applied {
		t.Fatalf("Outcome = %v, want Applied", res.Outcome)
	}
	if !res.HasShortBuff {
		t.Fatal("HasShortBuff = false, want true for an HP-potion-family skill")
	}
	if res.ShortBuffSkillID != 2031 || res.ShortBuffLevel != 1 || res.ShortBuffDurationSeconds != 14 {
		t.Fatalf("short buff = skill %d level %d duration %d, want 2031/1/14", res.ShortBuffSkillID, res.ShortBuffLevel, res.ShortBuffDurationSeconds)
	}
}

func TestUseSkipsShortBuffForNonHPPotionSkill(t *testing.T) {
	potion := modelskill.Definition{
		ID: 9999, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}},
	}
	caster := &fakeCaster{}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if res.HasShortBuff {
		t.Fatal("HasShortBuff = true, want false for a skill outside the HP-potion family")
	}
}

func TestUseSkipsShortBuffWhenIDLosesToCurrent(t *testing.T) {
	// A Lesser Healing Potion (2031) must not override a Greater Healing
	// Potion (2037) already showing on the HUD, matching the reference's
	// own id-ordering gate.
	potion := modelskill.Definition{
		ID: 2031, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}},
	}
	caster := &fakeCaster{shortBuffTaskSkillID: 2037}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if res.HasShortBuff {
		t.Fatal("HasShortBuff = true, want false when the new skill id loses to the currently-showing one")
	}
}

func TestUseAllowsShortBuffWhenIDMatchesOrWins(t *testing.T) {
	potion := modelskill.Definition{
		ID: 2037, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}},
	}
	caster := &fakeCaster{shortBuffTaskSkillID: 2031}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if !res.HasShortBuff {
		t.Fatal("HasShortBuff = false, want true when the new skill id is numerically >= the currently-showing one")
	}
}
