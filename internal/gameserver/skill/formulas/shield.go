package formulas

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"

// ShieldDefense is the outcome of a shield-block attempt.
type ShieldDefense uint8

const (
	ShieldFailed ShieldDefense = iota
	ShieldSuccess
	ShieldPerfect
)

// ShieldUse resolves a shield-block roll. Call sites that already know the
// attempt can't possibly succeed — the attacking skill ignores shields
// entirely, the target has no shield equipped, or the target isn't facing
// the attacker closely enough — should short-circuit to ShieldFailed
// without calling this at all.
//
// baseRate is the target's already-computed SHIELD_RATE stat (0 always
// fails); dex is the target's DEX; isBow reports whether the attacker
// wields a bow (triples the rate); isCrit reports whether this is a
// critical hit (also triples the rate); perfectBlockRate is the
// configured flat percent chance of an unconditional perfect block; roll
// is a uniform random draw in [0, 100).
func ShieldUse(baseRate float64, dex int, isBow, isCrit bool, perfectBlockRate, roll int) ShieldDefense {
	if baseRate == 0 {
		return ShieldFailed
	}

	rate := baseRate * statbonus.DEXBonus[dex]
	if isBow {
		rate *= 3
	}
	if isCrit {
		rate *= 3
	}

	switch {
	case roll < perfectBlockRate:
		return ShieldPerfect
	case float64(roll) < rate:
		return ShieldSuccess
	default:
		return ShieldFailed
	}
}
