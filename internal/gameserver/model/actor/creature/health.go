package creature

import "sync"

// DeathHP is the half-point death threshold every creature shares: HP that
// falls below it is a death, not just HP that reaches zero. Fractional current
// HP is ordinary (regeneration and heals write fractions straight into it),
// so a remainder in (0, DeathHP) after a blow is already dead.
const DeathHP = 0.5

// Health guards one actor's current hit points. An attacker's hit damages
// them from the attacker's queue.
type Health struct {
	mu      sync.Mutex
	current *float64
}

// NewHealth returns a Health component backed by current.
func NewHealth(current *float64) Health {
	return Health{current: current}
}

// Bind points h at current. It lets restored or literal actors bind after
// construction.
func (h *Health) Bind(current *float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = current
}

// Current returns the current hit points, or zero when h is not bound.
func (h *Health) Current() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil {
		return 0
	}
	return *h.current
}

// SetCurrent overrides current hit points unless the actor is already dead.
func (h *Health) SetCurrent(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil || *h.current <= 0 {
		return
	}
	*h.current = v
}

// Add restores non-negative hit points up to max and returns the applied amount.
func (h *Health) Add(amount, max float64) float64 {
	if amount <= 0 {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil || *h.current >= max {
		return 0
	}
	if *h.current+amount > max {
		amount = max - *h.current
	}
	*h.current += amount
	return amount
}

// Damage applies non-negative damage, clamps at zero, and reports whether
// this damage newly left HP below DeathHP.
func (h *Health) Damage(dmg int) bool {
	if dmg < 0 {
		dmg = 0
	}
	return h.DamageValue(float64(dmg))
}

// DamageValue applies non-negative fractional damage, clamps at zero, and
// reports whether this damage newly left HP below DeathHP.
func (h *Health) DamageValue(dmg float64) bool {
	if dmg < 0 {
		dmg = 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil || *h.current <= 0 {
		return false
	}

	*h.current -= dmg
	if *h.current >= DeathHP {
		return false
	}
	*h.current = 0
	return true
}
