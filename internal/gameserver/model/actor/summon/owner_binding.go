package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// ownerBinding is the owner a summon answers to and that owner's inventory,
// replaced together.
type ownerBinding struct {
	owner     Owner
	inventory *itemcontainer.Inventory
}

func (a *Actor) bindOwner(owner Owner, inv *itemcontainer.Inventory) {
	a.binding.Store(&ownerBinding{owner: owner, inventory: inv})
}

// currentOwner returns the owner a answers to, or nil.
func (a *Actor) currentOwner() Owner {
	if b := a.binding.Load(); b != nil {
		return b.owner
	}
	return nil
}

// ownerInv returns the inventory of the owner a answers to, or nil.
func (a *Actor) ownerInv() *itemcontainer.Inventory {
	if b := a.binding.Load(); b != nil {
		return b.inventory
	}
	return nil
}

// RelinkOwner hands a pet corpse its owner left behind to that owner's new
// session: from here on it answers to owner, reads owner's inventory for its
// collar, and its work runs on q, owner's queue. It is then fully the new
// session's pet again, one that can be revived and, once revived, commanded;
// event.OwnerRelinked tells the runtime to move the rest of its work onto q.
//
// It reports false, changing nothing, for a servitor, a summon whose owner
// has not left, a pet that is no longer dead, one whose decay has already
// claimed it, and an owner with another object id. A relink and a decay
// claim take the same lock, so a corpse is either relinked or decays under
// the session that left it, never both.
//
// Call it as the owner of the corpse's own queue (sim.RunOwned): none of the
// corpse's work runs during the move, and work that queue accepted before it
// moves on to q (Post).
func (a *Actor) RelinkOwner(owner Owner, inv *itemcontainer.Inventory, q *sim.Queue) bool {
	if a == nil || !a.isPet || owner == nil || q == nil || owner.ObjectID() != a.OwnerID() {
		return false
	}
	a.vitals.mu.Lock()
	if !a.dead || a.decayed || !a.ownerLeft.Load() {
		a.vitals.mu.Unlock()
		return false
	}
	a.bindOwner(owner, inv)
	a.vitals.mu.Unlock()
	a.queue.Store(q)
	a.movement.SetQueue(q)
	a.emit(event.OwnerRelinked{})
	// Last, so nothing that waits for the owner to be back (a revive, a
	// later logout's leave) sees it before the move is complete.
	a.ownerLeft.Store(false)
	return true
}

// Post runs fn on a's queue and reports whether a queue accepted it. A job
// accepted by a queue a has since moved off (AdoptCorpseQueue, RelinkOwner)
// is handed on to the queue a moved to rather than run beside a's work there:
// a moves only while the queue it leaves is owned, from a task on it or
// through sim.RunOwned, so the check each job makes before running cannot
// race the move.
func (a *Actor) Post(fn func()) bool {
	q := a.Queue()
	if q == nil {
		return false
	}
	return q.Post(func() {
		if a.Queue() != q {
			a.Post(fn)
			return
		}
		fn()
	})
}
