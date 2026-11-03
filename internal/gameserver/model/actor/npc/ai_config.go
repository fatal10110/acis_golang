package npc

import "sync/atomic"

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

// HostileSettings holds the AI switches and raid multipliers in force for
// every hostile that uses it (UseSettings). Each hostile reads it at every
// use, so storing a new value reaches all of them at once. The zero value
// holds DefaultAIConfig and DefaultRaidMultipliers.
type HostileSettings struct {
	v atomic.Pointer[hostileSettingsValue]
}

type hostileSettingsValue struct {
	ai   AIConfig
	raid RaidMultipliers
}

// defaultSettings is what a hostile with no settings installed reads.
var defaultSettings HostileSettings

// NewHostileSettings returns a holder of c and m.
func NewHostileSettings(c AIConfig, m RaidMultipliers) *HostileSettings {
	s := &HostileSettings{}
	s.v.Store(&hostileSettingsValue{ai: c, raid: m})
	return s
}

// AI returns the AI switches in force.
func (s *HostileSettings) AI() AIConfig {
	if v := s.v.Load(); v != nil {
		return v.ai
	}
	return DefaultAIConfig()
}

// Raid returns the raid multipliers in force.
func (s *HostileSettings) Raid() RaidMultipliers {
	if v := s.v.Load(); v != nil {
		return v.raid
	}
	return DefaultRaidMultipliers()
}

// UseSettings makes h read its AI switches and raid multipliers from s, a
// holder it shares with every other hostile using it. Until set, h uses
// DefaultAIConfig and DefaultRaidMultipliers.
func (h *Hostile) UseSettings(s *HostileSettings) {
	h.settings.Store(s)
}

// SetAIConfig gives h a holder of its own, with the target-selection
// switches c and the raid multipliers h has now.
func (h *Hostile) SetAIConfig(c AIConfig) {
	h.settings.Store(NewHostileSettings(c, h.hostileSettings().Raid()))
}

// hostileSettings returns the holder h reads its settings from.
func (h *Hostile) hostileSettings() *HostileSettings {
	if s := h.settings.Load(); s != nil {
		return s
	}
	return &defaultSettings
}

func (h *Hostile) aiSettings() AIConfig {
	return h.hostileSettings().AI()
}
