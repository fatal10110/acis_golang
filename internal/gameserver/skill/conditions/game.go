package conditions

import "github.com/fatal10110/acis_golang/internal/commons/rnd"

// GameChance passes with the given percent chance (0-100) on every test —
// re-rolled independently each time, not cached per attempt.
type GameChance struct{ Percent int }

func (c GameChance) Test(effector, effected, skill any) bool {
	return rnd.Get(100) < c.Percent
}

// NightSource reports whether it is currently night in-game; *task.GameClock
// satisfies it directly.
type NightSource interface {
	IsNight() bool
}

// GameTime requires the current in-game time of day to match Night.
// Clock supplies "is it night right now" as an explicit dependency.
type GameTime struct {
	Clock NightSource
	Night bool
}

func (c GameTime) Test(effector, effected, skill any) bool {
	return c.Clock.IsNight() == c.Night
}
