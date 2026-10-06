package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// QuestStore reads character_quests rows.
type QuestStore struct {
	db *sql.DB
}

// NewQuestStore returns a QuestStore backed by db.
func NewQuestStore(db *sql.DB) *QuestStore {
	return &QuestStore{db: db}
}

// ListByOwner returns ownerID's journal rows in primary-key order (quest
// name, then variable), the order the journal is rebuilt in. A NULL value
// reads as "".
func (s *QuestStore) ListByOwner(ctx context.Context, ownerID int32) ([]questlog.Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name,var,value FROM character_quests WHERE charId=? ORDER BY name,var`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list quests for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	var out []questlog.Row
	for rows.Next() {
		var r questlog.Row
		var value sql.NullString
		if err := rows.Scan(&r.Quest, &r.Var, &value); err != nil {
			return nil, fmt.Errorf("list quests for owner %d: %w", ownerID, err)
		}
		r.Value = value.String
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list quests for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// ApplyJournal runs ownerID's journal writes, in order, in one transaction:
// either every write lands or none does.
func (s *QuestStore) ApplyJournal(ctx context.Context, ownerID int32, writes []questlog.Write) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("write quests for owner %d: %w", ownerID, err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, w := range writes {
		var err error
		switch w.Op {
		case questlog.OpSet:
			_, err = tx.ExecContext(ctx,
				`INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE value=VALUES(value)`,
				ownerID, w.Quest, w.Var, w.Value)
		case questlog.OpUnset:
			_, err = tx.ExecContext(ctx, `DELETE FROM character_quests WHERE charId=? AND name=? AND var=?`, ownerID, w.Quest, w.Var)
		case questlog.OpDelete:
			_, err = tx.ExecContext(ctx, `DELETE FROM character_quests WHERE charId=? AND name=?`, ownerID, w.Quest)
		case questlog.OpComplete:
			_, err = tx.ExecContext(ctx, `DELETE FROM character_quests WHERE charId=? AND name=? AND var<>'<state>'`, ownerID, w.Quest)
		default:
			err = fmt.Errorf("unknown journal write %d", w.Op)
		}
		if err != nil {
			return fmt.Errorf("write quest %s for owner %d: %w", w.Quest, ownerID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("write quests for owner %d: %w", ownerID, err)
	}
	return nil
}
