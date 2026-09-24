package conditions

import "github.com/fatal10110/acis_golang/internal/commons/rnd"

// GameChance passes with the given percent chance (0-100) on every test —
// re-rolled independently each time, not cached per attempt.
type GameChance struct{ Percent int }

func (c GameChance) Test(effector, effected Actor, skill Skill) bool {
	return rnd.Get(100) < c.Percent
}

// NightSource reports whether it is currently night in-game; *task.GameClock
// satisfies it directly.
type NightSource interface {
	IsNight() bool
}

// GameTime requires the current in-game time of day on the effector's
// server to match Night.
type GameTime struct {
	Night bool
}

func (c GameTime) Test(effector, effected Actor, skill Skill) bool {
	return effector.IsNight() == c.Night
}
