package gameservertest

import (
	"context"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// CharacterNameFault makes the character-name calls admin edits issue
// fail: a non-nil Lookup fails every name uniqueness lookup, a non-nil
// Store every name write. Every other character edit reaches the database
// as usual, and so does character creation.
type CharacterNameFault struct {
	Lookup error
	Store  error
}

// WithCharacterNameFault installs fault on the store admin edits use, so a
// suite can drive the rename paths a characters table that cannot answer
// takes.
func WithCharacterNameFault(fault CharacterNameFault) Option {
	return func(o *options) { o.characterNameFault = fault }
}

// faultyCharacterNameStore is the character store wrapper
// WithCharacterNameFault installs.
type faultyCharacterNameStore struct {
	*gamesql.CharacterStore
	fault CharacterNameFault
}

func (s faultyCharacterNameStore) NameTaken(ctx context.Context, name string) (bool, error) {
	if s.fault.Lookup != nil {
		return false, s.fault.Lookup
	}
	return s.CharacterStore.NameTaken(ctx, name)
}

func (s faultyCharacterNameStore) SetName(ctx context.Context, objectID int32, name string) error {
	if s.fault.Store != nil {
		return s.fault.Store
	}
	return s.CharacterStore.SetName(ctx, objectID, name)
}
