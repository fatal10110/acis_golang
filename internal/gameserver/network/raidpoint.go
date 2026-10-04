package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
)

// RecordRaidKill implements manager.RaidKillRecorder: every member of the
// killer's party, wherever it stands, or the partyless killer alone, earns
// its own roll of the boss's raid points, and each noble among them gets
// the kill in its hero diary.
func (l *GameClientLink) RecordRaidKill(killer *player.Character, bossID int32, bossLevel int) {
	if l == nil {
		return
	}
	ids := []int32{killer.ObjectID()}
	var nobles []int32
	if killer.IsNoble() {
		nobles = append(nobles, killer.ObjectID())
	}
	if l.parties != nil {
		if view, ok := l.parties.View(killer.ObjectID()); ok {
			ids, nobles = ids[:0], nobles[:0]
			for _, m := range view.Members {
				ids = append(ids, m.ObjectID())
				if m.IsNoble() {
					nobles = append(nobles, m.ObjectID())
				}
			}
		}
	}
	if l.raidPoints != nil {
		l.raidPoints.CreditKill(ids, bossID, bossLevel)
	}
	if l.heroes != nil {
		for _, id := range nobles {
			l.heroes.AddDiaryEntry(id, hero.DiaryRaidKilled, int(bossID))
		}
	}
}

// sendBossRecord answers RequestGetBossRecord with the player's rank,
// total and points per boss.
func (l *GameClientLink) sendBossRecord(live *livePlayer) {
	var rec raidpoint.Record
	if l.raidPoints != nil {
		rec = l.raidPoints.Record(live.ObjectID())
	}
	entries := make([]serverpackets.BossRecordEntry, len(rec.Entries))
	for i, e := range rec.Entries {
		entries[i] = serverpackets.BossRecordEntry{BossID: e.BossID, Points: e.Points}
	}
	live.SendFrame(serverpackets.FrameExGetBossRecord(rec.Rank, rec.Total, entries, rec.Found))
}
