package item

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Handler names for the direct-from-window shot etc items UseShot covers.
// Beast (summon-charged) variants and fishing shots are not handled here:
// they need charged-shot state on the summon actor and on the (currently
// unmodeled) fishing state respectively, neither of which exists yet.
const (
	SoulShotsHandler          = "SoulShots"
	SpiritShotsHandler        = "SpiritShots"
	BlessedSpiritShotsHandler = "BlessedSpiritShots"
)

// ShotCharger is the actor charging a shot: it can attempt a soulshot or
// spiritshot charge on its active weapon and knows whether an item is
// enabled for AutoSoulShot (which suppresses a rejection message, but not
// the caller's ActionFailed reply).
type ShotCharger interface {
	ChargeSoulshot(shotCrystal modelitem.CrystalType, reducedRoll int) (int32, player.ChargeShotResult)
	ChargeSpiritshot(kind modelitem.ShotKind, shotCrystal modelitem.CrystalType) (int32, player.ChargeShotResult)
	AutoSoulShotEnabled(itemID int32) bool
}

// ShotOutcome classifies the result of one UseShot attempt.
type ShotOutcome uint8

const (
	// ShotNotHandled means tmpl isn't a shot etc item this path covers.
	ShotNotHandled ShotOutcome = iota
	// ShotApplied means the weapon was charged and the shot count consumed.
	ShotApplied
	// ShotAlreadyCharged means the weapon already carries this charge; the
	// reference treats this as a pure no-op, not a rejection.
	ShotAlreadyCharged
	// ShotNoCapacity means no real weapon is equipped, or it can't carry
	// this shot kind at all.
	ShotNoCapacity
	// ShotGradeMismatch means the shot's crystal grade doesn't match the
	// weapon's.
	ShotGradeMismatch
	// ShotNotEnoughItems means the stack couldn't be decremented.
	ShotNotEnoughItems
)

// ShotUseRequest carries the collaborators UseShot needs to charge one
// shot item.
type ShotUseRequest struct {
	Caster    ShotCharger
	Inventory *itemcontainer.Inventory
	Item      *modelitem.Instance
	Template  *modelitem.Template
	Destroyer InventoryDestroyer
}

// ShotUseResult is the outcome of one UseShot call. AutoEnabled reports
// whether Item's template is enabled for AutoSoulShot, so the caller can
// suppress a rejection message the reference itself suppresses in that
// case. SkillID is the visual charge skill to broadcast on ShotApplied (0
// if the template attaches none).
type ShotUseResult struct {
	Outcome     ShotOutcome
	AutoEnabled bool
	SkillID     int32
}

// UseShot charges req.Caster's active weapon with the soulshot or
// spiritshot req.Template carries (resolved from its EtcItem handler
// name), consuming the weapon's shot count from req.Item's stack on
// success. It reports ShotNotHandled for any handler name this path
// doesn't cover, so the caller's next branch still gets a chance to
// answer the client.
func UseShot(req ShotUseRequest) ShotUseResult {
	if req.Template == nil || req.Template.EtcItem == nil {
		return ShotUseResult{Outcome: ShotNotHandled}
	}

	var consume int32
	var result player.ChargeShotResult
	switch req.Template.EtcItem.Handler {
	case SoulShotsHandler:
		consume, result = req.Caster.ChargeSoulshot(req.Template.Crystal, rnd.Get(100))
	case SpiritShotsHandler:
		consume, result = req.Caster.ChargeSpiritshot(modelitem.ShotSpirit, req.Template.Crystal)
	case BlessedSpiritShotsHandler:
		consume, result = req.Caster.ChargeSpiritshot(modelitem.ShotBlessedSpirit, req.Template.Crystal)
	default:
		return ShotUseResult{Outcome: ShotNotHandled}
	}

	autoEnabled := req.Caster.AutoSoulShotEnabled(req.Template.ID)
	switch result {
	case player.ChargeShotAlreadyCharged:
		return ShotUseResult{Outcome: ShotAlreadyCharged, AutoEnabled: autoEnabled}
	case player.ChargeShotNoCapacity:
		return ShotUseResult{Outcome: ShotNoCapacity, AutoEnabled: autoEnabled}
	case player.ChargeShotGradeMismatch:
		return ShotUseResult{Outcome: ShotGradeMismatch, AutoEnabled: autoEnabled}
	}

	if _, ok := req.Destroyer.DestroyItem(req.Inventory, req.Item.ObjectID, int(consume)); !ok {
		return ShotUseResult{Outcome: ShotNotEnoughItems, AutoEnabled: autoEnabled}
	}

	var skillID int32
	if len(req.Template.AttachedSkills) > 0 {
		skillID = int32(req.Template.AttachedSkills[0].ID)
	}
	return ShotUseResult{Outcome: ShotApplied, AutoEnabled: autoEnabled, SkillID: skillID}
}
