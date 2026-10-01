package clan

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// seededClanID is the clan the seeded clan worlds start in.
const seededClanID = 268435456

// clanSeed is a stored clan the founder leads when the server boots.
type clanSeed struct {
	level, reputation int
	// fillers is how many offline members it has besides the founder.
	fillers int
	// dissolving is the epoch millisecond a pending dissolution ends.
	dissolving int64
}

// seedClan stores s with the founder as its leader before the clans are
// restored.
func seedClan(t *testing.T, s clanSeed) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		exec := func(query string, args ...any) {
			if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
				t.Fatalf("%s: %v", query, err)
			}
		}
		exec(`UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Founder'`, seededClanID)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, dissolving_expiry_time, leader_id)
			SELECT ?, 'Seeded', ?, ?, ?, obj_Id FROM characters WHERE char_name = 'Founder'`,
			seededClanID, s.level, s.reputation, s.dissolving)
		for i := range s.fillers {
			exec(`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('filler', ?, ?, 40, ?, 6)`,
				900000+i, "Filler"+strconv.Itoa(i), seededClanID)
		}
	})
}

// clanLevelItems boots the shared item catalog plus the three items clan
// levels 3 to 5 are paid with.
func clanLevelItems() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for id, name := range map[int32]string{1419: "Blood Mark", 3874: "Alliance Manifesto", 3870: "Seal of Aspiration"} {
		templates = append(templates, &item.Template{
			ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// storedClan reads the seeded clan's stored level and reputation.
func storedClan(t *testing.T, w *clanWorld) (level, reputation int64) {
	t.Helper()
	w.srv.FlushPersistence(t)
	return queryInt(t, w, `SELECT clan_level FROM clan_data WHERE clan_id = ?`, seededClanID),
		queryInt(t, w, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, seededClanID)
}

// TestRaiseClanLevelWithReputation buys level 6 with the clan's 10000
// reputation. With 29 members the clan is refused; once the recruit makes
// 30 the price is taken: every online member first learns its clan skills
// turned off as the score reaches 0, with its skill list, and gets the
// clan's header; then the leader is told what was deducted and that its
// clan earns reputation, and every member sees the new level.
func TestRaiseClanLevelWithReputation(t *testing.T) {
	w := bootClanWorld(t, 40, 0, 0, seedClan(t, clanSeed{level: 5, reputation: 10000, fillers: 28}))
	w.talkToMaster(t)

	if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Equal(ids, []int{serverpackets.SystemMessageFailedToIncreaseClanLevel}) {
		t.Fatalf("level-up with 29 members = %v, want FAILED_TO_INCREASE_CLAN_LEVEL", ids)
	}
	if level, rep := storedClan(t, w); level != 5 || rep != 10000 {
		t.Fatalf("stored clan after the refusal = level %d reputation %d, want 5 and 10000", level, rep)
	}

	w.recruit(t)
	frames := w.masterCommand(t, "increase_clan_level")
	if ids := messages(t, frames); !slices.Equal(ids, []int{
		serverpackets.SystemMessageReputationLowClanSkillsDeactivated, serverpackets.SystemMessageS1DeductedFromClanRep,
		serverpackets.SystemMessageClanCanAccumulateReputation, serverpackets.SystemMessageClanLevelIncreased,
	}) {
		t.Fatalf("level-up messages = %v (%x)", ids, opcodes(frames))
	}
	const sys, skills, info, visual = serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeMagicSkillUse
	if got := only(frames, sys, skills, info, visual); string(got) != string([]byte{sys, skills, info, sys, sys, info, sys, visual}) {
		t.Fatalf("leader's level-up frames = %x", opcodes(frames))
	}
	for _, f := range frames {
		if f[0] != sys {
			continue
		}
		if id, params := sysMsg(t, f); id == serverpackets.SystemMessageS1DeductedFromClanRep && !slices.Equal(params, []string{"10000"}) {
			t.Fatalf("deduction notice = %v, want 10000", params)
		}
	}
	member := drainFrames(t, w.member)
	if got := only(member, sys, skills, info); string(got) != string([]byte{sys, skills, info, info, sys}) {
		t.Fatalf("member's view of the level-up = %x", opcodes(member))
	}
	if ids := messages(t, member); !slices.Equal(ids, []int{serverpackets.SystemMessageReputationLowClanSkillsDeactivated, serverpackets.SystemMessageClanLevelIncreased}) {
		t.Fatalf("member's messages = %v", ids)
	}
	if level, rep := storedClan(t, w); level != 6 || rep != 0 {
		t.Fatalf("stored clan = level %d reputation %d, want 6 and 0", level, rep)
	}
}

// TestRaiseClanLevelReputationShort refuses level 6 to a clan of 30
// members one point short of the price, taking nothing.
func TestRaiseClanLevelReputationShort(t *testing.T) {
	w := bootClanWorld(t, 40, 0, 0, seedClan(t, clanSeed{level: 5, reputation: 9999, fillers: 29}))
	w.talkToMaster(t)

	if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Equal(ids, []int{serverpackets.SystemMessageFailedToIncreaseClanLevel}) {
		t.Fatalf("level-up short of reputation = %v, want FAILED_TO_INCREASE_CLAN_LEVEL", ids)
	}
	if level, rep := storedClan(t, w); level != 5 || rep != 9999 {
		t.Fatalf("stored clan = level %d reputation %d, want 5 and 9999", level, rep)
	}
}

// TestRaiseClanLevelWithItem raises levels 2, 3 and 4 for their SP and
// item: the item and SP go with their notices, and reaching level 5 also
// tells the leader its clan earns reputation. Without the item the leader
// is told it lacks it and the level-up fails.
func TestRaiseClanLevelWithItem(t *testing.T) {
	cases := []struct {
		level  int
		sp     int
		itemID int32
	}{
		{2, 500000, 1419},
		{3, 1400000, 3874},
		{4, 3500000, 3870},
	}
	for _, tc := range cases {
		t.Run("level "+strconv.Itoa(tc.level), func(t *testing.T) {
			w := bootClanWorldCarrying(t, 40, tc.sp, map[int32]int32{tc.itemID: 1}, clanLevelItems(), seedClan(t, clanSeed{level: tc.level}))
			w.talkToMaster(t)

			frames := w.masterCommand(t, "increase_clan_level")
			want := []int{serverpackets.SystemMessageS1Disappeared, serverpackets.SystemMessageSPDecreasedS1}
			if tc.level+1 == 5 {
				want = append(want, serverpackets.SystemMessageClanCanAccumulateReputation)
			}
			want = append(want, serverpackets.SystemMessageClanLevelIncreased)
			if ids := messages(t, frames); !slices.Equal(ids, want) {
				t.Fatalf("level-up messages = %v, want %v", ids, want)
			}
			if level, _ := storedClan(t, w); level != int64(tc.level+1) {
				t.Fatalf("stored clan level = %d, want %d", level, tc.level+1)
			}
		})
	}

	t.Run("without the item", func(t *testing.T) {
		w := bootClanWorldCarrying(t, 40, 500000, nil, clanLevelItems(), seedClan(t, clanSeed{level: 2}))
		w.talkToMaster(t)
		if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Equal(ids, []int{
			serverpackets.SystemMessageNotEnoughItems, serverpackets.SystemMessageFailedToIncreaseClanLevel,
		}) {
			t.Fatalf("level-up without the item = %v, want NOT_ENOUGH_ITEMS then FAILED_TO_INCREASE_CLAN_LEVEL", ids)
		}
		if level, _ := storedClan(t, w); level != 2 {
			t.Fatalf("stored clan level = %d, want 2", level)
		}
	})
}

// TestRaiseClanLevelWhileDissolving refuses any level-up while the clan's
// dissolution is pending, even one the leader could pay.
func TestRaiseClanLevelWhileDissolving(t *testing.T) {
	dissolving := time.Now().Add(24 * time.Hour).UnixMilli()
	w := bootClanWorld(t, 40, 30000, 650000, seedClan(t, clanSeed{dissolving: dissolving}))
	w.talkToMaster(t)

	if ids := messages(t, w.masterCommand(t, "increase_clan_level")); !slices.Equal(ids, []int{serverpackets.SystemMessageCannotRiseLevelWhileDissolving}) {
		t.Fatalf("level-up while dissolving = %v, want CANNOT_RISE_LEVEL_WHILE_DISSOLUTION_IN_PROGRESS", ids)
	}
	if level, _ := storedClan(t, w); level != 0 {
		t.Fatalf("stored clan level = %d, want 0", level)
	}
}
