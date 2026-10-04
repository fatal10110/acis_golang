package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
)

// lotteryGameID is the games row family the lottery's rounds use.
const lotteryGameID = 1

// LotteryStore reads and writes the lottery's rounds in games, and reads
// the tickets sold for a round from items.
type LotteryStore struct {
	db *sql.DB
}

// NewLotteryStore returns a LotteryStore backed by db.
func NewLotteryStore(db *sql.DB) *LotteryStore {
	return &LotteryStore{db: db}
}

// LoadRounds returns every stored round, by number. A round counts as
// drawn only when its finished flag is 1.
func (s *LotteryStore) LoadRounds(ctx context.Context) ([]lottery.StoredRound, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT idnr, number1, number2, prize, newprize, prize1, prize2, prize3, enddate, finished FROM games WHERE id = ? ORDER BY idnr",
		lotteryGameID)
	if err != nil {
		return nil, fmt.Errorf("load lottery rounds: %w", err)
	}
	defer rows.Close()
	var out []lottery.StoredRound
	for rows.Next() {
		var r lottery.StoredRound
		var finished int32
		if err := rows.Scan(&r.ID, &r.Draw.Low, &r.Draw.High, &r.Prize, &r.NewPrize,
			&r.Draw.Prize1, &r.Draw.Prize2, &r.Draw.Prize3, &r.EndDate, &finished); err != nil {
			return nil, fmt.Errorf("load lottery rounds: %w", err)
		}
		r.Finished = finished == 1
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load lottery rounds: %w", err)
	}
	return out, nil
}

// InsertRound stores a new round, its jackpot also passing to the next one
// until it is drawn.
func (s *LotteryStore) InsertRound(ctx context.Context, id int32, endDate int64, prize int32) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO games(id, idnr, enddate, prize, newprize) VALUES (?, ?, ?, ?, ?)",
		lotteryGameID, id, endDate, prize, prize,
	); err != nil {
		return fmt.Errorf("store lottery round %d: %w", id, err)
	}
	return nil
}

// SavePrize sets round id's jackpot, and what passes to the next round.
func (s *LotteryStore) SavePrize(ctx context.Context, id, prize int32) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE games SET prize=?, newprize=? WHERE id = ? AND idnr = ?",
		prize, prize, lotteryGameID, id,
	); err != nil {
		return fmt.Errorf("save lottery round %d jackpot: %w", id, err)
	}
	return nil
}

// FinishRound marks round id drawn.
func (s *LotteryStore) FinishRound(ctx context.Context, id, prize, newPrize int32, draw lottery.Draw) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE games SET finished=1, prize=?, newprize=?, number1=?, number2=?, prize1=?, prize2=?, prize3=? WHERE id=? AND idnr=?",
		prize, newPrize, draw.Low, draw.High, draw.Prize1, draw.Prize2, draw.Prize3, lotteryGameID, id,
	); err != nil {
		return fmt.Errorf("store lottery round %d drawing: %w", id, err)
	}
	return nil
}

// Tickets returns the numbers of every stored ticket of round id, wherever
// it is kept.
func (s *LotteryStore) Tickets(ctx context.Context, id int32) ([]lottery.Numbers, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT enchant_level, custom_type2 FROM items WHERE item_id = ? AND custom_type1 = ?",
		lottery.TicketID, id)
	if err != nil {
		return nil, fmt.Errorf("read lottery round %d tickets: %w", id, err)
	}
	defer rows.Close()
	var out []lottery.Numbers
	for rows.Next() {
		var n lottery.Numbers
		if err := rows.Scan(&n.Low, &n.High); err != nil {
			return nil, fmt.Errorf("read lottery round %d tickets: %w", id, err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read lottery round %d tickets: %w", id, err)
	}
	return out, nil
}
