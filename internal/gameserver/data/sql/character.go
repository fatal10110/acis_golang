package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
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
	COALESCE(deletetime,0), COALESCE(title,''), COALESCE(accesslevel,0), COALESCE(lastAccess,0),
	COALESCE(onlinetime,0),
	COALESCE(death_penalty_level,0), rec_have, rec_left,
	clan_join_expiry_time, clan_create_expiry_time,
	COALESCE(punish_level,0), COALESCE(punish_timer,0), COALESCE(wantspeace,0), nobless, isin7sdungeon, varka_ketra_ally`

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
	return insertCharacter(ctx, s.db, c)
}

// CreateWithStarters inserts c as Create does together with its starter
// items and its starter shortcuts of the base class, as one transaction: a
// failure at any row leaves none of them.
func (s *CharacterStore) CreateWithStarters(ctx context.Context, c *player.Character, items []item.Instance, shortcuts []shortcut.Shortcut) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create character %q: begin: %w", c.CharacterName(), err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertCharacter(ctx, tx, c); err != nil {
		return err
	}
	for _, inst := range items {
		if err := insertItem(ctx, tx, c.ID, inst); err != nil {
			return err
		}
	}
	for _, sc := range shortcuts {
		if err := saveShortcut(ctx, tx, c.ID, 0, sc); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("create character %q: commit: %w", c.CharacterName(), err)
	}
	return nil
}

// execer runs a write on a handle or within a transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertCharacter(ctx context.Context, db execer, c *player.Character) error {
	resources := c.ResourceValues()
	_, err := db.ExecContext(ctx,
		`INSERT INTO characters
				(account_name, obj_Id, char_name, level, maxHp, curHp, maxCp, curCp, maxMp, curMp,
				 face, hairStyle, hairColor, sex, heading, x, y, z, exp, sp, race, classid, base_class, title, accesslevel, online, lastAccess)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.AccountName, c.ID, c.CharacterName(), c.CharLevel, resources.MaxHP, resources.CurrentHP, resources.MaxCP, resources.CurrentCP, resources.MaxMP, resources.CurrentMP,
		c.Face, c.HairStyle, c.HairColor, byte(c.Sex()), c.LastHeading, c.Location.X, c.Location.Y, c.Location.Z,
		c.Exp, c.SP, int(c.Race), c.ClassID(), c.BaseClassID(), c.Title(), c.AccessLevel, 0, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("create character %q: %w", c.CharacterName(), err)
	}
	return nil
}

// Save persists a character's progress — the active and base classes, the base
// class's level, exp and sp, expBeforeDeath, cur/max HP/CP/MP,
// karma/pvpkills/pkkills (clamped to their column ranges), death_penalty_level, the accumulated session
// playtime, the personal-surrender flag, the noblesse status, the Seven Signs
// dungeon membership, the Ketra/Varka faction standing, the face and hair,
// the access level, the title and clan penalties (see clanColumnsQuery),
// the position and heading, and each subclass's progression — so a later reload reflects everything
// gained since the last save instead of the row's creation-time values.
// The row is also marked online: Save only runs for characters currently in
// game. Online is 1, or 2 for a character lingering after its connection
// dropped. lastAccess is left as it is.
//
// The characters row is one statement; with subclasses it and their rows
// commit as one transaction, so a failure leaves all of them as they were.
func (s *CharacterStore) Save(ctx context.Context, st player.SaveState) error {
	return s.save(ctx, st, saveCharacterQuery+" WHERE obj_Id = ?", onlineColumn(st.ClientDetached))
}

// SaveOffline is Save for a character leaving the game: the same write marks
// the row offline and stamps lastAccess (epoch milliseconds), so a later
// char-select reload highlights the most recently played character.
func (s *CharacterStore) SaveOffline(ctx context.Context, st player.SaveState, lastAccess int64) error {
	return s.save(ctx, st, saveCharacterQuery+", lastAccess = ? WHERE obj_Id = ?", 0, lastAccess)
}

// saveCharacterQuery is the column list Save and SaveOffline write, ending
// with online; each completes it with its own tail.
const saveCharacterQuery = `UPDATE characters SET level = ?, maxHp = ?, curHp = ?, maxCp = ?, curCp = ?, maxMp = ?, curMp = ?, exp = ?, expBeforeDeath = ?, sp = ?, karma = ?, pvpkills = ?, pkkills = ?, classid = ?, base_class = ?, death_penalty_level = ?, onlinetime = ?, wantspeace = ?, nobless = ?, isin7sdungeon = ?, varka_ketra_ally = ?, face = ?, hairStyle = ?, hairColor = ?, accesslevel = ?, ` + clanColumnsQuery + `, heading = ?, x = ?, y = ?, z = ?, online = ?`

