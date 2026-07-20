package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// itemSkillsHandler is the etc-item handler name that routes a consumable
// (potions and similar) to the skill its template carries. Other handler
// names are not implemented by this path yet.
const itemSkillsHandler = "ItemSkills"

// useConsumableSkillItem handles the ItemSkills etc-item path for
// instant-cast consumables — potions and anything whose carried skill is
// flagged as a potion or simultaneous-cast. It resolves the item's skill,
// rejects a still-cooling reuse, consumes one unit from the clicked stack,
// installs the item's reuse delay, broadcasts the instant skill animation,
// notifies the client, and applies the skill's effects to the player.
//
// It reports whether inst was handled by this path. A non-consumable, an
// item whose handler is something else, or a carried skill that is neither a
// potion nor simultaneous-cast returns false so the caller's equip-toggle
// fallback still answers the client. Herbs are intentionally not handled
// here: their no-consume and servitor-mirror behavior is a separate path.
func (l *GameClientLink) useConsumableSkillItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Kind != item.KindEtcItem || tmpl.EtcItem == nil {
		return false
	}
	if tmpl.EtcItem.Handler != itemSkillsHandler || tmpl.EtcItem.Type == item.EtcItemHerb {
		return false
	}
	if len(tmpl.AttachedSkills) == 0 {
		return false
	}

	def, ok := l.resolveInstantItemSkill(tmpl.AttachedSkills)
	if !ok {
		return false
	}

	reuseKey := actorcast.ReuseKey(def)
	if live.SkillDisabled(reuseKey) {
		sendMagicCastFailure(live, def, actorcast.ErrSkillDisabled)
		return true
	}

	beforeVitals := live.Vitals()
	if _, ok := l.inventoryService().DestroyItem(inv, inst.ObjectID, 1); !ok {
		sendMagicCastFailure(live, def, actorcast.ErrNotEnoughItems)
		return true
	}
	l.sendInventoryUpdate(live, inv)

	installItemSkillReuse(live, def, reuseKey, tmpl.EtcItem.ReuseDelay)

	self := skillCastObject(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(self, self, int32(def.ID), int32(def.Level), 0, 0, false)
	})
	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageUseS1, int32(def.ID), int32(def.Level)))

	actorcast.ApplyEffects(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, live.Character, def)
	sendMagicStatusUpdate(live, beforeVitals)
	return true
}

// resolveInstantItemSkill returns the first carried skill of refs that
// resolves to a potion or simultaneous-cast definition, mirroring the
// instant-cast gate the consumable path applies. None of the carried
// skills matching leaves the item to the caller's fallback.
func (l *GameClientLink) resolveInstantItemSkill(refs []item.SkillRef) (modelskill.Definition, bool) {
	if l.skills == nil {
		return modelskill.Definition{}, false
	}
	for _, ref := range refs {
		def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if !ok {
			continue
		}
		if def.Potion || def.SimultaneousCast {
			return def, true
		}
	}
	return modelskill.Definition{}, false
}

// installItemSkillReuse applies the item-driven reuse delay to the skill's
// cooldown key, taking the longer of the skill's own reuse delay and the
// item's, the way an item-carried skill's timestamp is recorded.
func installItemSkillReuse(live *livePlayer, def modelskill.Definition, reuseKey int32, itemReuseDelay int32) {
	if live == nil {
		return
	}
	reuse := time.Duration(def.ReuseDelay) * time.Millisecond
	if item := time.Duration(itemReuseDelay) * time.Millisecond; item > reuse {
		reuse = item
	}
	if reuse <= 0 {
		return
	}
	live.DisableSkill(reuseKey, reuse)
	live.AddSkillReuse(modelskill.Ref{ID: def.ID, Level: def.Level}, reuseKey, reuse)
}
