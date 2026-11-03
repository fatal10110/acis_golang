package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

var _ player.Reviver = (*player.Character)(nil)

type resurrectHandler struct{}

func (resurrectHandler) Types() []string { return []string{"RESURRECT"} }

// Use resurrects the resolved targets at the caster's revive power (the
// skill's power scaled by the caster's WIT). A player caster asks first: a
// dead player gets the offer itself, and another player's dead pet gets it
// through its owner. The caster's own dead pet and any dead servitor are
// revived outright, a pet getting the revive power's share of its lost exp
// back; a servitor revived this way keeps its pending decay. Any other
// caster revives a dead player or summon outright, restoring the revive
// power's share of the lost exp; a summon it revives drops its decay. The
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
			if target, ok := asPlayer(obj); ok {
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
	s, ok := asSummon(obj)
	if !ok {
		return
	}
	if !s.IsPet() {
		s.ReviveRestoringExp(power)
		return
	}
	owner, ok := asPlayer(s.SummonOwner())
	if !ok {
		return
	}
	if owner.ObjectID() == cast.Caster.ObjectID() {
		s.ReviveRestoringExp(power)
		return
	}
	owner.ReviveRequest(reviver, power, true)
}

// reviveTargets revives every dead player and summon target outright. A
// summon revives on its own queue and drops its decay, one whose owner is
// offline included.
func reviveTargets(cast Cast, power float64) {
	for _, obj := range cast.Targets {
		if s, ok := asSummon(obj); ok {
			s.ResurrectOutright(power)
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
