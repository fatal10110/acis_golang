package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// reviveRequestTarget is a character that can be offered a resurrection,
// for itself or for its pet.
type reviveRequestTarget interface {
	ObjectID() int32
	ReviveRequest(reviver player.Reviver, power float64, isPet bool)
}

// petIdentity tells a pet from a servitor.
type petIdentity interface {
	IsPet() bool
}

var (
	_ reviveRequestTarget = (*player.Character)(nil)
	_ player.Reviver      = (*player.Character)(nil)
)

type resurrectHandler struct{}

func (resurrectHandler) Types() []string { return []string{"RESURRECT"} }

// Use resurrects the resolved targets at the caster's revive power (the
// skill's power scaled by the caster's WIT). A player caster asks first: a
// dead player gets the offer itself, and another player's dead pet gets it
// through its owner. Any other caster revives a dead player outright,
// restoring the revive power's share of the lost exp.
//
// A player's own dead pet, a servitor, and any caster reviving a summon
// revive it outright; summon revival is not modeled yet (#2679), so those
// targets are left dead. The spiritshot is spent either way.
func (resurrectHandler) Use(cast Cast) {
	if cast.Caster != nil {
		power := formulas.RevivePower(statbonus.WITBonus[cast.Caster.WIT()], float64(cast.Skill.Power))
		if reviver, ok := playerReviver(cast.Caster); ok {
			offerRevives(cast, reviver, power)
		} else {
			reviveTargets(cast, power)
		}
	}
	dischargeSpiritshot(cast)
}

func playerReviver(caster Creature) (player.Reviver, bool) {
	if caster.Kind() != actor.KindPlayer {
		return nil, false
	}
	reviver, ok := caster.(player.Reviver)
	return reviver, ok
}

// offerRevives sends every target's resurrection offer from a player
// caster.
func offerRevives(cast Cast, reviver player.Reviver, power float64) {
	for _, obj := range cast.Targets {
		if obj == nil {
			continue
		}
		switch obj.Kind() {
		case actor.KindPlayer:
			if target, ok := obj.(reviveRequestTarget); ok {
				target.ReviveRequest(reviver, power, false)
			}
		case actor.KindSummon:
			offerPetRevive(cast, obj, reviver, power)
		}
	}
}

// offerPetRevive sends a dead pet's resurrection offer to its owner, unless
// the caster owns it.
func offerPetRevive(cast Cast, obj Actor, reviver player.Reviver, power float64) {
	pet, ok := obj.(Summon)
	if !ok {
		return
	}
	if identity, ok := obj.(petIdentity); !ok || !identity.IsPet() {
		return
	}
	owner, ok := pet.SummonOwner().(reviveRequestTarget)
	if !ok || owner.ObjectID() == cast.Caster.ObjectID() {
		return
	}
	owner.ReviveRequest(reviver, power, true)
}

// reviveTargets revives every dead player target outright.
func reviveTargets(cast Cast, power float64) {
	for _, obj := range cast.Targets {
		target, ok := asPlayer(obj)
		if !ok {
			continue
		}
		// The hit reuses the launch-time targets without re-checking them,
		// so a target may have gone back to town since. Only a player who
		// is still dead gets the exp back and the revive.
		target.ReviveRestoringExp(power)
	}
}
