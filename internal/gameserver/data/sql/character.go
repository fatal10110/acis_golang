package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ErrCharacterNotFound is returned when no characters row matches the given
// object id.
var ErrCharacterNotFound = errors.New("character not found")

// characterColumns lists, in scan order, every characters column this store
// reads. Columns the game hasn't grown a use for yet (clan/social/event
// state) are left untouched by both this list and Create, so they keep
// whatever default the schema gives them. Nullable columns are wrapped in
// COALESCE so a row missing an optional value (e.g. one never assigned a
// world position) scans as the type's zero value instead of failing.
const characterColumns = `obj_Id, account_name, char_name,
	COALESCE(level,0), COALESCE(maxHp,0), COALESCE(curHp,0),
	COALESCE(maxCp,0), COALESCE(curCp,0), COALESCE(maxMp,0), COALESCE(curMp,0),
	COALESCE(face,0), COALESCE(hairStyle,0), COALESCE(hairColor,0), COALESCE(sex,0),
	COALESCE(heading,0), COALESCE(x,0), COALESCE(y,0), COALESCE(z,0),
	exp, sp, COALESCE(karma,0), COALESCE(pvpkills,0), COALESCE(pkkills,0), COALESCE(clanid,0),
	COALESCE(race,0), COALESCE(classid,0), base_class,
	COALESCE(deletetime,0), COALESCE(title,''), COALESCE(accesslevel,0), COALESCE(lastAccess,0)`

// CharacterStore reads and writes the characters table.
type CharacterStore struct {
	db *sql.DB
}

// NewCharacterStore returns a CharacterStore backed by db.
func NewCharacterStore(db *sql.DB) *CharacterStore {
	return &CharacterStore{db: db}
}

// Create inserts c as a new characters row. It writes exactly the columns a
// freshly created character has values for; clan and every other column a
// character only gains once it is actually played keep the schema's own
// default until something sets them.
func (s *CharacterStore) Create(ctx context.Context, c *player.Character) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO characters
			(account_name, obj_Id, char_name, level, maxHp, curHp, maxCp, curCp, maxMp, curMp,
			 face, hairStyle, hairColor, sex, heading, x, y, z, exp, sp, race, classid, base_class, title, accesslevel)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.AccountName, c.ID, c.Name, c.Level, c.MaxHP, c.CurHP, c.MaxCP, c.CurCP, c.MaxMP, c.CurMP,
		c.Face, c.HairStyle, c.HairColor, byte(c.Sex), c.LastHeading, c.Location.X, c.Location.Y, c.Location.Z,
		c.Exp, c.SP, int(c.Race), c.ClassID, c.BaseClassID, c.Title, c.AccessLevel,
	)
	if err != nil {
		return fmt.Errorf("create character %q: %w", c.Name, err)
	}
	return nil
}

// Get returns the character with the given object id, or
// ErrCharacterNotFound if no such row exists.
func (s *CharacterStore) Get(ctx context.Context, objectID int32) (*player.Character, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+characterColumns+" FROM characters WHERE obj_Id = ?", objectID)
	c, err := scanCharacter(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCharacterNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query character %d: %w", objectID, err)
	}
	return c, nil
}

// ListByAccount returns every character on accountName, ordered by object
// id for a stable, repeatable result. A character whose account has none
// returns an empty, non-nil slice.
func (s *CharacterStore) ListByAccount(ctx context.Context, accountName string) ([]*player.Character, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+characterColumns+" FROM characters WHERE account_name = ? ORDER BY obj_Id ASC", accountName)
	if err != nil {
		return nil, fmt.Errorf("list characters for %q: %w", accountName, err)
	}
	defer rows.Close()

	out := []*player.Character{}
	for rows.Next() {
		c, err := scanCharacter(rows)
		if err != nil {
			return nil, fmt.Errorf("list characters for %q: %w", accountName, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list characters for %q: %w", accountName, err)
	}
	return out, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCharacter(row rowScanner) (*player.Character, error) {
	var c player.Character
	var sex byte
	var race, classID int

	err := row.Scan(
		&c.ID, &c.AccountName, &c.Name,
		&c.Level, &c.MaxHP, &c.CurHP, &c.MaxCP, &c.CurCP, &c.MaxMP, &c.CurMP,
		&c.Face, &c.HairStyle, &c.HairColor, &sex,
		&c.LastHeading, &c.Location.X, &c.Location.Y, &c.Location.Z,
		&c.Exp, &c.SP, &c.Karma, &c.PvPKills, &c.PKKills, &c.ClanID,
		&race, &classID, &c.BaseClassID,
		&c.DeleteAt, &c.Title, &c.AccessLevel, &c.LastAccess,
	)
	if err != nil {
		return nil, err
	}
	c.Sex = player.Sex(sex)
	c.Race = player.Race(race)
	c.ClassID = classID
	return &c, nil
}

// CountByAccount returns how many characters exist on accountName. Matching
// is case-insensitive, since account names are.
func (s *CharacterStore) CountByAccount(ctx context.Context, accountName string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE LOWER(account_name) = LOWER(?)", accountName).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count characters for %q: %w", accountName, err)
	}
	return n, nil
}

// NameTaken reports whether a character named name already exists.
// Matching is case-insensitive, since character names are.
func (s *CharacterStore) NameTaken(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE LOWER(char_name) = LOWER(?)", name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check name %q: %w", name, err)
	}
	return n > 0, nil
}

// SetDeleteAt updates the character's persisted deletion deadline (epoch
// milliseconds; 0 clears it, un-scheduling the deletion).
func (s *CharacterStore) SetDeleteAt(ctx context.Context, objectID int32, at int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET deletetime = ? WHERE obj_Id = ?", at, objectID); err != nil {
		return fmt.Errorf("set delete time for %d: %w", objectID, err)
	}
	return nil
}

// SetPosition updates the character's persisted world position and facing.
func (s *CharacterStore) SetPosition(ctx context.Context, objectID int32, loc location.Location, heading int) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET heading = ?, x = ?, y = ?, z = ? WHERE obj_Id = ?", heading, loc.X, loc.Y, loc.Z, objectID); err != nil {
		return fmt.Errorf("set position for %d: %w", objectID, err)
	}
	return nil
}

// Delete removes the character row for objectID. It reports whether a row
// was deleted.
func (s *CharacterStore) Delete(ctx context.Context, objectID int32) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM characters WHERE obj_Id = ?", objectID)
	if err != nil {
		return false, fmt.Errorf("delete character %d: %w", objectID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete character %d: %w", objectID, err)
	}
	return n > 0, nil
}
