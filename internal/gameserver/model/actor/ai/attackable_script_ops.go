package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// CleanAllHate zeroes the hate of every threat entry, keeping the entries
// and their damage, and drops every queued attack desire. Unlike
// StopAggroHate it never sends the NPC back to peace.
func (a *Attackable) CleanAllHate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, t := range a.threats.Load().Snapshot() {
		a.threats.Load().StopHate(t.Attacker)
	}
	a.desires.Load().RemoveIf(func(d *Desire) bool { return d.Kind == IntentionAttack })
}

// CanAttack reports whether the actor may physically attack target now, as
// its attack controller's gate decides.
func (a *Attackable) CanAttack(target attackable.Combatant) bool {
	c := a.attackController()
	return c != nil && c.CanAttack(target)
}

// RemoveAllDesire drops every queued desire and zeroes the hate of every
// threat and hate-list entry, keeping the entries and their damage. It
// never sends the NPC back to peace.
func (a *Attackable) RemoveAllDesire() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.desires.Load().Clear()
	a.threats.Load().ZeroHate()
	a.hates.Load().ZeroHate()
}

// CleanAllHates zeroes the hate of every hate-list entry, keeping the
// entries, and drops the cast desires aimed at each of them.
func (a *Attackable) CleanAllHates() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, e := range a.hates.Load().Snapshot() {
		a.desires.Load().Remove(IntentionCast, e.Attacker)
	}
	a.hates.Load().ZeroHate()
}

// RemoveThreat drops target's threat entry, its damage and its hate. No
// desire changes and the NPC is not sent back to peace.
func (a *Attackable) RemoveThreat(target attackable.Combatant) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.threats.Load().Remove(target)
}
