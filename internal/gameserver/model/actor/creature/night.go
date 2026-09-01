package creature

import "sync"

// NightSource reports whether it is currently night in-game. *task.GameClock
// satisfies it; boot installs that single clock via SetNightSource.
type NightSource interface {
	IsNight() bool
}

var (
	nightMu     sync.RWMutex
	nightSource NightSource
)

// SetNightSource installs the in-game clock melee hit-chance reads for the
// night penalty. Call once at boot before any auto-attack can resolve.
func SetNightSource(src NightSource) {
	nightMu.Lock()
	defer nightMu.Unlock()
	nightSource = src
}

// Night reports whether it is currently night. Missing source is day.
func Night() bool {
	nightMu.RLock()
	defer nightMu.RUnlock()
	return nightSource != nil && nightSource.IsNight()
}
