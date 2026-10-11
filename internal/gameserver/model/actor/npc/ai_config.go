package npc

// AIConfig holds the npcs.properties switches that shape which targets an
// NPC picks on its own.
type AIConfig struct {
	// MobAggroInPeaceZone (MobAggroInPeaceZone) lets an NPC other than a
	// Guard or FriendlyMonster pick a target standing in a peace zone.
	MobAggroInPeaceZone bool
	// GuardAttackAggroMob (GuardAttackAggroMob) lets a Guard pick an
	// aggressive Monster-family NPC.
	GuardAttackAggroMob bool
	// SeeRange (DefaultSeeRange) is how far an NPC other than a raid or
	// grand boss sees around itself; zero keeps the shipped 450.
	SeeRange int
}

// DefaultAIConfig is the shipped npcs.properties setting: peace-zone aggro
// allowed, guards ignore aggressive monsters, a see range of 450.
func DefaultAIConfig() AIConfig {
	return AIConfig{MobAggroInPeaceZone: true, SeeRange: DefaultSeeRange}
}

// SetAIConfig installs the target-selection switches h consults. Until set,
// h uses DefaultAIConfig.
func (h *Hostile) SetAIConfig(c AIConfig) {
	h.aiConfig.Store(&c)
}

func (h *Hostile) aiSettings() AIConfig {
	if c := h.aiConfig.Load(); c != nil {
		return *c
	}
	return DefaultAIConfig()
}
