package creature

import "sync"

// Health guards one actor's current hit points.
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

// Damage applies non-negative damage, clamps at zero, and reports whether
// this damage newly reached zero.
func (h *Health) Damage(dmg int) bool {
	if dmg < 0 {
		dmg = 0
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil || *h.current <= 0 {
		return false
	}

	*h.current -= float64(dmg)
	if *h.current > 0 {
		return false
	}
	*h.current = 0
	return true
}
