package summon

// AI returns the brain Attach installed, nil before Attach or when none was
// given. It is set once before the summon is published, so any goroutine may
// read it.
func (a *Actor) AI() AI { return a.brain }
