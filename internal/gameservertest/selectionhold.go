package gameservertest

import (
	"context"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// WithSelectionHold runs hold inside every character selection, after the
// selected character is restored and attached as a live player and before
// it is registered in the world: in the write that marks its row online. A
// suite uses it to act in that window, from the selecting connection's own
// goroutine.
func WithSelectionHold(hold func(objectID int32)) Option {
	return func(o *options) { o.selectionHold = hold }
}

// WithCharacterCreateHold runs hold inside every character creation, once
// the name has passed its checks and before the new row is inserted, from
// the creating connection's own goroutine. A non-nil error from hold fails
// the insert. A suite uses it to act while a creation holds its name, or to
// drive a creation whose insert fails.
func WithCharacterCreateHold(hold func(name string) error) Option {
	return func(o *options) { o.characterCreateHold = hold }
}

// holdingCharacterStore is the roster's characters store, which runs hold,
// if any, ahead of marking a row online (WithSelectionHold), and createHold
// ahead of inserting a row (WithCharacterCreateHold).
type holdingCharacterStore struct {
	*gamesql.CharacterStore
	hold       func(objectID int32)
	createHold func(name string) error
}

func (s holdingCharacterStore) Create(ctx context.Context, c *player.Character) error {
	if s.createHold != nil {
		if err := s.createHold(c.CharacterName()); err != nil {
			return err
		}
	}
	return s.CharacterStore.Create(ctx, c)
}

func (s holdingCharacterStore) SetOnline(ctx context.Context, objectID int32, lastAccess int64) error {
	if s.hold != nil {
		s.hold(objectID)
	}
	return s.CharacterStore.SetOnline(ctx, objectID, lastAccess)
}
