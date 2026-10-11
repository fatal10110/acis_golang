package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// CleanAllHate zeroes the hate of every threat entry, keeping the entries
// and their damage, and drops every queued attack desire. Unlike
// StopAggroHate it never sends the NPC back to peace.
func (a *Attackable) CleanAllHate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, t := range a.threats.Snapshot() {
		a.threats.StopHate(t.Attacker)
	}
	a.desires.RemoveIf(func(d *Desire) bool { return d.Kind == IntentionAttack })
}

// CanAttack reports whether the actor may physically attack target now, as
// its attack controller's gate decides.
func (a *Attackable) CanAttack(target attackable.Combatant) bool {
	return a.attack.CanAttack(target)
}
