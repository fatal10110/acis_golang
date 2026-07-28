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
	target.AddHP(e.Template.Value)
	return true
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
