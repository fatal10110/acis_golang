package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
)

// ClanStore reads and writes clan_data, clan_privs, clan_skills and the
// clan columns of characters.
type ClanStore struct {
	db *sql.DB
}

// NewClanStore returns a ClanStore backed by db.
func NewClanStore(db *sql.DB) *ClanStore {
	return &ClanStore{db: db}
}

// Load reads every clan row, every clan member's characters row, every
// rank privilege row, every sub-unit and every war.
func (s *ClanStore) Load(ctx context.Context) (clan.Snapshot, error) {
	var snap clan.Snapshot
	rows, err := s.db.QueryContext(ctx, `SELECT clan_id, COALESCE(clan_name,''), clan_level, reputation_score, hasCastle,
		ally_id, COALESCE(ally_name,''), leader_id, new_leader_id, crest_id, crest_large_id, ally_crest_id,
		ally_penalty_expiry_time, ally_penalty_type, char_penalty_expiry_time, dissolving_expiry_time,
		enabled, COALESCE(notice,''), COALESCE(introduction,'')
		FROM clan_data`)
	if err != nil {
		return snap, fmt.Errorf("load clans: %w", err)
	}
	for rows.Next() {
		var r clan.Row
		if err := rows.Scan(&r.ID, &r.Name, &r.Level, &r.Reputation, &r.CastleID,
			&r.AllyID, &r.AllyName, &r.LeaderID, &r.NewLeaderID, &r.CrestID, &r.CrestLargeID, &r.AllyCrestID,
			&r.AllyPenaltyExpiry, &r.AllyPenaltyType, &r.CharPenaltyExpiry, &r.DissolvingExpiry,
			&r.NoticeEnabled, &r.Notice, &r.Introduction); err != nil {
			rows.Close()
			return snap, fmt.Errorf("load clans: %w", err)
		}
		snap.Clans = append(snap.Clans, r)
	}
	if err := closeRows(rows); err != nil {
		return snap, fmt.Errorf("load clans: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `SELECT clanid, char_name, COALESCE(level,0), COALESCE(classid,0), obj_Id, COALESCE(title,''),
		COALESCE(power_grade,0), subpledge, apprentice, sponsor, COALESCE(sex,0), COALESCE(race,0), lvl_joined_academy
		FROM characters WHERE clanid > 0 ORDER BY obj_Id`)
	if err != nil {
		return snap, fmt.Errorf("load clan members: %w", err)
	}
	for rows.Next() {
		var m clan.MemberRow
		if err := rows.Scan(&m.ClanID, &m.Name, &m.Level, &m.ClassID, &m.ObjectID, &m.Title,
			&m.PowerGrade, &m.PledgeType, &m.Apprentice, &m.Sponsor, &m.Sex, &m.Race, &m.LvlJoinedAcademy); err != nil {
			rows.Close()
			return snap, fmt.Errorf("load clan members: %w", err)
		}
		snap.Members = append(snap.Members, m)
	}
	if err := closeRows(rows); err != nil {
		return snap, fmt.Errorf("load clan members: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `SELECT clan_id, ranking, privs FROM clan_privs`)
	if err != nil {
		return snap, fmt.Errorf("load clan privileges: %w", err)
	}
	for rows.Next() {
		var p clan.PrivilegeRow
		if err := rows.Scan(&p.ClanID, &p.Rank, &p.Privs); err != nil {
			rows.Close()
			return snap, fmt.Errorf("load clan privileges: %w", err)
		}
		snap.Privileges = append(snap.Privileges, p)
	}
	if err := closeRows(rows); err != nil {
		return snap, fmt.Errorf("load clan privileges: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `SELECT clan_id, skill_id, skill_level FROM clan_skills`)
	if err != nil {
		return snap, fmt.Errorf("load clan skills: %w", err)
	}
	for rows.Next() {
		var r clan.SkillRow
		if err := rows.Scan(&r.ClanID, &r.ID, &r.Level); err != nil {
			rows.Close()
			return snap, fmt.Errorf("load clan skills: %w", err)
		}
		snap.Skills = append(snap.Skills, r)
	}
	if err := closeRows(rows); err != nil {
		return snap, fmt.Errorf("load clan skills: %w", err)
	}
	if err := s.loadSubunitsAndWars(ctx, &snap); err != nil {
		return snap, err
	}
	return snap, nil
}

func closeRows(rows *sql.Rows) error {
	err := rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return err
}

// InsertClan stores a newly founded clan.
func (s *ClanStore) InsertClan(ctx context.Context, r clan.Row) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO clan_data (clan_id,clan_name,clan_level,hasCastle,ally_id,ally_name,leader_id,new_leader_id,crest_id,crest_large_id,ally_crest_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.Level, r.CastleID, r.AllyID, nullString(r.AllyName), r.LeaderID, r.NewLeaderID, r.CrestID, r.CrestLargeID, r.AllyCrestID)
	if err != nil {
		return fmt.Errorf("insert clan %d: %w", r.ID, err)
	}
	return nil
}

// UpdateClan stores a clan's leader, nominated leader, alliance, reputation
// and penalty columns.
func (s *ClanStore) UpdateClan(ctx context.Context, r clan.Row) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clan_data SET leader_id=?,new_leader_id=?,ally_id=?,ally_name=?,reputation_score=?,
		ally_penalty_expiry_time=?,ally_penalty_type=?,char_penalty_expiry_time=?,dissolving_expiry_time=? WHERE clan_id=?`,
		r.LeaderID, r.NewLeaderID, r.AllyID, nullString(r.AllyName), r.Reputation,
		r.AllyPenaltyExpiry, r.AllyPenaltyType, r.CharPenaltyExpiry, r.DissolvingExpiry, r.ID)
	if err != nil {
		return fmt.Errorf("update clan %d: %w", r.ID, err)
	}
	return nil
}

// UpdateLevel stores a clan's level.
func (s *ClanStore) UpdateLevel(ctx context.Context, clanID int32, level int) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE clan_data SET clan_level = ? WHERE clan_id = ?`, level, clanID); err != nil {
		return fmt.Errorf("update clan %d level: %w", clanID, err)
	}
	return nil
}

// UpdateReputation stores a clan's reputation score.
func (s *ClanStore) UpdateReputation(ctx context.Context, clanID int32, score int) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE clan_data SET reputation_score=? WHERE clan_id=?`, score, clanID); err != nil {
		return fmt.Errorf("update clan %d reputation: %w", clanID, err)
	}
	return nil
}

