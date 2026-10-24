package clanhall

import (
	"context"
	"slices"
)

// The flag war halls (the Bandit Stronghold and the Wild Beast Reserve)
// keep their registrations in tables of their own: per clan its flag and
// the NPC it picked, the members it registered, and the NPC the hall's
// owner fights with. Their script loads them as it binds the hall's siege
// and stores each change as it makes it.

// FlagWarClanRow is one clanhall_flagwar_attackers row.
type FlagWarClanRow struct {
	HallID, Flag, NPC, ClanID int32
}

// FlagWarMemberRow is one clanhall_flagwar_members row.
type FlagWarMemberRow struct {
	HallID, ClanID, ObjectID int32
}

// FlagWarOwnerNPCRow is one clanhall_flagwar_owner_npcs row.
type FlagWarOwnerNPCRow struct {
	HallID, NPCID, ClanID int32
}

// FlagWarRows are the stored flag war tables.
type FlagWarRows struct {
	Clans     []FlagWarClanRow
	Members   []FlagWarMemberRow
	OwnerNPCs []FlagWarOwnerNPCRow
}

// FlagWarClan is a clan registered on a flag war, as stored: its flag, the
// NPC it picked (0 for none) and the object ids of the members it
// registered.
type FlagWarClan struct {
	ClanID, Flag, NPC int32
	Members           []int32
}

// FlagWarClans returns the clans stored as registered on the hall's flag
// war, in the order the store lists them, skipping clans that no longer
// exist. Each clan's members are every member row stored for that clan,
// whichever hall it names.
func (s *HallSiege) FlagWarClans() []FlagWarClan {
	hs := s.hs
	hs.siegeMu.Lock()
	rows := hs.flagWar
	hs.siegeMu.Unlock()
	var out []FlagWarClan
	for _, r := range rows.Clans {
		if r.HallID != s.h.id() {
			continue
		}
		if _, ok := hs.clans.Get(r.ClanID); !ok {
			continue
		}
		c := FlagWarClan{ClanID: r.ClanID, Flag: r.Flag, NPC: r.NPC}
		for _, m := range rows.Members {
			if m.ClanID == r.ClanID {
				c.Members = append(c.Members, m.ObjectID)
			}
		}
		out = append(out, c)
	}
	return out
}

// FlagWarOwnerNPC returns the first stored owner NPC row of the hall: the
// clan it was saved for and its NPC; ok is false when there is none.
func (s *HallSiege) FlagWarOwnerNPC() (clanID, npcID int32, ok bool) {
	hs := s.hs
	hs.siegeMu.Lock()
	rows := hs.flagWar.OwnerNPCs
	hs.siegeMu.Unlock()
	i := slices.IndexFunc(rows, func(r FlagWarOwnerNPCRow) bool { return r.HallID == s.h.id() })
	if i < 0 {
		return 0, 0, false
	}
	return rows[i].ClanID, rows[i].NPCID, true
}

// StoreFlagWarClan stores clanID as registered on the hall's flag war
// under flag, with no NPC picked.
func (s *HallSiege) StoreFlagWarClan(clanID, flag int32) {
	row := FlagWarClanRow{HallID: s.h.id(), Flag: flag, ClanID: clanID}
	s.hs.writeSiege("store flag war clan", row.HallID, func(ctx context.Context, st SiegeStore) error {
		return st.InsertFlagWarClan(ctx, row)
	})
}

// StoreFlagWarNPC stores npcID as the NPC clanID picked, on every hall's
// flag war the clan is stored for.
func (s *HallSiege) StoreFlagWarNPC(clanID, npcID int32) {
	s.hs.writeSiege("store flag war npc", s.h.id(), func(ctx context.Context, st SiegeStore) error {
		return st.UpdateFlagWarNPC(ctx, clanID, npcID)
	})
}

// StoreFlagWarMember stores the player objectID as registered for clanID
// on the hall's flag war.
func (s *HallSiege) StoreFlagWarMember(clanID, objectID int32) {
	row := FlagWarMemberRow{HallID: s.h.id(), ClanID: clanID, ObjectID: objectID}
	s.hs.writeSiege("store flag war member", row.HallID, func(ctx context.Context, st SiegeStore) error {
		return st.InsertFlagWarMember(ctx, row)
	})
}

// StoreFlagWarOwnerNPC stores npcID as the NPC the hall's owner clanID
// fights with.
func (s *HallSiege) StoreFlagWarOwnerNPC(clanID, npcID int32) {
	row := FlagWarOwnerNPCRow{HallID: s.h.id(), NPCID: npcID, ClanID: clanID}
	s.hs.writeSiege("store flag war owner npc", row.HallID, func(ctx context.Context, st SiegeStore) error {
		return st.InsertFlagWarOwnerNPC(ctx, row)
	})
}

// DeleteFlagWarOwnerNPC deletes the stored owner NPC rows whose clan id is
// the hall's id.
func (s *HallSiege) DeleteFlagWarOwnerNPC() {
	id := s.h.id()
	s.hs.writeSiege("delete flag war owner npc", id, func(ctx context.Context, st SiegeStore) error {
		return st.DeleteFlagWarOwnerNPCs(ctx, id)
	})
}

// ClearFlagWar deletes the hall's stored flag war clans and members.
func (s *HallSiege) ClearFlagWar() {
	id := s.h.id()
	s.hs.writeSiege("clear flag war", id, func(ctx context.Context, st SiegeStore) error {
		return st.ClearFlagWar(ctx, id)
	})
}
