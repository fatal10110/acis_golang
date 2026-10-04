package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// olympiadCycleMemo is the server_memo variable holding the Olympiad cycle.
const olympiadCycleMemo = "olympiad_cycle"

// OlympiadStore reads and writes the Olympiad's cycle number, kept in
// server_memo, and the nobles' records in olympiad_nobles, whose month's
// standings are copied to olympiad_nobles_eom.
type OlympiadStore struct {
	db *sql.DB
}

// NewOlympiadStore returns an OlympiadStore backed by db.
func NewOlympiadStore(db *sql.DB) *OlympiadStore {
	return &OlympiadStore{db: db}
}

// LoadCycle returns the stored cycle number, found=false when none is
// stored. A value that is not a 32-bit decimal integer is an error.
func (s *OlympiadStore) LoadCycle(ctx context.Context) (int32, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = ?", olympiadCycleMemo).Scan(&value)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load olympiad cycle: %w", err)
	}
	cycle, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, false, fmt.Errorf("load olympiad cycle %q: %w", value, err)
	}
	return int32(cycle), true, nil
}

// SaveCycle stores the cycle number.
func (s *OlympiadStore) SaveCycle(ctx context.Context, cycle int32) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO server_memo (var, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)",
		olympiadCycleMemo, strconv.FormatInt(int64(cycle), 10),
	); err != nil {
		return fmt.Errorf("save olympiad cycle: %w", err)
	}
	return nil
}

// LoadNobles returns the record of every noble whose character still
// exists, with the character's current name.
func (s *OlympiadStore) LoadNobles(ctx context.Context) (map[int32]olympiad.Noble, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT olympiad_nobles.char_id, olympiad_nobles.class_id, characters.char_name, olympiad_nobles.olympiad_points,
			olympiad_nobles.competitions_done, olympiad_nobles.competitions_won, olympiad_nobles.competitions_lost,
			olympiad_nobles.competitions_drawn, olympiad_nobles.rewarded
		FROM olympiad_nobles, characters WHERE characters.obj_Id = olympiad_nobles.char_id`)
	if err != nil {
		return nil, fmt.Errorf("load olympiad nobles: %w", err)
	}
	defer rows.Close()
	nobles := map[int32]olympiad.Noble{}
	for rows.Next() {
		var (
			id       int32
			n        olympiad.Noble
			rewarded int
		)
		if err := rows.Scan(&id, &n.ClassID, &n.Name, &n.Points, &n.CompDone, &n.CompWon, &n.CompLost, &n.CompDrawn, &rewarded); err != nil {
			return nil, fmt.Errorf("load olympiad nobles: %w", err)
		}
		n.Rewarded = rewarded != 0
		nobles[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load olympiad nobles: %w", err)
	}
	return nobles, nil
}

// SaveNobles inserts each record, or updates its points, competition
// counts and rewarded flag when the noble already has one; a stored class
// is kept.
func (s *OlympiadStore) SaveNobles(ctx context.Context, nobles map[int32]olympiad.Noble) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save olympiad nobles: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO olympiad_nobles (`char_id`,`class_id`,`olympiad_points`,`competitions_done`,`competitions_won`,`competitions_lost`,`competitions_drawn`,`rewarded`) VALUES (?,?,?,?,?,?,?,?) "+
			"ON DUPLICATE KEY UPDATE olympiad_points=VALUES(olympiad_points), competitions_done=VALUES(competitions_done), competitions_won=VALUES(competitions_won), "+
			"competitions_lost=VALUES(competitions_lost), competitions_drawn=VALUES(competitions_drawn), rewarded=VALUES(rewarded)")
	if err != nil {
		return fmt.Errorf("save olympiad nobles: %w", err)
	}
	defer stmt.Close()
	for id, n := range nobles {
		if _, err := stmt.ExecContext(ctx, id, n.ClassID, n.Points, n.CompDone, n.CompWon, n.CompLost, n.CompDrawn, n.Rewarded); err != nil {
			return fmt.Errorf("save olympiad noble %d: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save olympiad nobles: %w", err)
	}
	return nil
}

// DeleteNobles removes every record.
func (s *OlympiadStore) DeleteNobles(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "TRUNCATE olympiad_nobles"); err != nil {
		return fmt.Errorf("delete olympiad nobles: %w", err)
	}
	return nil
}

// SnapshotMonth replaces the month's standings with a copy of every stored
// record but its rewarded flag.
func (s *OlympiadStore) SnapshotMonth(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "TRUNCATE olympiad_nobles_eom"); err != nil {
		return fmt.Errorf("clear olympiad month standings: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO olympiad_nobles_eom SELECT char_id, class_id, olympiad_points, competitions_done, competitions_won, competitions_lost, competitions_drawn FROM olympiad_nobles",
	); err != nil {
		return fmt.Errorf("copy olympiad month standings: %w", err)
	}
	return nil
}

// ClassLeaders returns the current names of the ten best nobles of classID
// in the month's standings with at least minMatches matches, by points,
// then matches, then wins.
func (s *OlympiadStore) ClassLeaders(ctx context.Context, classID, minMatches int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT characters.char_name FROM olympiad_nobles_eom, characters
		WHERE characters.obj_Id = olympiad_nobles_eom.char_id AND olympiad_nobles_eom.class_id = ? AND olympiad_nobles_eom.competitions_done >= ?
		ORDER BY olympiad_nobles_eom.olympiad_points DESC, olympiad_nobles_eom.competitions_done DESC, olympiad_nobles_eom.competitions_won DESC LIMIT 10`,
		classID, minMatches)
	if err != nil {
		return nil, fmt.Errorf("load olympiad class %d leaders: %w", classID, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("load olympiad class %d leaders: %w", classID, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load olympiad class %d leaders: %w", classID, err)
	}
	return names, nil
}

// SaveFight stores the result of one Olympiad match.
func (s *OlympiadStore) SaveFight(ctx context.Context, f olympiad.Fight) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO olympiad_fights (charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed) VALUES (?,?,?,?,?,?,?,?)",
		f.CharOneID, f.CharTwoID, f.CharOneClass, f.CharTwoClass, f.Winner, f.Start, f.Time, f.Classed,
	); err != nil {
		return fmt.Errorf("save olympiad fight %d-%d: %w", f.CharOneID, f.CharTwoID, err)
	}
	return nil
}
