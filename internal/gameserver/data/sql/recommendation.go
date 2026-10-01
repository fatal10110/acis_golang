package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// RecommendationStore reads and writes character_recommends rows and the
// characters table's rec_have and rec_left columns.
type RecommendationStore struct {
	db *sql.DB
}

// NewRecommendationStore returns a RecommendationStore backed by db.
func NewRecommendationStore(db *sql.DB) *RecommendationStore {
	return &RecommendationStore{db: db}
}

// ListRecommended returns the characters charID recommended since the last
// daily refresh.
func (s *RecommendationStore) ListRecommended(ctx context.Context, charID int32) ([]int32, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT target_id FROM character_recommends WHERE char_id = ?`, charID)
	if err != nil {
		return nil, fmt.Errorf("list recommendations of %d: %w", charID, err)
	}
	defer rows.Close()

	var out []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list recommendations of %d: %w", charID, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recommendations of %d: %w", charID, err)
	}
	return out, nil
}

// Give records that giverID recommended targetID, then stores the target's
// new count and the giver's remaining one. Each write commits on its own
// and the first failure skips the rest.
func (s *RecommendationStore) Give(ctx context.Context, giverID, targetID int32, targetHave, giverLeft int) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO character_recommends (char_id, target_id) VALUES (?, ?)`, giverID, targetID); err != nil {
		return fmt.Errorf("record recommendation of %d by %d: %w", targetID, giverID, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE characters SET rec_have = ? WHERE obj_Id = ?`, targetHave, targetID); err != nil {
		return fmt.Errorf("save recommendations of %d: %w", targetID, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE characters SET rec_left = ? WHERE obj_Id = ?`, giverLeft, giverID); err != nil {
		return fmt.Errorf("save recommendations left of %d: %w", giverID, err)
	}
	return nil
}

// RefreshDaily applies the daily refresh to every stored character: the
// record of who recommended whom is emptied, and each row's counters are
// reset from its stored level as player.DailyRecommendations says.
func (s *RecommendationStore) RefreshDaily(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `TRUNCATE character_recommends`); err != nil {
		return fmt.Errorf("refresh recommendations: clear: %w", err)
	}
	type update struct {
		id         int32
		have, left int
	}
	rows, err := s.db.QueryContext(ctx, `SELECT obj_Id, COALESCE(level, 0), rec_have FROM characters`)
	if err != nil {
		return fmt.Errorf("refresh recommendations: list characters: %w", err)
	}
	var updates []update
	for rows.Next() {
		var u update
		var level int
		if err := rows.Scan(&u.id, &level, &u.have); err != nil {
			_ = rows.Close()
			return fmt.Errorf("refresh recommendations: list characters: %w", err)
		}
		var loss int
		u.left, loss = player.DailyRecommendations(level)
		u.have = max(u.have-loss, 0)
		updates = append(updates, u)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("refresh recommendations: list characters: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("refresh recommendations: list characters: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("refresh recommendations: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `UPDATE characters SET rec_left = ?, rec_have = ? WHERE obj_Id = ?`)
	if err != nil {
		return fmt.Errorf("refresh recommendations: prepare: %w", err)
	}
	defer stmt.Close()
	for _, u := range updates {
		if _, err := stmt.ExecContext(ctx, u.left, u.have, u.id); err != nil {
			return fmt.Errorf("refresh recommendations of %d: %w", u.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("refresh recommendations: commit: %w", err)
	}
	return nil
}
