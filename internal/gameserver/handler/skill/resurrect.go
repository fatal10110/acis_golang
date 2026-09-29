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

// summonReviver is a summon a resurrection revives outright.
type summonReviver interface {
	ReviveRestoringExp(power float64) bool
	CancelDecay()
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
// through its owner. The caster's own dead pet and any dead servitor are
// revived outright, a pet getting the revive power's share of its lost exp
// back; a servitor revived this way keeps its pending decay. Any other
// caster revives a dead player or summon outright, restoring the revive
// power's share of the lost exp, and drops the summon's decay first. The
// spiritshot is spent either way.
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
			reviveOrOfferSummon(cast, obj, reviver, power)
		}
	}
}

// reviveOrOfferSummon revives a dead servitor or the caster's own dead pet,
// and sends another player's dead pet's resurrection offer to its owner.
func reviveOrOfferSummon(cast Cast, obj Actor, reviver player.Reviver, power float64) {
	s, ok := obj.(Summon)
	if !ok {
		return
	}
	target, ok := obj.(summonReviver)
	if !ok {
		return
	}
	if identity, ok := obj.(petIdentity); !ok || !identity.IsPet() {
		target.ReviveRestoringExp(power)
		return
	}
	owner, ok := s.SummonOwner().(reviveRequestTarget)
	if !ok {
		return
	}
	if owner.ObjectID() == cast.Caster.ObjectID() {
		target.ReviveRestoringExp(power)
		return
	}
	owner.ReviveRequest(reviver, power, true)
}

// reviveTargets revives every dead player and summon target outright. A
// summon's decay is dropped first, whether or not it is still dead.
func reviveTargets(cast Cast, power float64) {
	for _, obj := range cast.Targets {
		if s, ok := obj.(summonReviver); ok && obj.Kind() == actor.KindSummon {
			s.CancelDecay()
			s.ReviveRestoringExp(power)
			continue
		}
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
