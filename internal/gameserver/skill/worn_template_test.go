package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestRefreshAndUnequipFollowAReloadedAwayTemplate wears a weapon whose
// template a reload then removes: an Expertise refresh still revokes and
// regrants its attached skill by the template it went on with, and taking
// it off revokes the skill and forgets that template.
func TestRefreshAndUnequipFollowAReloadedAwayTemplate(t *testing.T) {
	const weaponTemplateID int32 = 52
	const grantedSkillID = 952
	tmpl := &item.Template{
		ID: weaponTemplateID, Kind: item.KindWeapon, Slot: item.SlotRHand,
		Crystal:        item.CrystalD,
		Weapon:         &item.WeaponDetail{Type: item.WeaponSword},
		AttachedSkills: []item.SkillRef{{ID: grantedSkillID, Level: 1}},
	}
	templates := item.NewTable([]*item.Template{tmpl})
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: grantedSkillID, Level: 1, Activation: modelskill.ActivationActive},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, skills)
	ch := &player.Character{ID: 1}
	ch.SetSkillLevel(239, int(item.CrystalD))
	inst := inv.AddNew(weaponTemplateID, 1, 100)
	inv.EquipItem(inst, tmpl)
	if _, _, err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	templates.Replace(item.NewTable(nil))

	ch.SetSkillLevel(239, 0)
	if _, _, err := p.RefreshEquippedItemStats(ch, inv); err != nil {
		t.Fatalf("RefreshEquippedItemStats() error: %v", err)
	}
	if got := ch.SkillLevel(grantedSkillID); got != 0 {
		t.Fatalf("SkillLevel(%d) after losing Expertise = %d, want 0 (revoked)", grantedSkillID, got)
	}
	ch.SetSkillLevel(239, int(item.CrystalD))
	if _, _, err := p.RefreshEquippedItemStats(ch, inv); err != nil {
		t.Fatalf("RefreshEquippedItemStats() error: %v", err)
	}
	if got := ch.SkillLevel(grantedSkillID); got != 1 {
		t.Fatalf("SkillLevel(%d) after regaining Expertise = %d, want 1 (granted again)", grantedSkillID, got)
	}

	inv.UnequipItem(inst)
	if !p.UnequipItemStats(ch, inv, inst, nil) {
		t.Fatal("UnequipItemStats() skillsChanged = false, want true (the attached skill leaves)")
	}
	if got := ch.SkillLevel(grantedSkillID); got != 0 {
		t.Fatalf("SkillLevel(%d) after taking it off = %d, want 0", grantedSkillID, got)
	}
	if _, ok := ch.EquippedTemplate(inst); ok {
		t.Fatal("the template it went on with is still recorded after it came off")
	}
}
