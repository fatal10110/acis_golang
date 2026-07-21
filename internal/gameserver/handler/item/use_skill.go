// Package item handles item-use actions.
package item

import (
	"time"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ItemSkillsHandler is the etc-item handler name that routes a consumable
// (potions and similar) to the skill its template carries.
const ItemSkillsHandler = "ItemSkills"

// ElixirsHandler is the etc-item handler name for elixirs: the same
// instant-cast dispatch as ItemSkillsHandler, but restricted to a
// player-owned caster.
const ElixirsHandler = "Elixirs"

// Outcome classifies the result of an ItemSkills instant-cast use.
type Outcome uint8

const (
	// NotHandled means the item is not a consumable this path covers, so
	// the caller should fall through to its next branch.
	NotHandled Outcome = iota
	// Applied means the skill resolved and its effects were applied to the
	// caster. One unit was consumed unless the item is a herb.
	Applied
	// ReuseRejected means the skill's reuse delay is still cooling.
	ReuseRejected
	// NotEnoughItems means the stack couldn't be decremented.
	NotEnoughItems
	// PetRejected means an elixir was used by a non-player caster.
	PetRejected
)

// UseResult is the outcome of one ItemSkills instant-cast use. Skill is the
// resolved skill definition for every outcome except NotHandled, so the
// caller can name it in a rejection reply. The ShortBuff* fields are only
// meaningful on Applied when HasShortBuff is true: the caller drives the
// caster's short-buff HUD state with them (after sending its own cast
// packets, so the HUD update lands last on the wire, matching the
// reference's own ordering).
type UseResult struct {
	Outcome Outcome
	Skill   modelskill.Definition

	HasShortBuff             bool
	ShortBuffSkillID         int32
	ShortBuffLevel           int32
	ShortBuffDurationSeconds int32
}

// HPPotionSkillIDs are the healing-potion-family skill ids that drive the
// item-window short-buff HUD slot.
var hpPotionSkillIDs = map[int32]bool{2031: true, 2032: true, 2037: true}

// SkillCaster is the actor using the item: it identifies and positions
// itself (for the cast animation and effect resolution) and owns its own
// skill reuse/cooldown state.
type SkillCaster interface {
	actorcast.Target
	SkillDisabled(key int32) bool
	DisableSkill(key int32, delay time.Duration)
	AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration)

	// ShortBuffTaskSkillID returns the skill id of the short buff
	// currently showing on the item-window HUD slot, or 0 if none. Use
	// reads this only to decide HasShortBuff; it never mutates the HUD
	// state itself — see UseResult's doc comment.
	ShortBuffTaskSkillID() int32
}

// InventoryDestroyer decrements a stack by a count from an inventory.
type InventoryDestroyer interface {
	DestroyItem(inv *itemcontainer.Inventory, objectID int32, count int) (invops.Result, bool)
}

// UseRequest carries the collaborators the ItemSkills instant-cast path
// needs to validate, consume, and apply one item-carried skill.
type UseRequest struct {
	Caster      SkillCaster
	Inventory   *itemcontainer.Inventory
	Item        *modelitem.Instance
	Definitions actorcast.Definitions
	Effects     actorcast.EffectHandlers
	Destroyer   InventoryDestroyer

	// IsPet marks Caster as a non-player-owned actor (pet/servitor) rather
	// than the player itself. An Elixirs-handled item rejects such a
	// caster; ItemSkillsHandler items do not distinguish on this field.
	IsPet bool
}

