package sql

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
)

var _ clanhall.SiegeStore = (*ClanHallStore)(nil)

// UpdateSiegableHall writes a siegable hall's owner and next siege date,
// kept in endDate.
func (s *ClanHallStore) UpdateSiegableHall(ctx context.Context, hallID, ownerID int32, nextSiege int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE clanhall SET ownerId = ?, endDate = ? WHERE id = ?", ownerID, nextSiege, hallID); err != nil {
		return fmt.Errorf("store siegable hall %d: %w", hallID, err)
	}
	return nil
}

// LoadSiegeAttackers returns every clanhall_siege_attackers row.
func (s *ClanHallStore) LoadSiegeAttackers(ctx context.Context) ([]clanhall.AttackerRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT clanhall_id, attacker_id FROM clanhall_siege_attackers")
	if err != nil {
		return nil, fmt.Errorf("load hall siege attackers: %w", err)
	}
	var out []clanhall.AttackerRow
	for rows.Next() {
		var r clanhall.AttackerRow
		if err := rows.Scan(&r.HallID, &r.ClanID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan hall siege attacker: %w", err)
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load hall siege attackers: %w", err)
	}
	return out, nil
}

// SaveSiegeAttackers replaces hall hallID's attackers with clanIDs, in one
// transaction.
func (s *ClanHallStore) SaveSiegeAttackers(ctx context.Context, hallID int32, clanIDs []int32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store hall %d siege attackers: begin: %w", hallID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM clanhall_siege_attackers WHERE clanhall_id = ?", hallID); err != nil {
		return fmt.Errorf("delete hall %d siege attackers: %w", hallID, err)
	}
	for _, clanID := range clanIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO clanhall_siege_attackers VALUES (?, ?)", hallID, clanID); err != nil {
			return fmt.Errorf("insert hall %d siege attacker %d: %w", hallID, clanID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store hall %d siege attackers: commit: %w", hallID, err)
	}
	return nil
}

// LoadRainbowDecrees returns every rainbowsprings_attacker_list row.
func (s *ClanHallStore) LoadRainbowDecrees(ctx context.Context) ([]clanhall.DecreeRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT clanId, war_decrees_count FROM rainbowsprings_attacker_list")
	if err != nil {
		return nil, fmt.Errorf("load rainbow springs decrees: %w", err)
	}
	var out []clanhall.DecreeRow
	for rows.Next() {
		var r clanhall.DecreeRow
		if err := rows.Scan(&r.ClanID, &r.Count); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan rainbow springs decree: %w", err)
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load rainbow springs decrees: %w", err)
	}
	return out, nil
}

// AddRainbowDecree inserts r.
func (s *ClanHallStore) AddRainbowDecree(ctx context.Context, r clanhall.DecreeRow) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO rainbowsprings_attacker_list VALUES (?, ?)", r.ClanID, r.Count); err != nil {
		return fmt.Errorf("store rainbow springs decree of clan %d: %w", r.ClanID, err)
	}
	return nil
}

// DeleteRainbowDecree deletes clan clanID's decrees.
func (s *ClanHallStore) DeleteRainbowDecree(ctx context.Context, clanID int32) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM rainbowsprings_attacker_list WHERE clanId = ?", clanID); err != nil {
		return fmt.Errorf("delete rainbow springs decree of clan %d: %w", clanID, err)
	}
	return nil
}

// DeleteRainbowDecrees deletes every clan's decrees.
func (s *ClanHallStore) DeleteRainbowDecrees(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM rainbowsprings_attacker_list"); err != nil {
		return fmt.Errorf("delete rainbow springs decrees: %w", err)
	}
	return nil
}

