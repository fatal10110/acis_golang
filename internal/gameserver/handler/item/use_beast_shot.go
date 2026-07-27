package item

import (
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Handler names for the beast (summon-charged) shot etc items UseBeastShot
// covers.
const (
	BeastSoulShotsHandler   = "BeastSoulShots"
	BeastSpiritShotsHandler = "BeastSpiritShots"
)

// blessedBeastSpiritshotID selects the blessed variant within
// BeastSpiritShotsHandler; the reference distinguishes it by item id, not a
// separate handler name.
const blessedBeastSpiritshotID = 6647

// BeastShotCharger is the active summon a beast soulshot/spiritshot charges.
type BeastShotCharger interface {
	Dead() bool
	ChargedShot(kind modelitem.ShotKind) bool
	SetChargedShot(kind modelitem.ShotKind, charged bool)
	SSCount() int
	SPSCount() int
}

// BeastShotOutcome classifies the result of one UseBeastShot attempt.
type BeastShotOutcome uint8

const (
	// BeastShotNotHandled means tmpl isn't a beast shot item this path covers.
	BeastShotNotHandled BeastShotOutcome = iota
	// BeastShotApplied means the summon was charged and the shot count consumed.
	BeastShotApplied
	// BeastShotAlreadyCharged means the summon already carries this charge;
	// the reference treats this as a pure no-op, not a rejection.
	BeastShotAlreadyCharged
	// BeastShotCallerIsSummon means the item was used by a summon itself,
	// which cannot use beast shots.
	BeastShotCallerIsSummon
	// BeastShotNoSummon means the caster has no active pet or servitor.
	BeastShotNoSummon
	// BeastShotSummonDead means the caster's active summon is dead.
	BeastShotSummonDead
	// BeastShotNotEnoughItems means the stack couldn't be decremented.
	BeastShotNotEnoughItems
)

// AutoShotChecker reports whether an item is enabled for automatic shot
// use, so a not-enough-items rejection can suppress its message the way the
// reference's disableAutoShot does for an auto-enabled item.
type AutoShotChecker interface {
	AutoSoulShotEnabled(itemID int32) bool
}

// BeastShotUseRequest carries the collaborators UseBeastShot needs to charge
// one beast shot item onto the caster's active summon.
type BeastShotUseRequest struct {
	// CallerIsSummon is true when the entity using the item is itself a
	// summon (the reference's Playable-instanceof-Summon guard). Direct
	// item-window use is always player-initiated in this codebase, so real
	// callers always pass false; the field exists so this rejection branch
	// has independent oracle coverage.
	CallerIsSummon bool
	Caster         AutoShotChecker
	Summon         BeastShotCharger
	Inventory      *itemcontainer.Inventory
	Item           *modelitem.Instance
	Template       *modelitem.Template
	Destroyer      InventoryDestroyer
}

// BeastShotUseResult is the outcome of one UseBeastShot call. AutoEnabled
// reports whether Item's template is enabled for automatic shot use, so the
// caller can suppress a BeastShotNotEnoughItems message the reference
// itself suppresses in that case. SkillID is the visual charge skill to
// broadcast on BeastShotApplied (0 if the template attaches none).
type BeastShotUseResult struct {
	Outcome     BeastShotOutcome
	AutoEnabled bool
	SkillID     int32
}

// UseBeastShot charges req.Summon (the caster's active pet or servitor) with
// the beast soulshot or spiritshot req.Template carries (resolved from its
// EtcItem handler name), consuming the summon's per-hit shot count from
// req.Item's stack on success. It reports BeastShotNotHandled for any
// handler name this path doesn't cover, so the caller's next branch still
// gets a chance to answer the client.
func UseBeastShot(req BeastShotUseRequest) BeastShotUseResult {
	if req.Template == nil || req.Template.EtcItem == nil {
		return BeastShotUseResult{Outcome: BeastShotNotHandled}
	}

	var kind modelitem.ShotKind
	switch req.Template.EtcItem.Handler {
	case BeastSoulShotsHandler:
		kind = modelitem.ShotSoul
	case BeastSpiritShotsHandler:
		if req.Template.ID == blessedBeastSpiritshotID {
			kind = modelitem.ShotBlessedSpirit
		} else {
			kind = modelitem.ShotSpirit
		}
	default:
		return BeastShotUseResult{Outcome: BeastShotNotHandled}
	}

	if req.CallerIsSummon {
		return BeastShotUseResult{Outcome: BeastShotCallerIsSummon}
	}
	if req.Summon == nil {
		return BeastShotUseResult{Outcome: BeastShotNoSummon}
	}
	if req.Summon.Dead() {
		return BeastShotUseResult{Outcome: BeastShotSummonDead}
	}
	if req.Summon.ChargedShot(kind) {
		return BeastShotUseResult{Outcome: BeastShotAlreadyCharged}
	}

	consume := req.Summon.SSCount()
	if kind != modelitem.ShotSoul {
		consume = req.Summon.SPSCount()
	}

	if _, ok := req.Destroyer.DestroyItem(req.Inventory, req.Item.ObjectID, consume); !ok {
		var autoEnabled bool
		if req.Caster != nil {
			autoEnabled = req.Caster.AutoSoulShotEnabled(req.Template.ID)
		}
		return BeastShotUseResult{Outcome: BeastShotNotEnoughItems, AutoEnabled: autoEnabled}
	}

	req.Summon.SetChargedShot(kind, true)

	var skillID int32
	if len(req.Template.AttachedSkills) > 0 {
		skillID = int32(req.Template.AttachedSkills[0].ID)
	}
	return BeastShotUseResult{Outcome: BeastShotApplied, SkillID: skillID}
}
