package sql

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// DeleteExpiredWars drops the war penalties that ran out by nowMs, as the
// registry restores.
func (s *ClanStore) DeleteExpiredWars(ctx context.Context, nowMs int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM clan_wars WHERE expiry_time > 0 AND expiry_time <= ?`, nowMs); err != nil {
		return fmt.Errorf("delete expired clan wars: %w", err)
	}
	return nil
}

// loadSubunitsAndWars reads every clan_subpledges and clan_wars row into
// snap.
func (s *ClanStore) loadSubunitsAndWars(ctx context.Context, snap *clan.Snapshot) error {
	rows, err := s.db.QueryContext(ctx, `SELECT clan_id, sub_pledge_id, COALESCE(name,''), leader_id FROM clan_subpledges`)
	if err != nil {
		return fmt.Errorf("load clan sub-units: %w", err)
	}
	for rows.Next() {
		var r clan.SubunitRow
		if err := rows.Scan(&r.ClanID, &r.ID, &r.Name, &r.LeaderID); err != nil {
			rows.Close()
			return fmt.Errorf("load clan sub-units: %w", err)
		}
		snap.Subunits = append(snap.Subunits, r)
	}
	if err := closeRows(rows); err != nil {
		return fmt.Errorf("load clan sub-units: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `SELECT clan1, clan2, expiry_time FROM clan_wars`)
	if err != nil {
		return fmt.Errorf("load clan wars: %w", err)
	}
	for rows.Next() {
		var r clan.WarRow
		if err := rows.Scan(&r.ClanID, &r.TargetID, &r.Expiry); err != nil {
			rows.Close()
			return fmt.Errorf("load clan wars: %w", err)
		}
		snap.Wars = append(snap.Wars, r)
	}
	if err := closeRows(rows); err != nil {
		return fmt.Errorf("load clan wars: %w", err)
	}
	return nil
}

// SetPledgeType stores a member's sub-unit.
func (s *ClanStore) SetPledgeType(ctx context.Context, objectID int32, pledgeType int) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE characters SET subpledge=? WHERE obj_Id=?`, pledgeType, objectID); err != nil {
		return fmt.Errorf("set sub-unit of %d: %w", objectID, err)
	}
	return nil
}

// SetMentor stores a member's apprentice and sponsor.
func (s *ClanStore) SetMentor(ctx context.Context, objectID, apprentice, sponsor int32) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE characters SET apprentice=?,sponsor=? WHERE obj_Id=?`, apprentice, sponsor, objectID); err != nil {
		return fmt.Errorf("set apprentice and sponsor of %d: %w", objectID, err)
	}
	return nil
}

// InsertSubunit stores a newly founded sub-unit.
func (s *ClanStore) InsertSubunit(ctx context.Context, r clan.SubunitRow) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO clan_subpledges (clan_id,sub_pledge_id,name,leader_id) VALUES (?,?,?,?)`,
		r.ClanID, r.ID, r.Name, r.LeaderID)
	if err != nil {
		return fmt.Errorf("insert clan %d sub-unit %d: %w", r.ClanID, r.ID, err)
	}
	return nil
}

// UpdateSubunit stores a sub-unit's captain and name.
func (s *ClanStore) UpdateSubunit(ctx context.Context, r clan.SubunitRow) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clan_subpledges SET leader_id=?, name=? WHERE clan_id=? AND sub_pledge_id=?`,
		r.LeaderID, r.Name, r.ClanID, r.ID)
	if err != nil {
		return fmt.Errorf("update clan %d sub-unit %d: %w", r.ClanID, r.ID, err)
	}
	return nil
}

// InsertWar stores clanID's war on targetID.
func (s *ClanStore) InsertWar(ctx context.Context, clanID, targetID int32) error {
	if _, err := s.db.ExecContext(ctx, `REPLACE INTO clan_wars (clan1, clan2) VALUES(?,?)`, clanID, targetID); err != nil {
		return fmt.Errorf("store clan %d war on %d: %w", clanID, targetID, err)
	}
	return nil
}

// EndWar keeps clanID's ended war on targetID as a penalty until expiry,
// or deletes it when expiry is 0.
func (s *ClanStore) EndWar(ctx context.Context, clanID, targetID int32, expiry int64) error {
	var err error
	if expiry > 0 {
		_, err = s.db.ExecContext(ctx, `UPDATE clan_wars SET expiry_time=? WHERE clan1=? AND clan2=?`, expiry, clanID, targetID)
	} else {
		_, err = s.db.ExecContext(ctx, `DELETE FROM clan_wars WHERE clan1=? AND clan2=?`, clanID, targetID)
	}
	if err != nil {
		return fmt.Errorf("end clan %d war on %d: %w", clanID, targetID, err)
	}
	return nil
}