// LoadFlagWar returns the flag war tables: the clans by hall and flag, the
// members and the owner NPCs in their stored order.
func (s *ClanHallStore) LoadFlagWar(ctx context.Context) (clanhall.FlagWarRows, error) {
	var out clanhall.FlagWarRows
	err := s.scanRows(ctx, "SELECT clanhall_id, flag, npc, clan_id FROM clanhall_flagwar_attackers ORDER BY clanhall_id, flag", func(scan func(...any) error) error {
		var r clanhall.FlagWarClanRow
		if err := scan(&r.HallID, &r.Flag, &r.NPC, &r.ClanID); err != nil {
			return err
		}
		out.Clans = append(out.Clans, r)
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("load flag war clans: %w", err)
	}
	err = s.scanRows(ctx, "SELECT clanhall_id, clan_id, object_id FROM clanhall_flagwar_members", func(scan func(...any) error) error {
		var r clanhall.FlagWarMemberRow
		if err := scan(&r.HallID, &r.ClanID, &r.ObjectID); err != nil {
			return err
		}
		out.Members = append(out.Members, r)
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("load flag war members: %w", err)
	}
	err = s.scanRows(ctx, "SELECT clanhall_id, npc_id, clan_id FROM clanhall_flagwar_owner_npcs", func(scan func(...any) error) error {
		var r clanhall.FlagWarOwnerNPCRow
		if err := scan(&r.HallID, &r.NPCID, &r.ClanID); err != nil {
			return err
		}
		out.OwnerNPCs = append(out.OwnerNPCs, r)
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("load flag war owner npcs: %w", err)
	}
	return out, nil
}

// scanRows runs query and hands each row's Scan to row.
func (s *ClanHallStore) scanRows(ctx context.Context, query string, row func(scan func(...any) error) error) error {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	for rows.Next() {
		if err := row(rows.Scan); err != nil {
			_ = rows.Close()
			return err
		}
	}
	return closeRows(rows)
}

// InsertFlagWarClan inserts r, with no NPC picked.
func (s *ClanHallStore) InsertFlagWarClan(ctx context.Context, r clanhall.FlagWarClanRow) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO clanhall_flagwar_attackers VALUES (?, ?, ?, ?)", r.HallID, r.Flag, r.NPC, r.ClanID); err != nil {
		return fmt.Errorf("store flag war clan %d: %w", r.ClanID, err)
	}
	return nil
}

// UpdateFlagWarNPC sets npcID on every flag war clan row of clanID.
func (s *ClanHallStore) UpdateFlagWarNPC(ctx context.Context, clanID, npcID int32) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE clanhall_flagwar_attackers SET npc = ? WHERE clan_id = ?", npcID, clanID); err != nil {
		return fmt.Errorf("store flag war npc of clan %d: %w", clanID, err)
	}
	return nil
}

// InsertFlagWarMember inserts r.
func (s *ClanHallStore) InsertFlagWarMember(ctx context.Context, r clanhall.FlagWarMemberRow) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO clanhall_flagwar_members VALUES (?, ?, ?)", r.HallID, r.ClanID, r.ObjectID); err != nil {
		return fmt.Errorf("store flag war member %d of clan %d: %w", r.ObjectID, r.ClanID, err)
	}
	return nil
}

// InsertFlagWarOwnerNPC inserts r; a hall keeps one row, so a second one
// for the same hall fails.
func (s *ClanHallStore) InsertFlagWarOwnerNPC(ctx context.Context, r clanhall.FlagWarOwnerNPCRow) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO clanhall_flagwar_owner_npcs VALUES (?, ?, ?)", r.HallID, r.NPCID, r.ClanID); err != nil {
		return fmt.Errorf("store flag war owner npc of hall %d: %w", r.HallID, err)
	}
	return nil
}

// DeleteFlagWarOwnerNPCs deletes the owner NPC rows of clanID.
func (s *ClanHallStore) DeleteFlagWarOwnerNPCs(ctx context.Context, clanID int32) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM clanhall_flagwar_owner_npcs WHERE clan_id = ?", clanID); err != nil {
		return fmt.Errorf("delete flag war owner npcs of clan %d: %w", clanID, err)
	}
	return nil
}

// ClearFlagWar deletes hall hallID's flag war clan and member rows.
func (s *ClanHallStore) ClearFlagWar(ctx context.Context, hallID int32) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM clanhall_flagwar_attackers WHERE clanhall_id = ?", hallID); err != nil {
		return fmt.Errorf("delete flag war clans of hall %d: %w", hallID, err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM clanhall_flagwar_members WHERE clanhall_id = ?", hallID); err != nil {
		return fmt.Errorf("delete flag war members of hall %d: %w", hallID, err)
	}
	return nil
}
