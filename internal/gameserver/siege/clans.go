package siege

// MinClanLevel is the clan level a clan needs to register on a siege, which
// is also the level from which its leader holds the siege skills.
func (e *Engine) MinClanLevel() int {
	if e == nil {
		return DefaultConfig().MinClanLevel
	}
	return e.cfg.MinClanLevel
}

// Registration reports whether clanID is registered, on any side, on a
// castle's siege and whether the first such siege, in castle order, is in
// progress.
func (e *Engine) Registration(clanID int32) (registered, inProgress bool) {
	if e == nil {
		return false, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.order {
		if s.sideLocked(clanID) != SideNone {
			return true, s.status == StatusInProgress
		}
	}
	return false, false
}

// Fighting reports whether clanID is registered, on any side, on a siege in
// progress.
func (e *Engine) Fighting(clanID int32) bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.order {
		if s.status == StatusInProgress && s.sideLocked(clanID) != SideNone {
			return true
		}
	}
	return false
}

// Opposed reports whether clans a and b stand on opposite sides of a
// castle's siege (Siege.OnOppositeSides).
func (e *Engine) Opposed(a, b int32) bool {
	for _, s := range e.All() {
		if s.OnOppositeSides(a, b) {
			return true
		}
	}
	return false
}

// StateOf is the siege state a member of clanID takes as it enters the
// world or joins the clan: the attacker state when the clan attacks a siege
// in progress, the defender state when it owns or defends one, the last
// such siege in castle order deciding, and none otherwise.
func (e *Engine) StateOf(clanID int32) State {
	if e == nil {
		return StateNone
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	state := StateNone
	for _, s := range e.order {
		if s.status != StatusInProgress {
			continue
		}
		switch s.sideLocked(clanID) {
		case SideAttacker:
			state = StateAttacker
		case SideDefender, SideOwner:
			state = StateDefender
		}
	}
	return state
}

// Forget drops clanID's registration from every siege without storing it:
// a destroyed clan, whose rows go with the clan's own.
func (e *Engine) Forget(clanID int32) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.order {
		s.removeLocked(clanID)
	}
}
