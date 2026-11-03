package creature

// RegenMultipliers scale the base HP, MP and CP regeneration of a creature
// that is not raid related before its stat calculator finalizes them, as
// the players.properties HpRegenMultiplier, MpRegenMultiplier and
// CpRegenMultiplier keys configure them. Configured marks values read from
// the file, used as-is (an explicit 0 included); the zero value, never
// configured, leaves every base unchanged.
type RegenMultipliers struct {
	HP, MP, CP float64
	Configured bool
}

// DefaultRegenMultipliers is the shipped configuration: every base
// unchanged.
var DefaultRegenMultipliers = RegenMultipliers{HP: 1, MP: 1, CP: 1, Configured: true}

// InForce returns m, or DefaultRegenMultipliers when m was never
// configured.
func (m RegenMultipliers) InForce() RegenMultipliers {
	if !m.Configured {
		return DefaultRegenMultipliers
	}
	return m
}
