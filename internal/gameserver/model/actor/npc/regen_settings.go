package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"

// RegenSettings answers the base regeneration multipliers in force, which
// //reload config may change while the NPC lives.
type RegenSettings interface {
	RegenMultipliers() creature.RegenMultipliers
}

// regenInForce returns the multipliers s puts in force, the shipped ones
// for a nil s.
func regenInForce(s RegenSettings) creature.RegenMultipliers {
	if s == nil {
		return creature.DefaultRegenMultipliers
	}
	return s.RegenMultipliers()
}

// PartySettings answers the party range in force, which //reload config
// may change while the NPC lives.
type PartySettings interface {
	PartyRange() int
}
