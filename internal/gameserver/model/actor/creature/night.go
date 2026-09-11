package creature

import "sync/atomic"

// NightSource reports whether it is currently night in-game. *task.GameClock
// satisfies it; boot installs that single clock via SetNightSource.
type NightSource interface {
	IsNight() bool
}

// nightSource is read on every melee hit-chance roll, so it is an atomic
// load rather than a shared lock every attacking goroutine contends on.
var nightSource atomic.Pointer[NightSource]

// SetNightSource installs the in-game clock melee hit-chance reads for the
// night penalty. Call once at boot before any auto-attack can resolve.
func SetNightSource(src NightSource) {
	nightSource.Store(&src)
}

// Night reports whether it is currently night. Missing source is day.
func Night() bool {
	src := nightSource.Load()
	return src != nil && *src != nil && (*src).IsNight()
}
