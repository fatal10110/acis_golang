package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// SevenSignsStore reads and writes the seven_signs_status row and the
// seven_signs sign-ups. The festival columns of the status row
// (festival_cycle, accumulated_bonus0-4) belong to the festival and are
// never written here.
type SevenSignsStore struct {
	db *sql.DB
}

// NewSevenSignsStore returns a SevenSignsStore backed by db.
func NewSevenSignsStore(db *sql.DB) *SevenSignsStore {
	return &SevenSignsStore{db: db}
}

const sevenSignsStatusColumns = `current_cycle, active_period, date, previous_winner,
	dawn_stone_score, dawn_festival_score, dusk_stone_score, dusk_festival_score,
	avarice_owner, gnosis_owner, strife_owner,
	avarice_dawn_score, gnosis_dawn_score, strife_dawn_score,
	avarice_dusk_score, gnosis_dusk_score, strife_dusk_score`

// LoadStatus returns the status row id=0, or found=false when the table has
// not been seeded yet.
func (s *SevenSignsStore) LoadStatus(ctx context.Context) (sevensigns.StatusRow, bool, error) {
	var (
		row            sevensigns.StatusRow
		period, winner string
		owners         [3]string
		dateMillis     int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT `+sevenSignsStatusColumns+` FROM seven_signs_status WHERE id = 0`).Scan(
		&row.Cycle, &period, &dateMillis, &winner,
		&row.DawnStoneScore, &row.DawnFestivalScore, &row.DuskStoneScore, &row.DuskFestivalScore,
		&owners[0], &owners[1], &owners[2],
		&row.DawnSealVotes[0], &row.DawnSealVotes[1], &row.DawnSealVotes[2],
		&row.DuskSealVotes[0], &row.DuskSealVotes[1], &row.DuskSealVotes[2],
	)
	if err == sql.ErrNoRows {
		return sevensigns.StatusRow{}, false, nil
	}
	if err != nil {
		return sevensigns.StatusRow{}, false, fmt.Errorf("load seven signs status: %w", err)
	}
	if row.Period, err = sevensigns.ParsePeriod(period); err != nil {
		return sevensigns.StatusRow{}, false, err
	}
	if row.PreviousWinner, err = sevensigns.ParseCabal(winner); err != nil {
		return sevensigns.StatusRow{}, false, err
	}
	for i, name := range owners {
		if row.SealOwners[i], err = sevensigns.ParseCabal(name); err != nil {
			return sevensigns.StatusRow{}, false, err
		}
	}
	if dateMillis > 0 {
		row.LastSave = time.UnixMilli(dateMillis)
	}
	return row, true, nil
}

// SaveStatus writes row back to the status row id=0, inserting it with
// schema defaults for the festival columns when it is missing. A status
// never saved keeps the date 0.
func (s *SevenSignsStore) SaveStatus(ctx context.Context, row sevensigns.StatusRow) error {
	var dateMillis int64
	if !row.LastSave.IsZero() {
		dateMillis = row.LastSave.UnixMilli()
	}
	args := []any{
		row.Cycle, row.Period.String(), dateMillis, row.PreviousWinner.String(),
		row.DawnStoneScore, row.DawnFestivalScore, row.DuskStoneScore, row.DuskFestivalScore,
		row.SealOwners[0].String(), row.SealOwners[1].String(), row.SealOwners[2].String(),
		row.DawnSealVotes[0], row.DawnSealVotes[1], row.DawnSealVotes[2],
		row.DuskSealVotes[0], row.DuskSealVotes[1], row.DuskSealVotes[2],
	}
	res, err := s.db.ExecContext(ctx, `UPDATE seven_signs_status SET current_cycle = ?, active_period = ?, date = ?, previous_winner = ?,
	dawn_stone_score = ?, dawn_festival_score = ?, dusk_stone_score = ?, dusk_festival_score = ?,
	avarice_owner = ?, gnosis_owner = ?, strife_owner = ?,
	avarice_dawn_score = ?, gnosis_dawn_score = ?, strife_dawn_score = ?,
	avarice_dusk_score = ?, gnosis_dusk_score = ?, strife_dusk_score = ? WHERE id = 0`, args...)
	if err != nil {
		return fmt.Errorf("save seven signs status: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO seven_signs_status (id, `+sevenSignsStatusColumns+`) VALUES (0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		args...,
	); err != nil {
		return fmt.Errorf("insert seven signs status: %w", err)
	}
	return nil
}

// LoadPlayers returns every sign-up.
func (s *SevenSignsStore) LoadPlayers(ctx context.Context) ([]sevensigns.PlayerRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT char_obj_id, cabal, seal, red_stones, green_stones, blue_stones,
	ancient_adena_amount, contribution_score FROM seven_signs`)
	if err != nil {
		return nil, fmt.Errorf("load seven signs players: %w", err)
	}
	defer rows.Close()

	var out []sevensigns.PlayerRow
	for rows.Next() {
		var (
			p            sevensigns.PlayerRow
			cabal, seal  string
			adena, score float64
		)
		if err := rows.Scan(&p.ObjectID, &cabal, &seal, &p.RedStones, &p.GreenStones, &p.BlueStones, &adena, &score); err != nil {
			return nil, fmt.Errorf("scan seven signs player: %w", err)
		}
		if p.Cabal, err = sevensigns.ParseCabal(cabal); err != nil {
			return nil, fmt.Errorf("seven signs player %d: %w", p.ObjectID, err)
		}
		if p.Seal, err = sevensigns.ParseSeal(seal); err != nil {
			return nil, fmt.Errorf("seven signs player %d: %w", p.ObjectID, err)
		}
		p.AncientAdena, p.ContributionScore = int(adena), int(score)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load seven signs players: %w", err)
	}
	return out, nil
}

// InsertPlayer writes a new sign-up's cabal and seal; its counters take the
// schema defaults.
func (s *SevenSignsStore) InsertPlayer(ctx context.Context, row sevensigns.PlayerRow) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO seven_signs (char_obj_id, cabal, seal) VALUES (?, ?, ?)`,
		row.ObjectID, row.Cabal.String(), row.Seal.String()); err != nil {
		return fmt.Errorf("insert seven signs player %d: %w", row.ObjectID, err)
	}
	return nil
}

// SavePlayers updates every given sign-up's row in one transaction. A row
// missing from the table stays missing.
func (s *SevenSignsStore) SavePlayers(ctx context.Context, rows []sevensigns.PlayerRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save seven signs players: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `UPDATE seven_signs SET cabal = ?, seal = ?, red_stones = ?, green_stones = ?,
	blue_stones = ?, ancient_adena_amount = ?, contribution_score = ? WHERE char_obj_id = ?`)
	if err != nil {
		return fmt.Errorf("save seven signs players: %w", err)
	}
	defer stmt.Close()
	for _, p := range rows {
		if _, err := stmt.ExecContext(ctx, p.Cabal.String(), p.Seal.String(), p.RedStones, p.GreenStones,
			p.BlueStones, p.AncientAdena, p.ContributionScore, p.ObjectID); err != nil {
			return fmt.Errorf("save seven signs player %d: %w", p.ObjectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seven signs players: %w", err)
	}
	return nil
}
