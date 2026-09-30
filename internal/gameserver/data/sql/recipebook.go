package sql

import (
	"context"
	"database/sql"
	"fmt"
)

// RecipeBookStore reads and writes character_recipebook rows.
type RecipeBookStore struct {
	db *sql.DB
}

// NewRecipeBookStore returns a RecipeBookStore backed by db.
func NewRecipeBookStore(db *sql.DB) *RecipeBookStore {
	return &RecipeBookStore{db: db}
}

// ListByOwner returns the recipe ids ownerID has registered, in ascending
// order: the table's primary-key order, which is the order the book is
// rebuilt in.
func (s *RecipeBookStore) ListByOwner(ctx context.Context, ownerID int32) ([]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT recipeId FROM character_recipebook WHERE charId = ? ORDER BY recipeId`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list recipes for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list recipes for owner %d: %w", ownerID, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recipes for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// Insert registers recipeID in ownerID's book.
func (s *RecipeBookStore) Insert(ctx context.Context, ownerID int32, recipeID int) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO character_recipebook (charId, recipeId) VALUES (?, ?)`, ownerID, recipeID); err != nil {
		return fmt.Errorf("insert recipe %d for owner %d: %w", recipeID, ownerID, err)
	}
	return nil
}

// Delete removes recipeID from ownerID's book.
func (s *RecipeBookStore) Delete(ctx context.Context, ownerID int32, recipeID int) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM character_recipebook WHERE charId = ? AND recipeId = ?`, ownerID, recipeID); err != nil {
		return fmt.Errorf("delete recipe %d for owner %d: %w", recipeID, ownerID, err)
	}
	return nil
}
