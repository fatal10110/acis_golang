package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
)

// DerbyStore reads and writes the monster race track's records in
// mdt_history and the lanes' stakes in mdt_bets.
type DerbyStore struct {
	db *sql.DB
}

var _ derby.Store = (*DerbyStore)(nil)

// NewDerbyStore returns a DerbyStore backed by db.
func NewDerbyStore(db *sql.DB) *DerbyStore {
	return &DerbyStore{db: db}
}

// LoadHistory returns every stored race record.
func (s *DerbyStore) LoadHistory(ctx context.Context) ([]derby.History, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT race_id, first, second, odd_rate FROM mdt_history")
	if err != nil {
		return nil, fmt.Errorf("load derby history: %w", err)
	}
	defer rows.Close()
	var out []derby.History
	for rows.Next() {
		var h derby.History
		var first, second sql.NullInt64
		var odd sql.NullFloat64
		var race sql.NullInt64
		if err := rows.Scan(&race, &first, &second, &odd); err != nil {
			return nil, fmt.Errorf("load derby history: %w", err)
		}
		h.RaceID, h.First, h.Second, h.OddRate = int(race.Int64), int(first.Int64), int(second.Int64), odd.Float64
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load derby history: %w", err)
	}
	return out, nil
}

// LoadBets returns every lane's stored stake.
func (s *DerbyStore) LoadBets(ctx context.Context) ([]derby.Bet, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT lane_id, bet FROM mdt_bets")
	if err != nil {
		return nil, fmt.Errorf("load derby bets: %w", err)
	}
	defer rows.Close()
	var out []derby.Bet
	for rows.Next() {
		var lane, bet sql.NullInt64
		if err := rows.Scan(&lane, &bet); err != nil {
			return nil, fmt.Errorf("load derby bets: %w", err)
		}
		out = append(out, derby.Bet{Lane: int(lane.Int64), Amount: bet.Int64})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load derby bets: %w", err)
	}
	return out, nil
}

// SaveBet stores a lane's total stake.
func (s *DerbyStore) SaveBet(ctx context.Context, bet derby.Bet) error {
	if _, err := s.db.ExecContext(ctx, "REPLACE INTO mdt_bets (lane_id, bet) VALUES (?,?)", bet.Lane, bet.Amount); err != nil {
		return fmt.Errorf("save derby bet: %w", err)
	}
	return nil
}

// ClearBets sets every stored stake to 0.
func (s *DerbyStore) ClearBets(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE mdt_bets SET bet = 0"); err != nil {
		return fmt.Errorf("clear derby bets: %w", err)
	}
	return nil
}

// SaveHistory stores a finished race's record.
func (s *DerbyStore) SaveHistory(ctx context.Context, h derby.History) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO mdt_history (race_id, first, second, odd_rate) VALUES (?,?,?,?)", h.RaceID, h.First, h.Second, h.OddRate); err != nil {
		return fmt.Errorf("save derby history: %w", err)
	}
	return nil
}
