package item

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ScrollsOfResurrectionHandler is the etc-item handler name of the
// resurrection scrolls: using one casts its skill on the selected creature
// as an ordinary skill cast, which the skill's own consume item pays for.
const ScrollsOfResurrectionHandler = "ScrollsOfResurrection"

// ResurrectionScrollRefusal is why a resurrection scroll refuses its target
// before any of its skills is attempted.
type ResurrectionScrollRefusal uint8

const (
	// ResurrectionScrollAllowed means the scroll's skills go on to their
	// cast.
	ResurrectionScrollAllowed ResurrectionScrollRefusal = iota
	// ResurrectionScrollInvalidTarget means nothing, or no creature, is
	// selected.
	ResurrectionScrollInvalidTarget
	// ResurrectionScrollSiege means the dead player stands in an active
	// siege zone with no siege state.
	ResurrectionScrollSiege
	// ResurrectionScrollFestival means the dead player takes part in a
	// festival.
	ResurrectionScrollFestival
	// ResurrectionScrollPetOfferOpen means the dead player already has a
	// resurrection offer open for its pet.
	ResurrectionScrollPetOfferOpen
	// ResurrectionScrollAlreadyProposed means a resurrection offer for the
	// same creature is already open.
	ResurrectionScrollAlreadyProposed
	// ResurrectionScrollOwnerOfferOpen means the dead pet's owner already
	// has a resurrection offer open for itself.
	ResurrectionScrollOwnerOfferOpen
)

// ResurrectionScrollGate runs a resurrection scroll's own target checks for
// user against selected. Only a dead player or a dead pet is checked; any
// other creature is left to the skill's target type. A player's own dead
// pet is never refused here.
func ResurrectionScrollGate(userID int32, selected any) ResurrectionScrollRefusal {
	target, ok := selected.(skilltarget.Actor)
	if !ok || target == nil {
		return ResurrectionScrollInvalidTarget
	}
	if !target.Dead() {
		return ResurrectionScrollAllowed
	}
	switch target.Kind() {
	case actor.KindPlayer:
		return deadPlayerRefusal(target)
	case actor.KindSummon:
		if !target.IsPet() {
			return ResurrectionScrollAllowed
		}
		if owner, ok := target.Owner(); ok {
			return deadPetRefusal(userID, owner)
		}
	}
	return ResurrectionScrollAllowed
}

// deadPlayerRefusal checks a dead player target. One standing in an active
// siege zone with no siege state of its own is refused.
func deadPlayerRefusal(p skilltarget.Actor) ResurrectionScrollRefusal {
	if p.InSiegeZone() && p.SiegeState() == 0 {
		return ResurrectionScrollSiege
	}
	if p.FestivalParticipant() {
		return ResurrectionScrollFestival
	}
	if pending, forPet := p.ReviveOffer(); pending {
		if forPet {
			return ResurrectionScrollPetOfferOpen
		}
		return ResurrectionScrollAlreadyProposed
	}
	return ResurrectionScrollAllowed
}

// deadPetRefusal checks a dead pet target through its owner, unless user
// owns it. A pet's owner is a player, which is always a skill target.
func deadPetRefusal(userID int32, owner attackable.Combatant) ResurrectionScrollRefusal {
	if owner == nil || owner.ObjectID() == userID {
		return ResurrectionScrollAllowed
	}
	player, ok := owner.(skilltarget.Actor)
	if !ok {
		return ResurrectionScrollAllowed
	}
	if pending, forPet := player.ReviveOffer(); pending {
		if forPet {
			return ResurrectionScrollAlreadyProposed
		}
		return ResurrectionScrollOwnerOfferOpen
	}
	return ResurrectionScrollAllowed
}

// ResolveResurrectionScrollSkills returns every carried skill of tmpl that
// resolves to a definition, in template order, and whether tmpl is a
// resurrection scroll at all. Each one is cast as an ordinary skill: no
// item carries it, so the scroll is not consumed as the item used, only as
// the skill's own consume item.
func ResolveResurrectionScrollSkills(tmpl *modelitem.Template, defs actorcast.Definitions) ([]modelskill.Definition, bool) {
	if tmpl == nil || tmpl.Kind != modelitem.KindEtcItem || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != ScrollsOfResurrectionHandler {
		return nil, false
	}
	if defs == nil {
		return nil, true
	}
	out := make([]modelskill.Definition, 0, len(tmpl.AttachedSkills))
	for _, ref := range tmpl.AttachedSkills {
		if def, ok := defs.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)}); ok {
			out = append(out, def)
		}
	}
	return out, true
}
