package idfactory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// FirstObjectID and LastObjectID bound the range of ids Allocator hands out.
const (
	FirstObjectID = 0x10000000
	LastObjectID  = 0x7FFFFFFF
)

// ErrIDSpaceExhausted is returned by NextID when every id in range is in use.
var ErrIDSpaceExhausted = errors.New("idfactory: id space exhausted")

// usedObjectIDQueries lists, for every table that persists an object id
// handed out by this allocator, the query that reads those ids back.
var usedObjectIDQueries = [...]string{
	"SELECT obj_Id FROM characters",
	"SELECT object_id FROM items",
	"SELECT clan_id FROM clan_data",
	"SELECT object_id FROM items_on_ground",
	"SELECT id FROM mods_wedding",
	"SELECT oid FROM petition",
}

// orphanCleanupStatements deletes rows whose owner was never removed with
// them (a crash mid-deletion, or a deletion that predated a table), so a
// reallocated object id cannot resurrect stale data onto unrelated rows.
var orphanCleanupStatements = [...]string{
	// Character related
	"DELETE FROM augmentations WHERE augmentations.item_oid NOT IN (SELECT object_id FROM items)",
	"DELETE FROM character_hennas WHERE character_hennas.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_macroses WHERE character_macroses.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_memo WHERE character_memo.charId NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_quests WHERE character_quests.charId NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_raid_points WHERE character_raid_points.char_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_recipebook WHERE character_recipebook.charId NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_relations WHERE character_relations.char_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_relations WHERE character_relations.friend_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_shortcuts WHERE character_shortcuts.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_skills WHERE character_skills.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_skills_save WHERE character_skills_save.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM character_subclasses WHERE character_subclasses.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM cursed_weapons WHERE cursed_weapons.playerId NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM pets WHERE pets.item_obj_id NOT IN (SELECT object_id FROM items UNION SELECT object_id FROM items_on_ground)",
	"DELETE FROM seven_signs WHERE seven_signs.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
	// Olympiads & heroes
	"DELETE FROM heroes WHERE heroes.char_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM olympiad_nobles WHERE olympiad_nobles.char_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM olympiad_nobles_eom WHERE olympiad_nobles_eom.char_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM olympiad_fights WHERE olympiad_fights.charOneId NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM olympiad_fights WHERE olympiad_fights.charTwoId NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM heroes_diary WHERE heroes_diary.char_id NOT IN (SELECT obj_Id FROM characters)",
	// Auction
	"DELETE FROM auctions WHERE clanhall_id IN (SELECT id FROM clanhall WHERE ownerId <> 0 AND sellerClanName='')",
	// Clan related
	"DELETE FROM clan_data WHERE clan_data.leader_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM auctions WHERE auctions.clan_oid NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM clanhall_functions WHERE clanhall_functions.hall_id NOT IN (SELECT id FROM clanhall WHERE ownerId <> 0)",
	"DELETE FROM clan_privs WHERE clan_privs.clan_id NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM clan_skills WHERE clan_skills.clan_id NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM clan_subpledges WHERE clan_subpledges.clan_id NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM clan_wars WHERE clan_wars.clan1 NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM clan_wars WHERE clan_wars.clan2 NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM siege_clans WHERE siege_clans.clan_id NOT IN (SELECT clan_id FROM clan_data)",
	// Items
	"DELETE FROM items WHERE items.owner_id NOT IN (SELECT obj_Id FROM characters) AND items.owner_id NOT IN (SELECT clan_id FROM clan_data)",
	// Forum related
	"DELETE FROM bbs_forum WHERE bbs_forum.type='CLAN' AND bbs_forum.owner_id NOT IN (SELECT clan_id FROM clan_data)",
	"DELETE FROM bbs_forum WHERE bbs_forum.type='MEMO' AND bbs_forum.owner_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM bbs_topic WHERE bbs_topic.forum_id NOT IN (SELECT id FROM bbs_forum)",
	"DELETE FROM bbs_post WHERE bbs_post.forum_id NOT IN (SELECT id FROM bbs_forum)",
	"DELETE FROM bbs_post WHERE bbs_post.topic_id NOT IN (SELECT id FROM bbs_topic)",
	"DELETE FROM bbs_favorite WHERE bbs_favorite.player_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM bbs_mail WHERE bbs_mail.receiver_id NOT IN (SELECT obj_Id FROM characters)",
	// Petition
	"DELETE FROM petition WHERE petition.petitioner_oid NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM petition_message WHERE petition_message.petition_oid NOT IN (SELECT oid FROM petition)",
}

// orphanRepairStatements resets references left dangling by the same
// crashes, instead of deleting whole rows.
var orphanRepairStatements = [...]string{
	"UPDATE clan_data SET auction_bid_at = 0 WHERE auction_bid_at NOT IN (SELECT clanhall_id FROM auctions)",
	"UPDATE clan_data SET new_leader_id = 0 WHERE new_leader_id NOT IN (SELECT obj_Id FROM characters)",
	"UPDATE clan_subpledges SET leader_id=0 WHERE clan_subpledges.leader_id NOT IN (SELECT obj_Id FROM characters) AND leader_id > 0",
	"UPDATE castle SET currentTaxPercent=0, nextTaxPercent=0 WHERE castle.id NOT IN (SELECT hasCastle FROM clan_data)",
	"UPDATE characters SET clanid=0 WHERE characters.clanid NOT IN (SELECT clan_id FROM clan_data)",
	"UPDATE clanhall SET ownerId=0, paidUntil=0, paid=0 WHERE clanhall.ownerId NOT IN (SELECT clan_id FROM clan_data)",
}

