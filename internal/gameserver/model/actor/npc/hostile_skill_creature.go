package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/world"

// CurrentTarget reports nil: an NPC's target lives in its AI, not a
// player-style selection that aggression retargets.
func (h *Hostile) CurrentTarget() world.Tracked { return nil }

// SetTarget does nothing: aggression retargets playable creatures only.
func (h *Hostile) SetTarget(world.Tracked) {}

// AttackTarget does nothing: aggression retargets playable creatures only.
func (h *Hostile) AttackTarget(world.Tracked) {}