// clanColumnsQuery writes the title and the clan penalties only while the
// row still records the clan the copy was taken in. The clan rows write
// these columns too, on the same lane, but a member removed by someone
// else has its row written before its live character leaves the clan: a
// copy taken in between, by an autosave or the logout save, would put its
// old title back and lift the join penalty the removal stored.
const clanColumnsQuery = `title = IF(COALESCE(clanid, 0) = ?, ?, title), clan_join_expiry_time = IF(COALESCE(clanid, 0) = ?, ?, clan_join_expiry_time), clan_create_expiry_time = IF(COALESCE(clanid, 0) = ?, ?, clan_create_expiry_time)`

// save runs query, saveCharacterQuery completed by a tail whose arguments
// are tail followed by the character's id, and writes st's subclass rows in
// the same transaction.
func (s *CharacterStore) save(ctx context.Context, st player.SaveState, query string, tail ...any) error {
	resources, progression := st.Resources, st.Progression
	args := append([]any{
		progression.CharLevel, resources.MaxHP, resources.CurrentHP, resources.MaxCP, resources.CurrentCP, resources.MaxMP, resources.CurrentMP,
		progression.Exp, progression.ExpBeforeDeath, progression.SP, karmaColumn(int64(st.Karma)), killsColumn(int64(st.PvPKills)), killsColumn(int64(st.PKKills)), st.ClassID, st.BaseClassID, st.DeathPenaltyLevel, st.OnlineTime,
		st.WantsPeace, st.Noble, st.In7sDungeon, st.VarkaKetraAlliance, st.Face, st.HairStyle, st.HairColor, st.AccessLevel,
		st.ClanID, st.Title, st.ClanID, st.ClanJoinExpiry, st.ClanID, st.ClanCreateExpiry,
		st.Heading, st.Location.X, st.Location.Y, st.Location.Z,
	}, tail...)
	args = append(args, st.ID)
	if len(st.Subclasses) == 0 {
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("save character %d: %w", st.ID, err)
		}
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save character %d: begin: %w", st.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("save character %d: %w", st.ID, err)
	}
	// The characters row carries the base class's progression; each
	// subclass keeps its own in its row.
	if err := updateSubclasses(ctx, tx, st.ID, st.Subclasses); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save character %d: commit: %w", st.ID, err)
	}
	return nil
}

// Column ranges of the characters counters: pvpkills and pkkills are
// SMALLINT UNSIGNED, karma is INT UNSIGNED.
const (
	maxKillsColumn = 65535
	maxKarmaColumn = 4294967295
)

// killsColumn is a PvP or PK count as its column stores it: clamped to the
// column range, so a count past it never fails the save of the whole row.
func killsColumn(n int64) int64 { return min(max(n, 0), maxKillsColumn) }

// karmaColumn is karma clamped to the characters.karma range, for the same
// reason as killsColumn.
func karmaColumn(n int64) int64 { return min(max(n, 0), maxKarmaColumn) }

