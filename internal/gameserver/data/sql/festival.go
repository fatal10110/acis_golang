package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// FestivalStore reads and writes the seven_signs_festival scores and the
// festival's columns of the seven_signs_status row (festival_cycle,
// accumulated_bonus0-4).
type FestivalStore struct {
	db *sql.DB
}

// NewFestivalStore returns a FestivalStore backed by db.
func NewFestivalStore(db *sql.DB) *FestivalStore {
	return &FestivalStore{db: db}
}

// LoadScores returns every stored score.
func (s *FestivalStore) LoadScores(ctx context.Context) ([]festival.Score, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT festivalId, cabal, cycle, date, score, members FROM seven_signs_festival`)
	if err != nil {
		return nil, fmt.Errorf("load festival scores: %w", err)
	}
	defer rows.Close()

	var out []festival.Score
	for rows.Next() {
		var (
			sc    festival.Score
			cabal string
			date  sql.NullInt64
		)
		if err := rows.Scan(&sc.FestivalID, &cabal, &sc.Cycle, &date, &sc.Score, &sc.Members); err != nil {
			return nil, fmt.Errorf("scan festival score: %w", err)
		}
		if sc.Cabal, err = sevensigns.ParseCabal(cabal); err != nil {
			return nil, fmt.Errorf("festival %d cycle %d: %w", sc.FestivalID, sc.Cycle, err)
		}
		sc.Date = date.Int64
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load festival scores: %w", err)
	}
	return out, nil
}

// SaveScores inserts every score in one transaction, updating the date,
// score and members of one already stored for its festival, cabal and
// cycle.
func (s *FestivalStore) SaveScores(ctx context.Context, scores []festival.Score) error {
	if len(scores) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save festival scores: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO seven_signs_festival (festivalId, cabal, cycle, date, score, members) VALUES (?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE date = VALUES(date), score = VALUES(score), members = VALUES(members)`)
	if err != nil {
		return fmt.Errorf("save festival scores: %w", err)
	}
	defer stmt.Close()
	for _, sc := range scores {
		if _, err := stmt.ExecContext(ctx, sc.FestivalID, sc.Cabal.String(), sc.Cycle, sc.Date, sc.Score, sc.Members); err != nil {
			return fmt.Errorf("save festival %d %s cycle %d: %w", sc.FestivalID, sc.Cabal, sc.Cycle, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit festival scores: %w", err)
	}
	return nil
}

// LoadStatus returns the festival columns of the status row id=0, or
// found=false when the row does not exist.
func (s *FestivalStore) LoadStatus(ctx context.Context) (festival.Status, bool, error) {
	var st festival.Status
	err := s.db.QueryRowContext(ctx, `SELECT festival_cycle, accumulated_bonus0, accumulated_bonus1, accumulated_bonus2,
	accumulated_bonus3, accumulated_bonus4 FROM seven_signs_status WHERE id = 0`).Scan(
		&st.FestivalCycle, &st.Bonuses[0], &st.Bonuses[1], &st.Bonuses[2], &st.Bonuses[3], &st.Bonuses[4])
	if errors.Is(err, sql.ErrNoRows) {
		return festival.Status{}, false, nil
	}
	if err != nil {
		return festival.Status{}, false, fmt.Errorf("load festival status: %w", err)
	}
	return st, true, nil
}

// SaveStatus writes the festival columns of the status row id=0. A missing
// row stays missing: the Seven Signs status save that precedes this one
// creates it.
func (s *FestivalStore) SaveStatus(ctx context.Context, st festival.Status) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE seven_signs_status SET festival_cycle = ?, accumulated_bonus0 = ?,
	accumulated_bonus1 = ?, accumulated_bonus2 = ?, accumulated_bonus3 = ?, accumulated_bonus4 = ? WHERE id = 0`,
		st.FestivalCycle, st.Bonuses[0], st.Bonuses[1], st.Bonuses[2], st.Bonuses[3], st.Bonuses[4]); err != nil {
		return fmt.Errorf("save festival status: %w", err)
	}
	return nil
}
