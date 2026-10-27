package gameservertest

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// WithRegenMultipliers sets the players.properties HpRegenMultiplier,
// MpRegenMultiplier and CpRegenMultiplier (default 1 each, the shipped
// values) the characters, summons, NPCs and doors read.
func WithRegenMultipliers(hp, mp, cp float64) Option {
	return func(o *options) {
		o.regenMultipliers = creature.RegenMultipliers{HP: hp, MP: mp, CP: cp, Configured: true}
	}
}

// WithPartyRange sets the players.properties PartyRange (default 1500, the
// shipped value) the link, the kill rewards and the hostiles share.
func WithPartyRange(r int) Option {
	return func(o *options) { o.partyRange = r }
}

// WithEventRules sets the events.properties settings the event quests
// read, NeededPartyMembers and the Dimensional Rift settings (default
// script.DefaultEventRules, the shipped values), shared by the link and
// the scripts.
func WithEventRules(r script.EventRules) Option {
	return func(o *options) { o.eventRules = &r }
}

// fixtureEventRules are the event settings WithEventRules set, else the
// shipped ones.
func fixtureEventRules(o *options) script.EventRules {
	if o.eventRules != nil {
		return *o.eventRules
	}
	return script.DefaultEventRules()
}
