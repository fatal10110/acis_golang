package gameservertest

import (
	"context"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// WithSelectionHold runs hold inside every character selection, after the
// selected character is restored and attached as a live player and before
// it is registered in the world: in the write that marks its row online. A
// suite uses it to act in that window, from the selecting connection's own
// goroutine.
func WithSelectionHold(hold func(objectID int32)) Option {
	return func(o *options) { o.selectionHold = hold }
}

// holdingCharacterStore is the roster's characters store, which runs hold,
// if any, ahead of marking a row online (WithSelectionHold).
type holdingCharacterStore struct {
	*gamesql.CharacterStore
	hold func(objectID int32)
}

func (s holdingCharacterStore) SetOnline(ctx context.Context, objectID int32, lastAccess int64) error {
	if s.hold != nil {
		s.hold(objectID)
	}
	return s.CharacterStore.SetOnline(ctx, objectID, lastAccess)
}
