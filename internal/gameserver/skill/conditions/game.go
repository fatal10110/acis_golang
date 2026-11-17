package conditions

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
