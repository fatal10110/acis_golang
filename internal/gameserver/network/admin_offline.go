package network

import (
	"context"
	"time"
)

// adminLookupTimeout bounds the lookup of a character an admin command
// names.
const adminLookupTimeout = 5 * time.Second

// adminByName applies an admin command's change to the character named
// name, which no player in the world answers to by that name: with online,
// on the queue of a session of it found in the world, or that a selection
// of it under way ends with; with offline, given its object id, while it is
// out of the world. A selection under way holds the change until it ends,
// so the session it starts is not read from a row the change missed, and
// offline runs under the selection gate, queuing its write on the
// character's lane ahead of the next selection's read and after the saves
// of a session that left (settleWithLeftOwner). found is false when no
// character has that name; ok is false when the lookup failed, which is
// logged.
func (l *GameClientLink) adminByName(name string, online func(*livePlayer), offline func(objectID int32)) (found, ok bool) {
	if l.characters == nil {
		return false, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminLookupTimeout)
	defer cancel()
	ref, found, err := l.characters.FindByName(ctx, name)
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("admin: find character")
		return false, false
	}
	if !found {
		return false, true
	}
	l.settleWithLeftOwner(ref.ID, online, func() { offline(ref.ID) })
	return true, true
}
