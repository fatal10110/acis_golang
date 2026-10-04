package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	exp, COALESCE(expBeforeDeath,0), sp, COALESCE(karma,0), COALESCE(pvpkills,0), COALESCE(pkkills,0), COALESCE(clanid,0),
	COALESCE(race,0), COALESCE(classid,0), base_class,
	COALESCE(deletetime,0), COALESCE(title,''), COALESCE(accesslevel,0), COALESCE(hero,0), COALESCE(lastAccess,0),
	COALESCE(onlinetime,0),
	COALESCE(death_penalty_level,0), rec_have, rec_left,
	clan_join_expiry_time, clan_create_expiry_time,
	COALESCE(punish_level,0), COALESCE(punish_timer,0), COALESCE(wantspeace,0), nobless`

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
	resources := c.ResourceValues()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO characters
				(account_name, obj_Id, char_name, level, maxHp, curHp, maxCp, curCp, maxMp, curMp,
				 face, hairStyle, hairColor, sex, heading, x, y, z, exp, sp, race, classid, base_class, title, accesslevel, online, lastAccess)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.AccountName, c.ID, c.Name, c.CharLevel, resources.MaxHP, resources.CurrentHP, resources.MaxCP, resources.CurrentCP, resources.MaxMP, resources.CurrentMP,
		c.Face, c.HairStyle, c.HairColor, byte(c.Sex()), c.LastHeading, c.Location.X, c.Location.Y, c.Location.Z,
		c.Exp, c.SP, int(c.Race), c.ClassID(), c.BaseClassID(), c.Title(), c.AccessLevel, 0, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("create character %q: %w", c.Name, err)
	}
	return nil
}

// Save persists a character's progress — the active and base classes, the base
// class's level, exp and sp, expBeforeDeath, cur/max HP/CP/MP,
// karma/pvpkills/pkkills, death_penalty_level, the accumulated session
// playtime, the personal-surrender flag, the noblesse status and each subclass's progression — so a later reload reflects everything gained
// since the last save instead of the row's creation-time values. Location and
// appearance columns are not written here. The row is also marked online:
// Save only runs for characters currently in game.
func (s *CharacterStore) Save(ctx context.Context, st player.SaveState) error {
	resources, progression := st.Resources, st.Progression
	_, err := s.db.ExecContext(ctx,
		`UPDATE characters SET level = ?, maxHp = ?, curHp = ?, maxCp = ?, curCp = ?, maxMp = ?, curMp = ?, exp = ?, expBeforeDeath = ?, sp = ?, karma = ?, pvpkills = ?, pkkills = ?, classid = ?, base_class = ?, death_penalty_level = ?, onlinetime = ?, wantspeace = ?, nobless = ?, online = 1
			 WHERE obj_Id = ?`,
		progression.CharLevel, resources.MaxHP, resources.CurrentHP, resources.MaxCP, resources.CurrentCP, resources.MaxMP, resources.CurrentMP,
		progression.Exp, progression.ExpBeforeDeath, progression.SP, st.Karma, st.PvPKills, st.PKKills, st.ClassID, st.BaseClassID, st.DeathPenaltyLevel, st.OnlineTime,
		st.WantsPeace, st.Noble, st.ID,
	)
	if err != nil {
		return fmt.Errorf("save character %d: %w", st.ID, err)
	}
	// The characters row carries the base class's progression; each
	// subclass keeps its own in its row.
	return updateSubclasses(ctx, s.db, st.ID, st.Subclasses)
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
	var hero int
	var maxHP, curHP, maxCP, curCP, maxMP, curMP float64
	var deathPenaltyLevel, onlineTime int
	var recHave, recLeft int
	var clanID int32
	var title string
	var clanJoinExpiry, clanCreateExpiry int64
	var punishLevel int
	var punishTimer int64
	var wantsPeace int
	var noble int
	var baseClassID int

	err := row.Scan(
		&c.ID, &c.AccountName, &c.Name,
		&c.CharLevel, &maxHP, &curHP, &maxCP, &curCP, &maxMP, &curMP,
		&c.Face, &c.HairStyle, &c.HairColor, &sex,
		&c.LastHeading, &c.Location.X, &c.Location.Y, &c.Location.Z,
		&c.Exp, &c.ExpBeforeDeath, &c.SP, &c.KarmaPoints, &c.PvPKills, &c.PKKills, &clanID,
		&race, &classID, &baseClassID,
		&c.DeleteAt, &title, &c.AccessLevel, &hero, &c.LastAccess,
		&onlineTime,
		&deathPenaltyLevel, &recHave, &recLeft,
		&clanJoinExpiry, &clanCreateExpiry,
		&punishLevel, &punishTimer, &wantsPeace, &noble,
	)
	if err != nil {
		return nil, err
	}
	c.SetSex(player.Sex(sex))
	c.SetClanID(clanID)
	c.SetTitle(title)
	c.SetClanJoinExpiryTime(clanJoinExpiry)
	c.SetClanCreateExpiryTime(clanCreateExpiry)
	// Only a stored 1 raises the flag.
	c.SetWantsPeace(wantsPeace == 1)
	c.SetNoble(noble == 1)
	c.Race = player.Race(race)
	c.SetClassID(classID)
	c.SetBaseClassID(baseClassID)
	c.SetHero(hero != 0)
	c.SetDeathPenaltyLevel(deathPenaltyLevel)
	c.SetRecommendationCounts(recHave, recLeft)
	c.RestorePunishment(punishLevel, punishTimer)
	// The playtime clock starts at restore: every later save persists the
	// restored base plus the elapsed session time.
	c.SetOnlineTime(int64(onlineTime), time.Now())
	c.SetResourceValues(player.Resources{
		MaxHP: maxHP, CurrentHP: curHP,
		MaxCP: maxCP, CurrentCP: curCP,
		MaxMP: maxMP, CurrentMP: curMP,
	})
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

// SetDeathPenaltyLevel updates the character's persisted death-penalty
// debuff level.
func (s *CharacterStore) SetDeathPenaltyLevel(ctx context.Context, objectID int32, level int) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET death_penalty_level = ? WHERE obj_Id = ?", level, objectID); err != nil {
		return fmt.Errorf("set death penalty level for %d: %w", objectID, err)
	}
	return nil
}

// SetOnline marks the character in game and stamps lastAccess (epoch
// milliseconds). It runs when a client enters the world, so external DB
// consumers see the character as online from login until SetOffline.
func (s *CharacterStore) SetOnline(ctx context.Context, objectID int32, lastAccess int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET online = 1, lastAccess = ? WHERE obj_Id = ?", lastAccess, objectID); err != nil {
		return fmt.Errorf("set online recency for %d: %w", objectID, err)
	}
	return nil
}

// SetOffline marks the character offline and persists lastAccess (epoch
// milliseconds), so a later char-select reload highlights the most recently
// played character.
func (s *CharacterStore) SetOffline(ctx context.Context, objectID int32, lastAccess int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET online = 0, lastAccess = ? WHERE obj_Id = ?", lastAccess, objectID); err != nil {
		return fmt.Errorf("set offline recency for %d: %w", objectID, err)
	}
	return nil
}

// Purge removes the character row for objectID together with every row it
// owns - its items, shortcuts, hennas, recipe book, subclasses, skills,
// skill-save state, pets, item augmentations, the friend and block
// relations naming it on either side, and its Olympiad record - as one transaction, so a failure or
// cancellation partway through leaves all of them in place instead of
// orphaning owned rows behind a deleted character. Pets and augmentations are deleted
// before items, since both key off the character's still-live item ids. It
// reports whether a character row was deleted.
func (s *CharacterStore) Purge(ctx context.Context, objectID int32) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin purge character %d: %w", objectID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, "DELETE FROM characters WHERE obj_Id = ?", objectID)
	if err != nil {
		return false, fmt.Errorf("purge character %d: %w", objectID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("purge character %d: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_skills WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d skills: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_skills_save WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d skills_save: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pets WHERE item_obj_id IN (SELECT object_id FROM items WHERE items.owner_id = ?)", objectID); err != nil {
		return false, fmt.Errorf("purge character %d pets: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM augmentations WHERE item_oid IN (SELECT object_id FROM items WHERE items.owner_id = ?)", objectID); err != nil {
		return false, fmt.Errorf("purge character %d augmentations: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM items WHERE owner_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d items: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_shortcuts WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d shortcuts: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_macroses WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d macros: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_hennas WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d hennas: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_recipebook WHERE charId = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d recipe book: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_subclasses WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d subclasses: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_relations WHERE char_id = ? OR friend_id = ?", objectID, objectID); err != nil {
		return false, fmt.Errorf("purge character %d relations: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM olympiad_nobles WHERE char_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d olympiad record: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_raid_points WHERE char_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d raid points: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM seven_signs WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d seven signs: %w", objectID, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit purge character %d: %w", objectID, err)
	}
	committed = true
	return n > 0, nil
}
