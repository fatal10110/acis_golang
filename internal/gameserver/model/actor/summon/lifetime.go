package summon

// LifetimeState is a servitor's remaining-time and item-consumption
// countdown, ticked once per second while it is active.
type LifetimeState struct {
	// TimeRemaining and TotalLifeTime are in the same time unit (the
	// servitor's summon skill defines it in milliseconds).
	TimeRemaining int
	TotalLifeTime int
	// NextItemConsumeTime is the TimeRemaining value at which the next
	// upkeep item is due; -1 means this servitor never consumes one.
	NextItemConsumeTime int
	// ItemConsumeSteps is how many upkeep checkpoints the servitor's
	// total lifetime is divided into.
	ItemConsumeSteps int
}

// InitialNextConsumeTime returns the countdown value a freshly summoned
// servitor's NextItemConsumeTime starts at. A servitor with no consume item
// or zero consume steps never consumes one, signaled by -1.
func InitialNextConsumeTime(totalLifeTime, itemConsumeSteps, itemConsumeItemID int) int {
	if itemConsumeItemID == 0 || itemConsumeSteps == 0 {
		return -1
	}
	return totalLifeTime - totalLifeTime/(itemConsumeSteps+1)
}

// Tick advances state by one lifetime tick that costs cost time units
// (higher while the servitor's owner is in combat). It returns the updated
// state, whether the servitor's lifetime has now expired, and whether this
// tick crossed an upkeep checkpoint. On expiry the state is returned
// unmodified past TimeRemaining; the caller unsummons the servitor rather
// than continuing to tick it. On a checkpoint the caller is responsible for
// taking the upkeep item from the owner and unsummoning the servitor if
// that fails, exactly as it would for any other command outcome.
func Tick(state LifetimeState, cost int) (next LifetimeState, expired bool, dueForUpkeep bool) {
	next = state
	oldRemaining := state.TimeRemaining
	next.TimeRemaining = state.TimeRemaining - cost

	if next.TimeRemaining < 0 {
		return next, true, false
	}

	if next.TimeRemaining <= state.NextItemConsumeTime && oldRemaining > state.NextItemConsumeTime {
		next.NextItemConsumeTime = state.NextItemConsumeTime - state.TotalLifeTime/(state.ItemConsumeSteps+1)
		dueForUpkeep = true
	}
	return next, false, dueForUpkeep
}