// onlineColumn is the characters.online value of a character in game: 1,
// or 2 once its client detached.
func onlineColumn(clientDetached bool) int {
	if clientDetached {
		return 2
	}
	return 1
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
	var maxHP, curHP, maxCP, curCP, maxMP, curMP float64
	var deathPenaltyLevel, onlineTime int
	var recHave, recLeft int
	var clanID int32
	var name, title string
	var clanJoinExpiry, clanCreateExpiry int64
	var punishLevel int
	var punishTimer int64
	var wantsPeace int
	var noble int
	var in7sDungeon int
	var varkaKetraAlliance int
	var baseClassID int

	err := row.Scan(
		&c.ID, &c.AccountName, &name,
		&c.CharLevel, &maxHP, &curHP, &maxCP, &curCP, &maxMP, &curMP,
		&c.Face, &c.HairStyle, &c.HairColor, &sex,
		&c.LastHeading, &c.Location.X, &c.Location.Y, &c.Location.Z,
		&c.Exp, &c.ExpBeforeDeath, &c.SP, &c.KarmaPoints, &c.PvPKills, &c.PKKills, &clanID,
		&race, &classID, &baseClassID,
		&c.DeleteAt, &title, &c.AccessLevel, &c.LastAccess,
		&onlineTime,
		&deathPenaltyLevel, &recHave, &recLeft,
		&clanJoinExpiry, &clanCreateExpiry,
		&punishLevel, &punishTimer, &wantsPeace, &noble, &in7sDungeon, &varkaKetraAlliance,
	)
	if err != nil {
		return nil, err
	}
	c.SetName(name)
	c.SetSex(player.Sex(sex))
	c.SetClanID(clanID)
	c.SetTitle(title)
	c.SetClanJoinExpiryTime(clanJoinExpiry)
	c.SetClanCreateExpiryTime(clanCreateExpiry)
	// Only a stored 1 raises the flag.
	c.SetWantsPeace(wantsPeace == 1)
	c.SetNoble(noble == 1)
	c.SetIn7sDungeon(in7sDungeon == 1)
	c.SetVarkaKetraAlliance(varkaKetraAlliance)
	c.Race = player.Race(race)
	c.SetClassID(classID)
	c.SetBaseClassID(baseClassID)
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
// is case-insensitive, since account names are: the column's collation
// folds case, so the comparison is the column's own, which its index
// serves.
func (s *CharacterStore) CountByAccount(ctx context.Context, accountName string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE account_name = ?", accountName).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count characters for %q: %w", accountName, err)
	}
	return n, nil
}

// NameTaken reports whether a character named name already exists.
// Matching is case-insensitive, since character names are: the column's
// collation folds case, so the comparison is the column's own, which its
// index serves.
func (s *CharacterStore) NameTaken(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE char_name = ?", name).Scan(&n)
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

// SetOnline marks the character in game and stamps lastAccess (epoch
// milliseconds). It runs when a client enters the world, so external DB
// consumers see the character as online from login until SaveOffline.
func (s *CharacterStore) SetOnline(ctx context.Context, objectID int32, lastAccess int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET online = 1, lastAccess = ? WHERE obj_Id = ?", lastAccess, objectID); err != nil {
		return fmt.Errorf("set online recency for %d: %w", objectID, err)
	}
	return nil
}

// Purge removes the character row for objectID together with every row it
// owns - its items, the items its pets carry, shortcuts, macros, memos,
// quest journal, hennas, recipe book, subclasses, skills, skill-save state,
// pets, item augmentations, the friend and block relations naming it on
// either side, its Olympiad and hero records, raid points, Seven Signs
// record, bookmarks, buffer schemes and the marriage it is in on either
// side - as one transaction, so a failure or cancellation partway through
// leaves all of them in place instead of orphaning owned rows behind a
// deleted character. The same transaction retires the character's object
// id, which is never handed out again (#4676).
//
// The items its pets carry are saved under the character, at the pet
// locations, so they go with its own items. Pets and augmentations of the
// character's items are deleted before those items, while the rows naming
// them still stand. It reports whether a character row was deleted.
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
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_memo WHERE charId = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d memos: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_quests WHERE charId = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d quests: %w", objectID, err)
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
	if _, err := tx.ExecContext(ctx, "DELETE FROM heroes WHERE char_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d hero record: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM olympiad_nobles WHERE char_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d olympiad record: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM heroes WHERE char_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d hero record: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_raid_points WHERE char_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d raid points: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM seven_signs WHERE char_obj_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d seven signs: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM bookmarks WHERE obj_Id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d bookmarks: %w", objectID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM buffer_schemes WHERE object_id = ?", objectID); err != nil {
		return false, fmt.Errorf("purge character %d buffer schemes: %w", objectID, err)
	}
	// Either spouse's deletion ends the marriage (#4676).
	if _, err := tx.ExecContext(ctx, "DELETE FROM mods_wedding WHERE requesterId = ? OR partnerId = ?", objectID, objectID); err != nil {
		return false, fmt.Errorf("purge character %d marriage: %w", objectID, err)
	}
	// The id is retired, never handed out again, so no row the deletion
	// missed can attach to a later object (#4676).
	if n > 0 {
		if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO retired_object_ids (object_id) VALUES (?)", objectID); err != nil {
			return false, fmt.Errorf("purge character %d retire id: %w", objectID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit purge character %d: %w", objectID, err)
	}
	committed = true
	return n > 0, nil
}
