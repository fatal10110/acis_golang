package gameservertest

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"

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
