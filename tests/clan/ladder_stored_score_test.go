package clan

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ladderFieldBaseID numbers the ladder's 99 memberless level 5 clans.
const ladderFieldBaseID int32 = 0x7f000100

// TestClanLadderRefreshOrdersByStoredScore boots at 00:04 with the
// founder's Knights at level 4 whose row still says 10000 (it holds none
// below level 5), 98 level 5 clans scoring 100 to 197 and Last, level 5,
// scoring 50. The ladder takes the 99 best stored scores, so Knights
// takes a slot without a rank and Last stays off it, at boot and at the
// 00:05 refresh. A leader nomination stores Knights' row with the 0 it
// holds; the next day's refresh then ranks Last 99th.
func TestClanLadderRefreshOrdersByStoredScore(t *testing.T) {
	boot := time.Date(2026, 10, 6, 0, 4, 0, 0, time.UTC)
	lastID := ladderFieldBaseID + 98
	w := bootClanWorld(t, 10, 0, 0,
		gameservertest.WithClanSeed(func(db *sql.DB) {
			var field []string
			for i := range int32(98) {
				field = append(field, fmt.Sprintf("(%d, 'Ladder%d', 5, %d)", ladderFieldBaseID+i, i, 100+i))
			}
			field = append(field, fmt.Sprintf("(%d, 'Last', 5, 50)", lastID))
			seedStatements(t, db,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
					SELECT `+itoa(titleClanID)+`, 'Knights', 4, 10000, obj_Id FROM characters WHERE char_name = 'Founder'`,
				`UPDATE characters SET clanid = `+itoa(titleClanID)+`, power_grade = 0 WHERE char_name = 'Founder'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score) VALUES `+strings.Join(field, ", "),
			)
		}),
		shippedTask(t, "task.ClanLadderRefresh", scripttask.ClanLadderRefresh),
		gameservertest.WithScheduledTasks(boot),
	)
	table := w.srv.Clans.Table()
	rank := func(id int32) int {
		t.Helper()
		cl, ok := table.Get(id)
		if !ok {
			t.Fatalf("clan %d is not restored", id)
		}
		return cl.Info().Rank
	}
	check := func(when string, want map[int32]int) {
		t.Helper()
		for id, r := range want {
			if got := rank(id); got != r {
				t.Errorf("%s: clan %#x rank = %d, want %d", when, id, got, r)
			}
		}
	}
	slotTaken := map[int32]int{
		titleClanID:            0,
		ladderFieldBaseID + 97: 1,
		ladderFieldBaseID:      98,
		lastID:                 0,
	}
	check("boot", slotTaken)

	w.srv.ScheduleClock.Advance(boot.Add(time.Minute).Sub(w.srv.ScheduleClock.Now()))
	w.srv.Settle(t)
	check("00:05 refresh", slotTaken)

	w.recruit(t)
	if page := pageText(t, w.command(t, "change_clan_leader Recruit")); !strings.Contains(page, "has been submitted") {
		t.Fatalf("nomination page = %q, want 9000-07-success", page)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, titleClanID); got != 0 {
		t.Fatalf("Knights stored score after the nomination = %d, want the 0 it holds", got)
	}
	check("before the next refresh", slotTaken)

	w.srv.ScheduleClock.Advance(24 * time.Hour)
	w.srv.Settle(t)
	check("next day's refresh", map[int32]int{
		titleClanID:            0,
		ladderFieldBaseID + 97: 1,
		ladderFieldBaseID:      98,
		lastID:                 99,
	})
}