// Use runs the ItemSkills instant-cast path for one etc item: it
// discriminates an etc consumable whose handler is ItemSkills or Elixirs,
// resolves the first carried skill flagged as a potion or
// simultaneous-cast, rejects a still-cooling reuse, consumes one unit
// from the clicked stack (skipped for herbs, which apply without
// consuming), installs the item-driven reuse delay, and applies the
// skill's effects to the caster.
//
// It reports NotHandled for anything that isn't such an instant-cast
// consumable, so the caller's next branch (equip-toggle, etc.) still gets
// a chance to answer the client. Mirroring a herb's effect onto the
// caster's active servitor is not handled here: it needs a servitor cast
// surface this package doesn't have a dependency on yet.
func Use(req UseRequest) UseResult {
	if req.Caster == nil || req.Inventory == nil || req.Item == nil {
		return UseResult{Outcome: NotHandled}
	}
	tmpl, ok := req.Inventory.Templates().Get(req.Item.TemplateID)
	if !ok || tmpl.Kind != modelitem.KindEtcItem || tmpl.EtcItem == nil {
		return UseResult{Outcome: NotHandled}
	}
	handler := tmpl.EtcItem.Handler
	if handler != ItemSkillsHandler && handler != ElixirsHandler {
		return UseResult{Outcome: NotHandled}
	}
	if handler == ElixirsHandler && req.IsPet {
		return UseResult{Outcome: PetRejected}
	}
	if len(tmpl.AttachedSkills) == 0 {
		return UseResult{Outcome: NotHandled}
	}

	def, ok := resolveInstantItemSkill(tmpl.AttachedSkills, req.Definitions)
	if !ok {
		return UseResult{Outcome: NotHandled}
	}

	reuseKey := actorcast.ReuseKey(def)
	if req.Caster.SkillDisabled(reuseKey) {
		return UseResult{Outcome: ReuseRejected, Skill: def}
	}

	if tmpl.EtcItem.Type != modelitem.EtcItemHerb {
		if _, ok := req.Destroyer.DestroyItem(req.Inventory, req.Item.ObjectID, 1); !ok {
			return UseResult{Outcome: NotEnoughItems, Skill: def}
		}
	}

	installItemReuse(req.Caster, def, reuseKey, tmpl.EtcItem.ReuseDelay)
	actorcast.ApplyEffects(req.Effects, req.Caster, req.Caster, def)

	result := UseResult{Outcome: Applied, Skill: def}
	result.HasShortBuff, result.ShortBuffSkillID, result.ShortBuffLevel, result.ShortBuffDurationSeconds = shortBuffDecision(req.Caster, def)
	return result
}

// shortBuffDecision decides whether def should drive the item-window
// short-buff HUD slot: it's one of the healing-potion-family skills, and
// its id doesn't lose to whatever short buff is already showing — the
// reference's own gate (`skillInfo.getId() >= player.getShortBuffTaskSkillId()`).
// It only reads caster's current HUD state; actually updating that state
// (and sending the packet) is the caller's job once its own cast packets
// are already on the wire.
func shortBuffDecision(caster SkillCaster, def modelskill.Definition) (ok bool, skillID, level, durationSeconds int32) {
	if !hpPotionSkillIDs[int32(def.ID)] || int32(def.ID) < caster.ShortBuffTaskSkillID() {
		return false, 0, 0, 0
	}
	if len(def.Effects) == 0 {
		return false, 0, 0, 0
	}
	duration := int32(def.Effects[0].Count * def.Effects[0].Time)
	return true, int32(def.ID), int32(def.Level), duration
}

// resolveInstantItemSkill returns the first carried skill of refs that
// resolves to a potion or simultaneous-cast definition. None matching
// leaves the item to the caller's fallback.
func resolveInstantItemSkill(refs []modelitem.SkillRef, defs actorcast.Definitions) (modelskill.Definition, bool) {
	if defs == nil {
		return modelskill.Definition{}, false
	}
	for _, ref := range refs {
		def, ok := defs.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if !ok {
			continue
		}
		if def.Potion || def.SimultaneousCast {
			return def, true
		}
	}
	return modelskill.Definition{}, false
}

// installItemReuse applies the item-driven reuse delay to the skill's
// cooldown key, taking the longer of the skill's own reuse delay and the
// item's, the way an item-carried skill's timestamp is recorded.
func installItemReuse(caster SkillCaster, def modelskill.Definition, reuseKey int32, itemReuseDelay int32) {
	reuse := time.Duration(def.ReuseDelay) * time.Millisecond
	if item := time.Duration(itemReuseDelay) * time.Millisecond; item > reuse {
		reuse = item
	}
	if reuse <= 0 {
		return
	}
	caster.DisableSkill(reuseKey, reuse)
	caster.AddSkillReuse(modelskill.Ref{ID: def.ID, Level: def.Level}, reuseKey, reuse)
}
