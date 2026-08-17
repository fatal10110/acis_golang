package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestCharacterHealAmountUsesMagicAttackAndHealProficiency(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 49
	c := liveCharacter(1, tmpl, combatItems())
	c.AddStatFuncs([]effect.Mod{{Stat: stat.HealProficiency, Op: effect.OpAdd, Value: 11, Owner: testModOwner()}})

	amount, ok := c.HealAmount(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealAmount() ok = false")
	}
	if want := 41.099019513592786; !closeFloat(amount, want) {
		t.Fatalf("HealAmount() = %v, want %v", amount, want)
	}

	static, ok := c.HealAmount(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealAmount(HEAL_STATIC) ok = false")
	}
	if want := 25.0 + 11; !closeFloat(static, want) {
		t.Fatalf("HealAmount(HEAL_STATIC) = %v, want %v", static, want)
	}
}
