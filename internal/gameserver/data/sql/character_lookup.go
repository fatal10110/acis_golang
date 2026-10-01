package sql

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
)

// FindByName returns the character named name, matched case-insensitively.
// It reports false when no character has that name.
func (s *CharacterStore) FindByName(ctx context.Context, name string) (relation.Named, bool, error) {
	if name == "" {
		return relation.Named{}, false, nil
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT obj_Id, char_name, COALESCE(accesslevel,0) FROM characters WHERE LOWER(char_name) = LOWER(?) ORDER BY obj_Id", name)
	if err != nil {
		return relation.Named{}, false, fmt.Errorf("find character %q: %w", name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref relation.Named
		if err := rows.Scan(&ref.ID, &ref.Name, &ref.AccessLevel); err != nil {
			return relation.Named{}, false, fmt.Errorf("find character %q: %w", name, err)
		}
		// The collation folds more than case; only a case-only difference
		// names the same character.
		if strings.EqualFold(ref.Name, name) {
			return ref, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return relation.Named{}, false, fmt.Errorf("find character %q: %w", name, err)
	}
	return relation.Named{}, false, nil
}

// Names returns the stored name of each of ids that names a character; an
// id with no characters row is absent from the map.
func (s *CharacterStore) Names(ctx context.Context, ids []int32) (map[int32]string, error) {
	out := make(map[int32]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	query := "SELECT obj_Id, char_name FROM characters WHERE obj_Id IN (?" + strings.Repeat(",?", len(ids)-1) + ")"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("character names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id   int32
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("character names: %w", err)
		}
		out[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("character names: %w", err)
	}
	return out, nil
}
