package item

import (
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// FishShotsHandler is the etc-item handler name of fishing shots.
const FishShotsHandler = "FishShots"

// FishShotCharger is the player charging a fishing shot on the fishing rod
// in its hand.
type FishShotCharger interface {
	// FishingRod returns the rod's grade; ok is false without a rod.
	FishingRod() (grade modelitem.CrystalType, ok bool)
	ChargedShot(kind modelitem.ShotKind) bool
	SetChargedShot(kind modelitem.ShotKind, charged bool)
}

// FishShotOutcome classifies one UseFishShot attempt.
type FishShotOutcome uint8

const (
	// FishShotApplied means the rod was charged and one shot used up.
	FishShotApplied FishShotOutcome = iota
	// FishShotNoRod means no fishing rod is in hand.
	FishShotNoRod
	// FishShotAlreadyCharged means the rod already carries a fishing shot.
	FishShotAlreadyCharged
	// FishShotGradeMismatch means the shot's grade is not the rod's.
	FishShotGradeMismatch
	// FishShotNotEnoughItems means the shot could not be used up.
	FishShotNotEnoughItems
)

// FishShotUseRequest carries what UseFishShot needs to charge one fishing
// shot.
type FishShotUseRequest struct {
	Caster    FishShotCharger
	Inventory *itemcontainer.Inventory
	Item      *modelitem.Instance
	Template  *modelitem.Template
	Destroyer InventoryDestroyer
}

// UseFishShot charges the caster's fishing rod with one fishing shot of
// req.Item's stack: the rod must be in hand, not already charged and of the
// shot's grade. SkillID is the charge visual to show on FishShotApplied (0
// when the template attaches none).
func UseFishShot(req FishShotUseRequest) (outcome FishShotOutcome, skillID int32) {
	grade, ok := req.Caster.FishingRod()
	if !ok {
		return FishShotNoRod, 0
	}
	if req.Caster.ChargedShot(modelitem.ShotFishSoul) {
		return FishShotAlreadyCharged, 0
	}
	if grade != req.Template.Crystal {
		return FishShotGradeMismatch, 0
	}
	if _, ok := req.Destroyer.DestroyItem(req.Inventory, req.Item.ObjectID, 1); !ok {
		return FishShotNotEnoughItems, 0
	}
	req.Caster.SetChargedShot(modelitem.ShotFishSoul, true)
	if len(req.Template.AttachedSkills) > 0 {
		skillID = int32(req.Template.AttachedSkills[0].ID)
	}
	return FishShotApplied, skillID
}
