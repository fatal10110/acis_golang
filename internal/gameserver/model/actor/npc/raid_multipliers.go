package npc

// RaidMultipliers scale the base defences and regeneration of a raid-related
// NPC (a raid or grand boss and its minions) before its stat calculator
// finalizes them, as configured by the npcs.properties RaidDefenceMultiplier,
// RaidHpRegenMultiplier and RaidMpRegenMultiplier keys.
type RaidMultipliers struct {
	Defence, HPRegen, MPRegen float64
}

// DefaultRaidMultipliers leaves every raid-related base unchanged.
func DefaultRaidMultipliers() RaidMultipliers {
	return RaidMultipliers{Defence: 1, HPRegen: 1, MPRegen: 1}
}

// SetRaidMultipliers installs the raid base multipliers h applies while it
// is raid related. Until set, h uses DefaultRaidMultipliers.
func (h *Hostile) SetRaidMultipliers(m RaidMultipliers) {
	h.raidMultipliers.Store(&m)
}

// raidBaseMultipliers returns the multipliers h's base defences and
// regeneration take: the installed ones while h is raid related, otherwise
// DefaultRaidMultipliers.
func (h *Hostile) raidBaseMultipliers() RaidMultipliers {
	if m := h.raidMultipliers.Load(); m != nil && h.RaidRelated() {
		return *m
	}
	return DefaultRaidMultipliers()
}
