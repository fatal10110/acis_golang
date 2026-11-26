package objectids

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
)

// usedObjectIDQueries lists, for every table that persists an object id
// handed out by the object id allocator, the query that reads those ids back.
var usedObjectIDQueries = [...]string{
	"SELECT obj_Id FROM characters",
	"SELECT object_id FROM items",
	"SELECT clan_id FROM clan_data",
	"SELECT object_id FROM items_on_ground",
	"SELECT id FROM mods_wedding",
	"SELECT oid FROM petition",
	// Deleted characters' ids are never handed out again (#4676).
	"SELECT object_id FROM retired_object_ids",
}

// RetiredIDsSchema creates the table that records every deleted
// character's object id, so no later object can take it over and inherit a
// row the deletion missed (#4676). The shipped schema has no such table,
// so NewAllocator creates it.
const RetiredIDsSchema = "CREATE TABLE IF NOT EXISTS `retired_object_ids` (\n" +
	"  `object_id` INT UNSIGNED NOT NULL,\n" +
	"  PRIMARY KEY (`object_id`)\n" +
	")"

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
	// A deleted character's bookmarks, schemes and marriage go with it
	// (#4676); a couple's own id is not a character, its spouses are.
	"DELETE FROM bookmarks WHERE bookmarks.obj_Id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM buffer_schemes WHERE buffer_schemes.object_id NOT IN (SELECT obj_Id FROM characters)",
	"DELETE FROM mods_wedding WHERE mods_wedding.requesterId NOT IN (SELECT obj_Id FROM characters) OR mods_wedding.partnerId NOT IN (SELECT obj_Id FROM characters)",
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
	// Items. A pet's items are saved under its owner, so they stay with the
	// character.
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

// NewAllocator repairs the database's persistent-object integrity and then
// scans db for object ids already in use, returning an allocator seeded with
// them. The repair pass (reset stale online flags, purge orphan rows, drop
// expired skill timestamps) runs before the scan so the scan sees
// post-cleanup state. The scan counts every retired id as in use, creating
// their table first; a failed repair statement is logged and skipped, since
// booting without one cleanup beats not booting at all, while an id-scan
// error still fails loudly rather than booting with a partial id set.
func NewAllocator(ctx context.Context, db *sql.DB, log zerolog.Logger) (*idfactory.Allocator, error) {
	repair(ctx, db, log)
	if _, err := db.ExecContext(ctx, RetiredIDsSchema); err != nil {
		return nil, fmt.Errorf("idfactory: create retired id table: %w", err)
	}
	ids, err := idfactory.New(log, func(mark func(int64)) error { return ScanUsedIDs(ctx, db, mark) })
	if err != nil {
		return nil, fmt.Errorf("idfactory: %w", err)
	}
	return ids, nil
}

// repair runs the boot-time database repair pass. Each statement is
// independent: one failing (a table that doesn't exist yet, a transient
// lock) is logged at warn and the rest still run. ctx expiring mid-pass is
// not one of those independent per-statement failures: every remaining
// ExecContext would fail the same way, and running them anyway would bury
// the boot-deadline cause under one warn line per statement plus whatever
// unrelated error the id scan surfaces next. So each stage checks ctx.Err()
// once before starting and stops the whole pass there instead.
func repair(ctx context.Context, db *sql.DB, log zerolog.Logger) {
	cleanCount := int64(0)

	res, err := db.ExecContext(ctx, "UPDATE characters SET online = 0")
	if err != nil {
		log.Warn().Err(err).Msg("idfactory: couldn't set characters offline")
	} else {
		logRowsAffected(res, &cleanCount)
	}
	log.Info().Msg("idfactory: updated characters online status")

	for _, stmt := range orphanCleanupStatements {
		if ctx.Err() != nil {
			log.Warn().Err(ctx.Err()).Msg("idfactory: boot deadline hit during orphan cleanup, skipping remaining statements")
			return
		}
		res, err := db.ExecContext(ctx, stmt)
		if err != nil {
			log.Warn().Err(err).Str("statement", stmt).Msg("idfactory: couldn't cleanup database row orphans")
			continue
		}
		logRowsAffected(res, &cleanCount)
	}
	for _, stmt := range orphanRepairStatements {
		if ctx.Err() != nil {
			log.Warn().Err(ctx.Err()).Msg("idfactory: boot deadline hit during orphan repair, skipping remaining statements")
			return
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			log.Warn().Err(err).Str("statement", stmt).Msg("idfactory: couldn't repair database orphans")
		}
	}
	log.Info().Int64("cleaned", cleanCount).Msg("idfactory: cleaned elements from database")

	if ctx.Err() != nil {
		log.Warn().Err(ctx.Err()).Msg("idfactory: boot deadline hit before expired-timestamp cleanup, skipping")
		return
	}
	res, err = db.ExecContext(ctx, "DELETE FROM character_skills_save WHERE restore_type = 1 AND systime <= ?", time.Now().UnixMilli())
	if err != nil {
		log.Warn().Err(err).Msg("idfactory: couldn't cleanup expired timestamps")
	} else {
		expired, _ := res.RowsAffected()
		log.Info().Int64("cleaned", expired).Msg("idfactory: cleaned expired timestamps from database")
	}
}

func logRowsAffected(res sql.Result, total *int64) {
	n, err := res.RowsAffected()
	if err == nil {
		*total += n
	}
}

// ScanUsedIDs calls fn with every object id stored in db by a table whose
// ids the object id allocator hands out, so another id source can avoid them
// too. An id
// stored in several tables is passed once per table.
func ScanUsedIDs(ctx context.Context, db *sql.DB, fn func(id int64)) error {
	for _, query := range usedObjectIDQueries {
		if err := scanIDs(ctx, db, query, fn); err != nil {
			return err
		}
	}
	return nil
}

func scanIDs(ctx context.Context, db *sql.DB, query string, fn func(id int64)) error {
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
		fn(id)
	}
	return rows.Err()
}
