package boat

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// MaxHP is the maximum HP of every boat.
const MaxHP = 50000

// deathHP is the HP under which a boat breaks.
const deathHP = 0.5

// Attacker is whoever hits a boat: a hit lands only when it may deal damage.
type Attacker interface {
	CanGiveDamage() bool
}

// HP returns the boat's current HP.
func (b *Boat) HP() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hp
}

// Dead reports whether the boat is broken.
func (b *Boat) Dead() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dead
}

// ReduceHP takes amount from the boat's HP, never under zero, and breaks the
// boat once its HP falls under half a point: it stops where it stands,
// showing its observers so, and the leg it was under way on never ends. Its
// schedule, its passengers and its fare due stay as they are, so a boat
// broken at a dock still puts out on time. A broken or retired boat, or a hit
// from an attacker without damage permission, takes nothing; a nil attacker
// always hits. No status update goes out for a boat, so the loss itself
// shows nobody.
//
// The break runs under the fleet's lock, so it never interleaves with a
// position update or a schedule step.
func (b *Boat) ReduceHP(amount float64, attacker Attacker) {
	if attacker != nil && !attacker.CanGiveDamage() {
		return
	}
	b.fleetMu.Lock()
	defer b.fleetMu.Unlock()
	b.mu.Lock()
	if b.dead || b.retired {
		b.mu.Unlock()
		return
	}
	// A NaN amount takes nothing, like a negative one.
	if amount > 0 {
		b.hp = max(b.hp-amount, 0)
	}
	if b.hp >= deathHP {
		b.mu.Unlock()
		return
	}
	b.hp, b.dead = 0, true
	b.mu.Unlock()
	b.stop()
}

// SetMaxHP fills a living boat's HP. Nobody watches it, so it shows nobody.
func (b *Boat) SetMaxHP() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.dead {
		b.hp = MaxHP
	}
}

// Revive mends this broken boat with restoreHP of its maximum HP and shows
// its observers it revive. It stays where it broke: a leg cut short stays
// cut short. A living or retired boat is left as it is, and Revive reports
// false.
func (b *Boat) Revive(restoreHP float64) bool {
	b.fleetMu.Lock()
	defer b.fleetMu.Unlock()
	b.mu.Lock()
	if !b.dead || b.retired {
		b.mu.Unlock()
		return false
	}
	b.dead = false
	b.hp = min(MaxHP*restoreHP, MaxHP)
	b.mu.Unlock()
	b.emit(event.Revived{})
	return true
}
