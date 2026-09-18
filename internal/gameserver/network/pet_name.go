package network

import (
	"context"
	"regexp"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// petNamePattern matches StringUtil.isValidString(name, "^[A-Za-z0-9]{1,16}$")
// in RequestChangePetName.java.
var petNamePattern = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)

type petNameStore interface {
	NameTaken(context.Context, string) (bool, error)
}

// ownedPet resolves live's currently spawned pet actor, matching Java's
// player.hasPet() check: it does not require a pet inventory, unlike
// activePet (pet.go), which pet item transfers do.
func (l *GameClientLink) ownedPet(live *livePlayer) (*summon.Actor, bool) {
	if l == nil || live == nil || l.world == nil {
		return nil, false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil, false
	}
	actor, ok := obj.(*summon.Actor)
	if !ok || !actor.IsPet() {
		return nil, false
	}
	return actor, true
}

// renameTimeout bounds the pets-table uniqueness read. It runs on the
// connection goroutine with the request's own context, so a disconnect still
// cancels it; the timeout only stops a wedged database from holding the
// connection open.
const renameTimeout = 5 * time.Second

// handleRequestChangePetName runs RequestChangePetName's decoded gates in
// reference order: no active pet is silent, then length, then the
// "already named" gate, then the character pattern, then the npc-name
// collision, the pets-table uniqueness read, and the rename itself.
//
// The uniqueness read is the one step that cannot run on the player's queue,
// where a slow database would hold a pool worker. It runs here, on the
// connection goroutine, which is already waiting for the queued gates, and
// the rename is posted back to the queue with the result. Frame order is
// unchanged: the loop does not read the next frame until the rename has been
// applied or rejected.
func (l *GameClientLink) handleRequestChangePetName(ctx context.Context, live *livePlayer, req clientpackets.RequestChangePetName) {
	var lookup func(context.Context)
	onLive(live, func() { lookup = l.beginChangePetName(live, req) })
	if lookup != nil {
		lookup(ctx)
	}
}

// beginChangePetName runs every gate that needs no database, on live's queue.
// It returns the uniqueness lookup to run off the queue, or nil when the
// request is already answered or rejected.
func (l *GameClientLink) beginChangePetName(live *livePlayer, req clientpackets.RequestChangePetName) func(context.Context) {
	actor, ok := l.ownedPet(live)
	if !ok {
		return nil
	}
	if len(req.Name) < 1 || len(req.Name) > 16 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingCharnameUpTo16Chars))
		return nil
	}
	if actor.IsNamed() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingYouCannotSetNameOfThePet))
		return nil
	}
	if !petNamePattern.MatchString(req.Name) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingPetnameContainsInvalidChars))
		return nil
	}
	if l.npcs == nil || l.petStore == nil {
		return nil
	}
	// Java checks the npc-name collision (silent reject) before checking
	// the pets table (RequestChangePetName.java:63-71).
	if _, ok := l.npcs.GetByName(req.Name); ok {
		return nil
	}
	store, ok := l.petStore.(petNameStore)
	if !ok {
		return nil
	}
	name := req.Name
	return func(ctx context.Context) {
		readCtx, cancel := context.WithTimeout(ctx, renameTimeout)
		defer cancel()
		taken, err := store.NameTaken(readCtx, name)
		if err != nil {
			l.log.Error().Err(err).Str("name", name).Msg("check pet name")
			return
		}
		onLive(live, func() { l.finishChangePetName(live, name, taken) })
	}
}

// finishChangePetName applies the rename back on live's queue, with the
// uniqueness result the lookup read. The pet is re-resolved and re-checked
// here: an unsummon, a logout or another rename may have run on the queue
// while the read was outstanding.
func (l *GameClientLink) finishChangePetName(live *livePlayer, name string, taken bool) {
	if taken {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet))
		return
	}
	actor, ok := l.ownedPet(live)
	if !ok || actor.IsNamed() {
		return
	}

	oldName, oldNamed := actor.Name(), actor.IsNamed()
	actor.SetName(name)
	actor.SetNamed(true)
	itemObjectID, state, ok := actor.PetState()
	if !ok {
		actor.SetName(oldName)
		actor.SetNamed(oldNamed)
		return
	}
	// Written on the control item's lane, behind any pet save already queued,
	// so an older copy cannot land after it. The rename does not wait for the write:
	// the reference only renames in memory and stores the name with the pet's
	// next save, so a failed write is logged, not rolled back.
	pets, log := l.petStore, l.log
	l.persist.Enqueue(itemObjectID, func() {
		saveCtx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := pets.Save(saveCtx, itemObjectID, state); err != nil {
			log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("save pet name")
		}
	})
	if inv := live.Inventory(); inv != nil {
		if control := inv.ItemByObjectID(actor.ControlItemID()); control != nil {
			control.SetCustomType2(1)
		}
	}
	actor.UpdateStatus()
	sendSummonInfosToOwner(actor)
}
