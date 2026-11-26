package npc

// SetRaidMultipliers gives h a holder of its own, with the raid base
// multipliers m it applies while it is raid related and the AI switches h
// has now.
func (h *Hostile) SetRaidMultipliers(m RaidMultipliers) {
	h.settings.Store(NewHostileSettings(h.aiSettings(), m))
}