// SetPrivileges stores one rank's privileges.
func (s *ClanStore) SetPrivileges(ctx context.Context, clanID int32, rank int, privs int32) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO clan_privs (clan_id,ranking,privs) VALUES (?,?,?) ON DUPLICATE KEY UPDATE privs=VALUES(privs)`,
		clanID, rank, privs)
	if err != nil {
		return fmt.Errorf("store clan %d rank %d privileges: %w", clanID, rank, err)
	}
	return nil
}

// SaveMembership stores a character's clan, title, rank, sub-unit, join
// penalty and academy join level as it joins or founds a clan.
func (s *ClanStore) SaveMembership(ctx context.Context, r clan.MembershipRow) error {
	_, err := s.db.ExecContext(ctx, `UPDATE characters SET clanid=?, title=?, power_grade=?, subpledge=?, clan_join_expiry_time=?, lvl_joined_academy=? WHERE obj_Id=?`,
		r.ClanID, r.Title, r.PowerGrade, r.PledgeType, r.JoinExpiry, r.LvlJoinedAcademy, r.ObjectID)
	if err != nil {
		return fmt.Errorf("save clan membership of %d: %w", r.ObjectID, err)
	}
	return nil
}

// RemoveMembership clears a character's clan columns as it leaves a clan.
func (s *ClanStore) RemoveMembership(ctx context.Context, r clan.RemovalRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remove clan member %d: %w", r.ObjectID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if r.Online {
		_, err = tx.ExecContext(ctx, `UPDATE characters SET clanid=0, title='', power_grade=0, clan_join_expiry_time=?, clan_create_expiry_time=?,
			subpledge=0, lvl_joined_academy=0, apprentice=0, sponsor=0 WHERE obj_Id=?`, r.JoinExpiry, r.CreateExpiry, r.ObjectID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE characters SET clanid=0, title='', clan_join_expiry_time=?, clan_create_expiry_time=?, wantspeace=0,
			subpledge=0, lvl_joined_academy=0, apprentice=0, sponsor=0 WHERE obj_Id=?`, r.JoinExpiry, r.CreateExpiry, r.ObjectID)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE characters SET apprentice=0 WHERE apprentice=?`, r.ObjectID)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE characters SET sponsor=0 WHERE sponsor=?`, r.ObjectID)
		}
	}
	if err != nil {
		return fmt.Errorf("remove clan member %d: %w", r.ObjectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("remove clan member %d: %w", r.ObjectID, err)
	}
	return nil
}

// SetPowerGrade stores a member's power grade.
func (s *ClanStore) SetPowerGrade(ctx context.Context, objectID int32, grade int) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE characters SET power_grade=? WHERE obj_Id=?`, grade, objectID); err != nil {
		return fmt.Errorf("set power grade of %d: %w", objectID, err)
	}
	return nil
}

// UpdateCrest stores one of a clan's own crest id columns.
func (s *ClanStore) UpdateCrest(ctx context.Context, clanID int32, typ datacache.CrestType, crestID int32) error {
	var query string
	switch typ {
	case datacache.PledgeCrest:
		query = `UPDATE clan_data SET crest_id = ? WHERE clan_id = ?`
	case datacache.LargePledgeCrest:
		query = `UPDATE clan_data SET crest_large_id = ? WHERE clan_id = ?`
	case datacache.AllyCrest:
		query = `UPDATE clan_data SET ally_crest_id = ? WHERE clan_id = ?`
	default:
		return fmt.Errorf("update clan %d crest: unknown crest type %d", clanID, typ)
	}
	if _, err := s.db.ExecContext(ctx, query, crestID, clanID); err != nil {
		return fmt.Errorf("update clan %d crest: %w", clanID, err)
	}
	return nil
}

// SaveSkill stores a clan skill at its level, replacing the level stored
// before.
func (s *ClanStore) SaveSkill(ctx context.Context, clanID int32, sk clan.Skill) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO clan_skills (clan_id,skill_id,skill_level) VALUES (?,?,?) ON DUPLICATE KEY UPDATE skill_level=VALUES(skill_level)`,
		clanID, sk.ID, sk.Level)
	if err != nil {
		return fmt.Errorf("store clan %d skill %d: %w", clanID, sk.ID, err)
	}
	return nil
}

// nullString stores "" as NULL, as an unset alliance name is.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
