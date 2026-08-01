package effect

func healStart(e *Effect) bool {
	target, ok := e.Effected.(instantHealTarget)
	if !ok || !target.CanBeHealed() {
		return false
	}

	power := e.Template.Value
	if p, ok := e.Effected.(healProficiencyTarget); ok {
		power += p.HealProficiency()
	}
	effectiveness := 100.0
	if eff, ok := e.Effected.(healEffectivenessTarget); ok {
		effectiveness = eff.HealEffectiveness()
	}

	amount := target.AddHP(power * effectiveness / 100)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	target.AddHP(amount)
	return true
}

func healOverTimeAction(e *Effect) bool {
	target, ok := e.Effected.(instantHealTarget)
	if !ok || !target.CanBeHealed() {
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
	if !isPlayer(e.Effected) || e.Template.Count <= 0 || e.Template.Time <= 0 {
		return true
	}
	if target, ok := e.Effected.(regenMaxSender); ok {
		target.SendRegenMax(int32(e.Template.Count)*int32(e.Template.Time), int32(e.Template.Time), e.Template.Value)
	}
	return true
}

// broadcastStatus refreshes effected's health bars for everyone watching.
// A periodic effect action runs outside any client request, so unlike the
// cast and item paths — which send their own batched StatusUpdate at the
// call site — nothing else would tell the client the tick happened. Actors
// with no broadcast hook are left alone.
func broadcastStatus(effected any) {
	if b, ok := effected.(statusBroadcaster); ok {
		b.BroadcastStatus()
	}
}

func manaHealStart(e *Effect) bool {
	target, ok := e.Effected.(manaHealTarget)
	if !ok || !target.CanBeHealed() {
		return false
	}

	power := e.Template.Value
	if r, ok := e.Effected.(rechargeRateTarget); ok {
		power = r.RechargeMP(power)
	}

	amount := target.AddMP(power)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	target.AddMP(amount)
	return true
}

// chargesTarget is implemented by an actor that tracks Force/Soul charges.
