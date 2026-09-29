package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// phoenixRevivePower is the exp-restore power of a Phoenix Blessing's
// resurrection offer: all of the lost exp comes back.
const phoenixRevivePower = 100

// Reviver is who offers a resurrection: its name goes into the offer, and a
// refused offer is reported back to it.
type Reviver interface {
	CharacterName() string
	NotifyReviveRefused(event.ReviveRefusal)
}

// NotifyReviveRefused tells this character that its resurrection offer was
// refused, and why.
func (c *Character) NotifyReviveRefused(reason event.ReviveRefusal) {
	c.emit(event.ReviveRefused{Reason: reason})
}

// ReviveRequest offers this character a resurrection by reviver, for itself
// or (isPet) for its dead pet. power is the exp-restore power a skill's
// revive would carry; a Phoenix Blessing on whoever is being revived makes
// it 100 instead. Only one offer is open at a time: a second one is refused
// to its reviver, and an offer for a character (or pet) that is not dead is
// not made at all.
func (c *Character) ReviveRequest(reviver Reviver, power float64, isPet bool) {
	c.reviveMu.Lock()
	if c.reviveRequested {
		pet := c.revivePet
		c.reviveMu.Unlock()
		switch {
		case pet == isPet:
			reviver.NotifyReviveRefused(event.ReviveAlreadyProposed)
		case isPet:
			reviver.NotifyReviveRefused(event.RevivePetWhileOwnerPending)
		default:
			reviver.NotifyReviveRefused(event.ReviveOwnerWhilePetPending)
		}
		return
	}
	var blessed bool
	if isPet {
		summon, ok := c.deadSummon()
		if !ok {
			c.reviveMu.Unlock()
			return
		}
		blessed = summon.EffectList().IsAffected(effect.FlagPhoenixBlessing)
	} else {
		if !c.dead.Load() {
			c.reviveMu.Unlock()
			return
		}
		blessed = c.EffectList().IsAffected(effect.FlagPhoenixBlessing)
	}
	if blessed {
		power = phoenixRevivePower
	}
	c.reviveRequested, c.revivePower, c.revivePet = true, power, isPet
	c.reviveMu.Unlock()

	c.emit(event.ReviveRequested{ReviverName: reviver.CharacterName()})
}

// OfferSummonRevive offers this character the resurrection of its summon,
// which just died under a Phoenix Blessing.
func (c *Character) OfferSummonRevive() {
	c.ReviveRequest(c, 0, true)
}

// ReviveAnswer applies this character's answer to its pending resurrection
// offer (1 accepts). Nothing happens without an offer, or while whoever the
// offer is for is no longer dead; the offer then stays open. Declining uses
// up this character's own Phoenix Blessing. Accepting an offer for this
// character restores the offer's share of the lost exp and revives it;
// accepting one for its pet revives the pet the same way. Either way the
// offer is then closed.
//
// The pet is revived once the offer is closed and reviveMu released: a
// revived pet closes its owner's offer itself (ClearReviveOffer).
func (c *Character) ReviveAnswer(answer int32) {
	c.reviveMu.Lock()
	if !c.reviveRequested {
		c.reviveMu.Unlock()
		return
	}
	var pet reviveSummon
	if c.revivePet {
		summon, ok := c.summonActor()
		if ok && !summon.Dead() {
			c.reviveMu.Unlock()
			return
		}
		pet = summon
	} else if !c.dead.Load() {
		c.reviveMu.Unlock()
		return
	}

	switch {
	case answer == 0 && c.EffectList().IsAffected(effect.FlagPhoenixBlessing):
		c.stopPhoenixBlessing()
	case answer == 1 && !c.revivePet:
		if c.revivePower != 0 {
			c.reviveRestoringExp(c.revivePower)
		} else {
			c.revive()
		}
	}
	power := c.revivePower
	c.reviveRequested, c.revivePower = false, 0
	c.reviveMu.Unlock()

	if answer != 1 || pet == nil {
		return
	}
	if power != 0 {
		pet.ReviveRestoringExp(power)
	} else {
		pet.Revive()
	}
}

// ClearReviveOffer closes this character's pending resurrection offer, if
// any.
func (c *Character) ClearReviveOffer() {
	c.reviveMu.Lock()
	c.reviveRequested, c.revivePower = false, 0
	c.reviveMu.Unlock()
}

// reviveSummon is the part of a summon a resurrection offer reads and an
// accepted one revives.
type reviveSummon interface {
	Dead() bool
	EffectList() *effect.List
	Revive() bool
	ReviveRestoringExp(power float64) bool
}

// summonActor returns c's summon in the world, dead or alive.
func (c *Character) summonActor() (reviveSummon, bool) {
	if c.world == nil {
		return nil, false
	}
	obj, ok := c.world.Summon(c.ObjectID())
	if !ok {
		return nil, false
	}
	summon, ok := obj.(reviveSummon)
	return summon, ok
}

// deadSummon returns c's summon when it is dead.
func (c *Character) deadSummon() (reviveSummon, bool) {
	summon, ok := c.summonActor()
	if !ok || !summon.Dead() {
		return nil, false
	}
	return summon, true
}
