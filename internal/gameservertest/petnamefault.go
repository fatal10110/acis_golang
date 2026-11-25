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

// WithPetNameLookupHold runs hold ahead of every pet-name uniqueness lookup
// the server issues, on the renaming connection's goroutine, so a suite can
// hold one rename's read while another runs. It cannot be combined with
// WithSlowStores.
func WithPetNameLookupHold(hold func()) Option {
	return func(o *options) { o.petNameLookupHold = hold }
}

// failingPetNameStore is the pet store wrapper WithPetNameLookupError and
// WithPetNameLookupHold install.
type failingPetNameStore struct {
	*gamesql.PetStore
	err  error
	hold func()
}

func (s failingPetNameStore) NameTaken(ctx context.Context, name string) (bool, error) {
	if s.hold != nil {
		s.hold()
	}
	if s.err != nil {
		return false, s.err
	}
	return s.PetStore.NameTaken(ctx, name)
}
