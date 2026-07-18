package task

import "github.com/fatal10110/acis_golang/internal/gameserver/world"

type inactiveRegionSleeper interface {
	SleepWhenRegionInactive() bool
}

func regionActivity(state *world.State, actor world.Tracked) (placed, active bool) {
	if state == nil {
		return true, true
	}
	return state.RegionActivity(actor)
}

func sleepsWhenRegionInactive(actor world.Tracked) bool {
	sleeper, ok := actor.(inactiveRegionSleeper)
	return !ok || sleeper.SleepWhenRegionInactive()
}

func canWorkInRegion(state *world.State, actor world.Tracked) bool {
	placed, active := regionActivity(state, actor)
	if !placed {
		return false
	}
	return active || !sleepsWhenRegionInactive(actor)
}
