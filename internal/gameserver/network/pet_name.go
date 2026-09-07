package network

import (
	"context"
	"regexp"

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

type petRenameResult uint8

const (
	petRenameIgnored petRenameResult = iota
	petRenameNameTaken
	petRenameNPCName
	petRenameApplied
)

// renamePet applies the persistence and owner-refresh portion of pet naming.
// Packet decoding, length/pattern validation, and the "already named" gate
// belong to RequestChangePetName, which must run them in reference order
// around this call.
func (l *GameClientLink) renamePet(ctx context.Context, live *livePlayer, name string) petRenameResult {
	if l == nil {
		return petRenameIgnored
	}
	actor, ok := l.ownedPet(live)
	if !ok || l.npcs == nil || l.petStore == nil {
		return petRenameIgnored
	}
	// Java checks the npc-name collision (silent reject) before checking
	// the pets table (RequestChangePetName.java:63-71).
	if _, ok := l.npcs.GetByName(name); ok {
		return petRenameNPCName
	}
	store, ok := l.petStore.(petNameStore)
	if !ok {
		return petRenameIgnored
	}
	taken, err := store.NameTaken(ctx, name)
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("check pet name")
		return petRenameIgnored
	}
	if taken {
		return petRenameNameTaken
	}

	oldName, oldNamed := actor.Name(), actor.IsNamed()
	actor.SetName(name)
	actor.SetNamed(true)
	itemObjectID, state, ok := actor.PetState()
	if !ok {
		actor.SetName(oldName)
		actor.SetNamed(oldNamed)
		return petRenameIgnored
	}
	if err := l.petStore.Save(ctx, itemObjectID, state); err != nil {
		actor.SetName(oldName)
		actor.SetNamed(oldNamed)
		l.log.Error().Err(err).Int32("item_obj_id", itemObjectID).Msg("save pet name")
		return petRenameIgnored
	}
	if inv := live.Inventory(); inv != nil {
		if control := inv.ItemByObjectID(actor.ControlItemID()); control != nil {
			control.SetCustomType2(1)
		}
	}
	actor.UpdateStatus()
	return petRenameApplied
}

// handleRequestChangePetName runs RequestChangePetName's decoded gates in
// reference order: no active pet is silent, then length, then the
// "already named" gate, then the character pattern, then renamePet's
// npc-name/uniqueness checks and persistence.
func (l *GameClientLink) handleRequestChangePetName(ctx context.Context, live *livePlayer, req clientpackets.RequestChangePetName) {
	actor, ok := l.ownedPet(live)
	if !ok {
		return
	}
	if len(req.Name) < 1 || len(req.Name) > 16 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingCharnameUpTo16Chars))
		return
	}
	if actor.IsNamed() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingYouCannotSetNameOfThePet))
		return
	}
	if !petNamePattern.MatchString(req.Name) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingPetnameContainsInvalidChars))
		return
	}
	switch l.renamePet(ctx, live, req.Name) {
	case petRenameNameTaken:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet))
	case petRenameApplied:
		sendSummonInfosToOwner(actor)
	}
}
