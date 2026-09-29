package effect

func healStart(e *Effect) bool {
	target := e.Effected
	if !target.CanBeHealed() {
		return false
	}

	power := e.Template.Value + target.HealProficiency()
	amount := target.AddHP(power * target.HealEffectiveness() / 100)
	broadcastRestore(e.Effected, amount)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	broadcastRestore(e.Effected, target.AddHP(amount))
	notifyHealRestored(e, amount, false)
	return true
}

func healOverTimeAction(e *Effect) bool {
	target := e.Effected
	if !target.CanBeHealed() {
		return false
	}
	// A tick that healed nothing broadcasts nothing: the reference's HP
	// setter bypasses itself — and the status update with it — when the
	// applied amount is 0, which is every tick on an already-full target.
	if target.AddHP(e.Template.Value) > 0 {
		broadcastStatus(e.Effected)
	}
	return true
}

func healOverTimeStart(e *Effect) bool {
	if e.Template.Count <= 0 || e.Template.Time <= 0 {
		return true
	}
	if target, ok := asPlayer(e.Effected); ok {
		target.SendRegenMax(int32(e.Template.Count)*int32(e.Template.Time), int32(e.Template.Time), e.Template.Value)
	}
	return true
}

// broadcastStatus refreshes a player effected's own bars after an effect
// changed its vitals: nothing else tells the player, whether the change came
// from a periodic tick or from an instant effect landing inside a cast. A
// summon or NPC needs nothing here: its HP/MP mutators already republished
// its status (its targeters' health bar, and a summon's pet window and
// observers too), and a second push would double the frame.
func broadcastStatus(effected Actor) {
	if player, ok := asPlayer(effected); ok {
		player.BroadcastStatus()
	}
}

// broadcastRestore refreshes a player effected's own bars after one restore
// of an instant heal effect, when that restore applied anything: each of the
// start hook's two restores reports its own status, all of them ahead of the
// restored message.
func broadcastRestore(effected Actor, applied float64) {
	if applied > 0 {
		broadcastStatus(effected)
	}
}

func manaHealStart(e *Effect) bool {
	target := e.Effected
	if !target.CanBeHealed() {
		return false
	}

	amount := target.AddMP(target.RechargeMP(e.Template.Value))
	broadcastRestore(e.Effected, amount)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	broadcastRestore(e.Effected, target.AddMP(amount))
	notifyHealRestored(e, amount, true)
	return true
}

// notifyHealRestored tells a player target how much of one resource the
// first restore actually applied. The message uses that first applied
// amount even though the start hook adds it twice; a non-player target
// gets silence, matching the player-only send.
func notifyHealRestored(e *Effect, amount float64, mp bool) {
	notifier, ok := asPlayer(e.Effected)
	if !ok {
		return
	}
	name := ""
	if e.Effector != nil {
		name = e.Effector.CharacterName()
	}
	byOther := e.Effector != e.Effected
	restored := int(amount)
	if mp {
		notifier.NotifyMPRestored(name, restored, byOther)
		return
	}
	notifier.NotifyHPRestored(name, restored, byOther)
}
