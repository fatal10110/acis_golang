package enchant

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// MaxAdminLevel is the highest enchant level an administrator may set.
const MaxAdminLevel = 65535

// AdminSet is the outcome of SetLevel.
type AdminSet struct {
	// Item and Template are what the slot holds; nil when it is empty.
	Item     *item.Instance
	Template *item.Template
	// OldLevel is the item's enchant level before the change.
	OldLevel int
	// Unchanged is set when the item already had the level asked for, and
	// nothing was done.
	Unchanged bool
	// Steps are the skill changes the new level brings, then the
	// equipment broadcast; Persist writes the item's row.
	Steps   []Step
	Persist []inventory.Persist
}

// SetLevel gives the item inv holds in equip slot slot the enchant level
// level, which the caller has checked lies in [0, MaxAdminLevel]. An
// equipped weapon crossing +4 gains or loses its +4 skill. An equipped
// armor piece crossing +6 down loses the +6 skill of the armor set the worn
// chest belongs to; crossing +6 up gains it when every piece of that set is
// at +6 or higher.
func (s *Service) SetLevel(inv *itemcontainer.Inventory, slot, level int) AdminSet {
	inst := inv.ItemAt(slot)
	if inst == nil {
		return AdminSet{}
	}
	tmpl, _ := inv.Templates().Get(inst.TemplateID)
	out := AdminSet{Item: inst, Template: tmpl, OldLevel: inst.Snapshot().EnchantLevel}
	if out.OldLevel == level {
		out.Unchanged = true
		return out
	}
	if inv.SetEnchantLevel(inst, level) {
		out.Persist = append(out.Persist, inventory.Update(inst))
	}
	st := inst.Snapshot()
	if st.Equipped() && tmpl != nil {
		current := st.EnchantLevel
		switch {
		case tmpl.Weapon != nil:
			if hasEnchant4Skill(tmpl) {
				if out.OldLevel >= item.Enchant4SkillLevel && current < item.Enchant4SkillLevel {
					out.Steps = append(out.Steps, Step{Kind: StepRevokeEnchantSkill, Template: tmpl})
				} else if out.OldLevel < item.Enchant4SkillLevel && current >= item.Enchant4SkillLevel {
					out.Steps = append(out.Steps, Step{Kind: StepGrantEnchantSkill, Template: tmpl})
				}
			}
		case tmpl.Kind == item.KindArmor:
			if out.OldLevel >= armorset.Enchant6Level && current < armorset.Enchant6Level {
				if set, ok := s.armorSets.Worn(inv); ok && set.Enchant6Skill > 0 {
					out.Steps = append(out.Steps, Step{Kind: StepRevokeArmorSetSkill, SkillID: set.Enchant6Skill})
				}
			} else if out.OldLevel < armorset.Enchant6Level && current >= armorset.Enchant6Level {
				if skillID := s.wornSetEnchant6Skill(inv); skillID > 0 {
					out.Steps = append(out.Steps, Step{Kind: StepGrantArmorSetSkill, SkillID: skillID})
				}
			}
		}
	}
	out.Steps = append(out.Steps, Step{Kind: StepBroadcastEquipment})
	return out
}
