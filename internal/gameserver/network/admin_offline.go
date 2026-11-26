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
// of a session that left (settleWithLeftOwner).
//
// The name is looked up on gm's connection goroutine, off gm's queue
// (deferLookup); then, back on gm's queue, the change is applied and report
// told whether a character has that name. A lookup that fails is logged and
// reports nothing. Only a request that may defer lookups may call it.
func (l *GameClientLink) adminByName(gm *livePlayer, name string, online func(*livePlayer), offline func(objectID int32), report func(found bool)) {
	if l.characters == nil {
		report(false)
		return
	}
	gm.deferLookup(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, adminLookupTimeout)
		defer cancel()
		ref, found, err := l.characters.FindByName(ctx, name)
		onLive(gm, func() {
			switch {
			case err != nil:
				l.log.Error().Err(err).Str("name", name).Msg("admin: find character")
			case !found:
				report(false)
			default:
				l.settleWithLeftOwner(ref.ID, online, func() { offline(ref.ID) })
				report(true)
			}
		})
	})
}
