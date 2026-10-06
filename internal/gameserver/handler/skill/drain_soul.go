package skill

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// enhanceWeaponQuest is the quest whose started state lets a soul crystal's
// skill charge the crystal.
const enhanceWeaponQuest = "Q350_EnhanceYourWeapon"

// soulDrainer is a player caster the drain soul check reads: its quest
// journal and its inventory.
type soulDrainer interface {
	QuestStarted(name string) bool
	HoldsItem(objectID int32) bool
}

// soulAbsorber is a monster that records the players charging soul
// crystals on it.
type soulAbsorber interface {
	RegisterAbsorber(playerID int32, holds func(itemObjectID int32) bool)
}

type drainSoulHandler struct{}

func (drainSoulHandler) Types() []string { return []string{"DRAIN_SOUL"} }

// Use registers a living player caster with its quest started as an
// absorber of its first target, when that target is a living monster
// strictly within the skill's effect range of the caster, centre to centre
// in 3D. Anything else does nothing.
func (drainSoulHandler) Use(cast Cast) {
	caster, ok := asPlayer(cast.Caster)
	if !ok || caster.Dead() {
		return
	}
	drainer, ok := caster.(soulDrainer)
	if !ok || !drainer.QuestStarted(enhanceWeaponQuest) || len(cast.Targets) == 0 {
		return
	}
	target, ok := asNPC(cast.Targets[0])
	if !ok || !target.MonsterKind() || target.Dead() {
		return
	}
	absorber, ok := target.(soulAbsorber)
	if !ok {
		return
	}
	cx, cy, cz := caster.Position()
	tx, ty, tz := target.Position()
	if !location.In3DRadius(cx, cy, cz, tx, ty, tz, cast.Skill.EffectRange) {
		return
	}
	absorber.RegisterAbsorber(caster.ObjectID(), drainer.HoldsItem)
}
