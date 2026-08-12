package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

type petNameStore interface {
	NameTaken(context.Context, string) (bool, error)
}

type petRenameResult uint8

const (
	petRenameIgnored petRenameResult = iota
	petRenameNameTaken
	petRenameNPCName
	petRenameApplied
)

// renamePet applies the persistence and owner-refresh portion of pet naming.
// Packet decoding and name-format validation belong to RequestChangePetName.
func (l *GameClientLink) renamePet(ctx context.Context, live *livePlayer, name string) petRenameResult {
	if l == nil || live == nil || l.world == nil || l.npcs == nil || l.petStore == nil {
		return petRenameIgnored
	}
	pet, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return petRenameIgnored
	}
	actor, ok := pet.(*summon.Actor)
	if !ok || !actor.IsPet() {
		return petRenameIgnored
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
	if _, ok := l.npcs.GetByName(name); ok {
		return petRenameNPCName
	}

	oldName := actor.Name()
	actor.SetName(name)
	itemObjectID, state, ok := actor.PetState()
	if !ok {
		actor.SetName(oldName)
		return petRenameIgnored
	}
	if err := l.petStore.Save(ctx, itemObjectID, state); err != nil {
		actor.SetName(oldName)
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