// Allocator hands out unique object ids, reusing ids released back to it.
//
// An id released mid-session only becomes available again once allocation
// naturally reaches it (ids are handed out in increasing order and the
// search cursor never moves backward) or the Allocator is rebuilt via New,
// which reclaims every id no longer present in the database. This trades
// perfect same-session reuse for O(1) amortized allocation.
//
// mu guards used and next.
type Allocator struct {
	mu   sync.Mutex
	used map[int32]struct{}
	next int32

	first, last int32 // id range; always FirstObjectID/LastObjectID outside tests
	log         zerolog.Logger
}

// New repairs the database's persistent-object integrity and then scans db
// for object ids already in use, returning an Allocator seeded with them,
// ready to hand out ids that don't collide with existing rows. The repair
// pass (reset stale online flags, purge orphan rows, drop expired skill
// timestamps) runs before the scan so the scan sees post-cleanup state; a
// failed repair statement is logged and skipped, since booting without one
// cleanup beats not booting at all, while an id-scan error still fails
// loudly rather than booting with a partial id set.
func New(ctx context.Context, db *sql.DB, log zerolog.Logger) (*Allocator, error) {

	a := &Allocator{
		used:  make(map[int32]struct{}),
		first: FirstObjectID,
		last:  LastObjectID,
		log:   log,
	}

	a.cleanup(ctx, db)
	for _, query := range usedObjectIDQueries {
		if err := a.loadUsedIDs(ctx, db, query); err != nil {
			return nil, fmt.Errorf("idfactory: %w", err)
		}
	}

	a.next = a.first
	log.Info().Int("used_object_ids", len(a.used)).Msg("idfactory: initialized")
	return a, nil
}

// cleanup runs the boot-time database repair pass. Each statement is
// independent: one failing (a table that doesn't exist yet, a transient
// lock) is logged at warn and the rest still run.
func (a *Allocator) cleanup(ctx context.Context, db *sql.DB) {
	cleanCount := int64(0)

	res, err := db.ExecContext(ctx, "UPDATE characters SET online = 0")
	if err != nil {
		a.log.Warn().Err(err).Msg("idfactory: couldn't set characters offline")
	} else {
		logRowsAffected(res, &cleanCount)
	}
	a.log.Info().Msg("idfactory: updated characters online status")

	for _, stmt := range orphanCleanupStatements {
		res, err := db.ExecContext(ctx, stmt)
		if err != nil {
			a.log.Warn().Err(err).Str("statement", stmt).Msg("idfactory: couldn't cleanup database row orphans")
			continue
		}
		logRowsAffected(res, &cleanCount)
	}
	for _, stmt := range orphanRepairStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			a.log.Warn().Err(err).Str("statement", stmt).Msg("idfactory: couldn't repair database orphans")
		}
	}
	a.log.Info().Int64("cleaned", cleanCount).Msg("idfactory: cleaned elements from database")

	res, err = db.ExecContext(ctx, "DELETE FROM character_skills_save WHERE restore_type = 1 AND systime <= ?", time.Now().UnixMilli())
	if err != nil {
		a.log.Warn().Err(err).Msg("idfactory: couldn't cleanup expired timestamps")
	} else {
		expired, _ := res.RowsAffected()
		a.log.Info().Int64("cleaned", expired).Msg("idfactory: cleaned expired timestamps from database")
	}
}

func logRowsAffected(res sql.Result, total *int64) {
	n, err := res.RowsAffected()
	if err == nil {
		*total += n
	}
}

func (a *Allocator) loadUsedIDs(ctx context.Context, db *sql.DB, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query used object ids (%s): %w", query, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan used object id (%s): %w", query, err)
		}
		if id < int64(a.first) {
			a.log.Warn().Int64("object_id", id).Int32("minimum_id", a.first).Msg("idfactory: skipping object id below minimum")
			continue
		}
		a.used[int32(id)] = struct{}{}
	}
	return rows.Err()
}

// NextID returns the next available object id and marks it used, or
// ErrIDSpaceExhausted if every id in range is already in use.
func (a *Allocator) NextID() (int32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	id, err := a.nextFreeFrom(a.next)
	if err != nil {
		return 0, err
	}
	a.used[id] = struct{}{}
	a.next = id + 1
	return id, nil
}

// ReleaseID returns id to the pool so a later NextID call can hand it out
// again. Ids below FirstObjectID never came from this allocator; releasing
// one is logged and ignored rather than corrupting allocator state.
func (a *Allocator) ReleaseID(id int32) {
	if id < a.first {
		a.log.Warn().Int32("object_id", id).Int32("minimum_id", a.first).Msg("idfactory: ignored invalid object id release")
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, id)
}

// nextFreeFrom returns the first id >= from that isn't marked used, or
// ErrIDSpaceExhausted if none remains up to last. Callers hold mu.
func (a *Allocator) nextFreeFrom(from int32) (int32, error) {
	for id := from; id <= a.last; id++ {
		if _, used := a.used[id]; !used {
			return id, nil
		}
	}
	return 0, fmt.Errorf("%w: range [%d, %d]", ErrIDSpaceExhausted, a.first, a.last)
}
