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

// SetRaidMultipliers gives h a holder of its own, with the raid base
// multipliers m it applies while it is raid related and the AI switches h
// has now.
func (h *Hostile) SetRaidMultipliers(m RaidMultipliers) {
	h.settings.Store(NewHostileSettings(h.aiSettings(), m))
}

// raidBaseMultipliers returns the multipliers h's base defences and
// regeneration take: the installed ones while h is raid related, otherwise
// DefaultRaidMultipliers.
func (h *Hostile) raidBaseMultipliers() RaidMultipliers {
	if h.RaidRelated() {
		return h.hostileSettings().Raid()
	}
	return DefaultRaidMultipliers()
}
