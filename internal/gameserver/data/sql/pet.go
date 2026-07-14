package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
)

// PetStore reads and writes pet rows keyed by the pet collar item object id.
type PetStore struct {
	db *sql.DB
}

// NewPetStore returns a PetStore backed by db.
func NewPetStore(db *sql.DB) *PetStore {
	return &PetStore{db: db}
}

// Get returns the saved state for the pet whose collar is itemObjectID, or
// (State{}, false, nil) if no row exists yet for it — the case for a pet
// summoned for the first time from a freshly bought collar.
func (s *PetStore) Get(ctx context.Context, itemObjectID int32) (pet.State, bool, error) {
	var st pet.State
	err := s.db.QueryRowContext(ctx,
		`SELECT name, level, curHp, curMp, exp, sp, fed FROM pets WHERE item_obj_id = ?`, itemObjectID,
	).Scan(&st.Name, &st.Level, &st.CurHP, &st.CurMP, &st.Exp, &st.SP, &st.Fed)
	if errors.Is(err, sql.ErrNoRows) {
		return pet.State{}, false, nil
	}
	if err != nil {
		return pet.State{}, false, fmt.Errorf("get pet %d: %w", itemObjectID, err)
	}
	return st, true, nil
}

// Save inserts or updates the row for the pet whose collar is itemObjectID.
func (s *PetStore) Save(ctx context.Context, itemObjectID int32, st pet.State) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pets (name, level, curHp, curMp, exp, sp, fed, item_obj_id) VALUES (?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE name=VALUES(name), level=VALUES(level), curHp=VALUES(curHp), curMp=VALUES(curMp), exp=VALUES(exp), sp=VALUES(sp), fed=VALUES(fed)`,
		st.Name, st.Level, st.CurHP, st.CurMP, st.Exp, st.SP, st.Fed, itemObjectID,
	)
	if err != nil {
		return fmt.Errorf("save pet %d: %w", itemObjectID, err)
	}
	return nil
}

// DeleteByItemObjectID removes the pet row tied to itemObjectID, if any.
func (s *PetStore) DeleteByItemObjectID(ctx context.Context, itemObjectID int32) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM pets WHERE item_obj_id = ?", itemObjectID)
	if err != nil {
		return fmt.Errorf("delete pet item %d: %w", itemObjectID, err)
	}
	return nil
}
