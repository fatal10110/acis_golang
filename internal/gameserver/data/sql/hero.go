package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
)

// heroItemIDs are the hero weapons and the hero circlet, every item the
// end of an Olympiad takes back.
const heroItemIDs = "6842, 6611, 6612, 6613, 6614, 6615, 6616, 6617, 6618, 6619, 6620, 6621"

// HeroStore reads and writes the heroes in heroes and their diary entries
// in heroes_diary.
type HeroStore struct {
	db *sql.DB
}

// NewHeroStore returns a HeroStore backed by db.
func NewHeroStore(db *sql.DB) *HeroStore {
	return &HeroStore{db: db}
}

// LoadHeroes returns every heroes row whose character still exists, with
// the character's current name and clan.
func (s *HeroStore) LoadHeroes(ctx context.Context) ([]hero.Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT heroes.char_id, characters.char_name, heroes.class_id, heroes.count, heroes.played, heroes.active,
			COALESCE(characters.clanid, 0)
		FROM heroes, characters WHERE characters.obj_Id = heroes.char_id ORDER BY heroes.char_id`)
	if err != nil {
		return nil, fmt.Errorf("load heroes: %w", err)
	}
	defer rows.Close()
	var out []hero.Row
	for rows.Next() {
		var (
			r              hero.Row
			played, active int
		)
		if err := rows.Scan(&r.ObjectID, &r.Name, &r.ClassID, &r.Count, &played, &active, &r.ClanID); err != nil {
			return nil, fmt.Errorf("load heroes: %w", err)
		}
		r.Played, r.Active = played == 1, active == 1
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load heroes: %w", err)
	}
	return out, nil
}

// ClanID returns the clan objectID's character belongs to, 0 for none.
func (s *HeroStore) ClanID(ctx context.Context, objectID int32) (int32, bool, error) {
	var clanID int32
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(clanid, 0) FROM characters WHERE obj_Id = ?", objectID).Scan(&clanID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load hero %d clan: %w", objectID, err)
	}
	return clanID, true, nil
}

// ResetPlayed marks every stored hero as no longer of the running era.
func (s *HeroStore) ResetPlayed(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE heroes SET played = 0"); err != nil {
		return fmt.Errorf("reset heroes: %w", err)
	}
	return nil
}

// BestNoble returns the noble of classID ranked first by points, then
// matches, then wins, among the nobles whose character exists, with at
// least minMatches matches and a win. Nobles tied on all three go by
// character id.
func (s *HeroStore) BestNoble(ctx context.Context, classID, minMatches int) (int32, string, bool, error) {
	var (
		id   int32
		name string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT olympiad_nobles.char_id, characters.char_name FROM olympiad_nobles, characters
		WHERE characters.obj_Id = olympiad_nobles.char_id AND olympiad_nobles.class_id = ?
			AND olympiad_nobles.competitions_done >= ? AND olympiad_nobles.competitions_won > 0
		ORDER BY olympiad_nobles.olympiad_points DESC, olympiad_nobles.competitions_done DESC,
			olympiad_nobles.competitions_won DESC, olympiad_nobles.char_id LIMIT 1`,
		classID, minMatches,
	).Scan(&id, &name)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("load hero to be of class %d: %w", classID, err)
	}
	return id, name, true, nil
}

// DeleteHeroItems deletes every hero item whose owner is not a character
// with an access level above 0.
func (s *HeroStore) DeleteHeroItems(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM items WHERE item_id IN ("+heroItemIDs+") AND owner_id NOT IN (SELECT obj_Id FROM characters WHERE accesslevel > 0)",
	); err != nil {
		return fmt.Errorf("delete hero items: %w", err)
	}
	return nil
}

// SaveHeroes inserts each hero, or updates its count and flags when it is
// stored already; a stored class and message are kept.
func (s *HeroStore) SaveHeroes(ctx context.Context, heroes map[int32]hero.Hero) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save heroes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO heroes (char_id, class_id, count, played, active) VALUES (?,?,?,?,?) "+
			"ON DUPLICATE KEY UPDATE count=VALUES(count),played=VALUES(played),active=VALUES(active)")
	if err != nil {
		return fmt.Errorf("save heroes: %w", err)
	}
	defer stmt.Close()
	for id, h := range heroes {
		if _, err := stmt.ExecContext(ctx, id, h.ClassID, h.Count, boolInt(h.Played), boolInt(h.Active)); err != nil {
			return fmt.Errorf("save hero %d: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save heroes: %w", err)
	}
	return nil
}

// AddDiaryEntry stores one diary entry made at at, in Unix milliseconds.
func (s *HeroStore) AddDiaryEntry(ctx context.Context, objectID int32, at int64, action, param int) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO heroes_diary (char_id, time, action, param) values(?,?,?,?)", objectID, at, action, param,
	); err != nil {
		return fmt.Errorf("add hero %d diary entry: %w", objectID, err)
	}
	return nil
}
