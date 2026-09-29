package effect

func healStart(e *Effect) bool {
	target := e.Effected
	if !target.CanBeHealed() {
		return false
	}

	power := e.Template.Value + target.HealProficiency()
	amount := target.AddHP(power * target.HealEffectiveness() / 100)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	target.AddHP(amount)
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

// broadcastStatus refreshes a player effected's own bars after a periodic
// tick changed its vitals. A periodic effect action runs outside any client
// request, so unlike the cast and item paths — which send their own batched
// StatusUpdate at the call site — nothing else would tell the player the
// tick happened. A summon or NPC needs nothing here: its HP/MP mutators
// already republished its status (the owner's pet window and observers, or
// the targeters' health bar), and a second push would double the frame.
func broadcastStatus(effected Actor) {
	if player, ok := asPlayer(effected); ok {
		player.BroadcastStatus()
	}
}

func manaHealStart(e *Effect) bool {
	target := e.Effected
	if !target.CanBeHealed() {
		return false
	}

	amount := target.AddMP(target.RechargeMP(e.Template.Value))
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	target.AddMP(amount)
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
