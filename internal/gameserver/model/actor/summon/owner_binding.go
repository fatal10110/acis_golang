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

// OwnerItemCount is how many units of templateID the owner a answers to
// carries, or 0 with no owner inventory bound.
func (a *Actor) OwnerItemCount(templateID int32) int {
	inv := a.ownerInv()
	if inv == nil {
		return 0
	}
	return inv.ItemCount(templateID, -1, true)
}

// RelinkOwner hands a pet its owner left behind as a corpse to that owner's
// new session: from here on it answers to owner, reads owner's inventory for
// its collar, and its work runs on q, owner's queue. It is then fully the new
// session's pet again; event.OwnerRelinked tells the runtime to move the rest
// of its work onto q. A corpse can then be revived and, once revived,
// commanded. A pet revived while its owner was away (Revive) comes back
// alive where it stands: its owner-heal, if it is a baby pet, carries on on
// q, and owner's collar is lifted to the level the pet has since regained. It
// keeps what it was doing, so it follows its new owner only once told to,
// as a revived pet whose owner was away follows no one.
//
// It reports false, changing nothing, for a servitor, a summon whose owner
// has not left, a corpse whose decay has already claimed it, and an owner
// with another object id. A relink and a decay claim take the same lock, so
// a corpse is either relinked or decays under the session that left it,
// never both.
//
// Call it as the owner of the pet's own queue (sim.RunOwned): none of the
// pet's work runs during the move, and work that queue accepted before it
// moves on to q (Post).
func (a *Actor) RelinkOwner(owner Owner, inv *itemcontainer.Inventory, q *sim.Queue) bool {
	if a == nil || !a.isPet || owner == nil || q == nil || owner.ObjectID() != a.OwnerID() {
		return false
	}
	a.vitals.mu.Lock()
	if a.decayed || !a.ownerLeft.Load() {
		a.vitals.mu.Unlock()
		return false
	}
	a.bindOwner(owner, inv)
	a.vitals.mu.Unlock()
	a.queue.Store(q)
	a.movement.SetQueue(q)
	a.effects.SetQueue(q)
	a.emit(event.OwnerRelinked{})
	if !a.Dead() {
		// The owner-heal ticked on the queue the pet leaves, which closes
		// with the relink.
		a.moveBabyHeal()
		a.liftCollar(inv)
	}
	// Last, so nothing that waits for the owner to be back (a later
	// logout's leave) sees it before the move is complete.
	a.ownerLeft.Store(false)
	return true
}

// liftCollar sets the enchant of a pet's collar in inv to the pet's level.
// The owner's session reads its collar from its own saved row, which does
// not hold a level the pet regained after that session left. It is not
// SyncControlItemEnchant: that also refreshes the owner's pet window, which
// the relink, run mid-EnterWorld before the owner is in the world, must not
// do. The reference's collar already holds the level at login, so its
// restore refreshes nothing either, and the EnterWorld burst's own pet
// frames show the lifted level.
func (a *Actor) liftCollar(inv *itemcontainer.Inventory) {
	if inv == nil || a.controlItemID == 0 {
		return
	}
	if inst := inv.ItemByObjectID(a.controlItemID); inst != nil && inst.Snapshot().EnchantLevel != a.Level() {
		inv.SetEnchantLevel(inst, a.Level())
	}
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
