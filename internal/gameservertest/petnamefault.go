package gameservertest

import (
	"context"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// WithPetNameLookupError makes every pet-name uniqueness lookup the server
// issues fail with err, so a suite can drive the rename path a pets table
// that cannot answer takes. Every other pets-table call reaches the database
// as usual. It cannot be combined with WithSlowStores.
func WithPetNameLookupError(err error) Option {
	return func(o *options) { o.petNameLookupErr = err }
}

// failingPetNameStore is the pet store wrapper WithPetNameLookupError
// installs.
type failingPetNameStore struct {
	*gamesql.PetStore
	err error
}

func (s failingPetNameStore) NameTaken(context.Context, string) (bool, error) {
	return false, s.err
}
