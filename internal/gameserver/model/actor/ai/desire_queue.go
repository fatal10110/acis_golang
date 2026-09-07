package ai

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// maxDesires bounds how many concurrent candidate Desires a DesireQueue can
// hold, so a long-lived or very busy actor can't accumulate an unbounded
// number of queued requests.
const maxDesires = 50

// DesireQueue is a concurrency-safe, weight-ranked collection of an actor's
// pending Desires.
//
// mu guards desires.
type DesireQueue struct {
	mu      sync.RWMutex
	desires []*Desire
}

// NewDesireQueue returns an empty DesireQueue.
func NewDesireQueue() *DesireQueue {
	return &DesireQueue{}
}

// AddOrUpdate adds desire to the queue. If an already-queued Desire is
// Equal to it, desire's weight is folded into that existing entry in place
// and desire itself is discarded, so a repeated request accumulates weight
// instead of growing the queue. Otherwise desire is appended, unless the
// queue is already at its capacity, in which case it is silently dropped.
func (q *DesireQueue) AddOrUpdate(desire *Desire) {
	q.mu.Lock()
	defer q.mu.Unlock()

	merged := false
	for _, d := range q.desires {
		if d.Equal(desire) {
			d.addWeight(desire.Weight)
			merged = true
		}
	}
	if merged || len(q.desires) >= maxDesires {
		return
	}
	q.desires = append(q.desires, desire)
}

// Peek returns the queued Desire with the highest weight. ok is false if
// the queue is empty. Ties resolve to whichever entry the scan reaches
// first.
func (q *DesireQueue) Peek() (desire *Desire, ok bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.desires) == 0 {
		return nil, false
	}

	best := q.desires[0]
	for _, d := range q.desires[1:] {
		if d.Weight > best.Weight {
			best = d
		}
	}
	return best, true
}

// DecreaseWeightByType subtracts amount from the weight of every queued
// Desire of the given kind. A Desire whose weight would drop below zero is
// removed from the queue instead of going negative.
func (q *DesireQueue) DecreaseWeightByType(kind Intention, amount float64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	kept := q.desires[:0]
	for _, d := range q.desires {
		if d.Kind == kind {
			if d.Weight-amount < 0 {
				continue
			}
			d.Weight -= amount
		}
		kept = append(kept, d)
	}
	q.desires = kept
}

// Remove drops queued Desires of kind aimed at target.
func (q *DesireQueue) Remove(kind Intention, target attackable.Combatant) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.removeLocked(func(d *Desire) bool {
		return d.Kind == kind && sameCombatant(d.FinalTarget, target)
	})
}

// RemoveFinalTarget drops every queued Desire aimed at target.
func (q *DesireQueue) RemoveFinalTarget(target attackable.Combatant) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.removeLocked(func(d *Desire) bool {
		return sameCombatant(d.FinalTarget, target)
	})
}

// RemoveKind drops every queued Desire of kind.
func (q *DesireQueue) RemoveKind(kind Intention) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.removeLocked(func(d *Desire) bool {
		return d.Kind == kind
	})
}

func (q *DesireQueue) removeLocked(drop func(*Desire) bool) {
	kept := q.desires[:0]
	for _, d := range q.desires {
		if drop(d) {
			continue
		}
		kept = append(kept, d)
	}
	q.desires = kept
}

// RemoveIf drops every queued Desire for which drop returns true.
func (q *DesireQueue) RemoveIf(drop func(*Desire) bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.removeLocked(drop)
}

// hasKind reports whether any queued Desire has the given kind.
func (q *DesireQueue) hasKind(kind Intention) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, d := range q.desires {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

// Has reports whether a queued Desire is Equal to probe.
func (q *DesireQueue) Has(probe *Desire) bool {
	if probe == nil {
		return false
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, d := range q.desires {
		if d.Equal(probe) {
			return true
		}
	}
	return false
}

// NonMovingAttack returns the queued ATTACK Desire aimed at target whose
// MoveToTarget is false, if one is queued. Equivalent to the reference's
// Npc.canAutoAttack finding the first ATTACK desire for target and then
// checking !getMoveToTarget(): a matching desire with MoveToTarget true is
// treated the same as no match, since callers only act on the non-moving
// case.
func (q *DesireQueue) NonMovingAttack(target attackable.Combatant) (*Desire, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, d := range q.desires {
		if d.Kind == IntentionAttack && !d.MoveToTarget && sameCombatant(d.FinalTarget, target) {
			return d, true
		}
	}
	return nil, false
}

// Len returns the number of Desires currently queued.
func (q *DesireQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.desires)
}

// Clear drops every queued Desire.
func (q *DesireQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.desires = nil
}
