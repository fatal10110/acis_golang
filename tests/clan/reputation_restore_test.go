package clan

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// clampFieldBaseID numbers the memberless clans whose stored score lies
// past the reputation bounds.
const clampFieldBaseID int32 = 0x7f000300

// TestClanRestoreStoresClampedReputation boots with level 5 clans whose
// rows were edited past the reputation bounds, one in range and a level 4
// one past them. Restoring a clan of level 5 or more sets its score through
// the bounded setter, which stores the bounded score
// (ClanTable constructor, Clan.setReputationScore), so both rows past the
// bounds read the bound after boot; the in-range row is unchanged, and the
// level 4 clan, which holds no reputation, keeps its row as edited.
func TestClanRestoreStoresClampedReputation(t *testing.T) {
	t.Parallel()
	high, low, inRange, belowLevel := clampFieldBaseID, clampFieldBaseID+1, clampFieldBaseID+2, clampFieldBaseID+3
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedStatements(t, db,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score) VALUES
					(`+itoa(high)+`, 'High', 5, 150000000),
					(`+itoa(low)+`, 'Low', 8, -150000000),
					(`+itoa(inRange)+`, 'Steady', 5, 12345),
					(`+itoa(belowLevel)+`, 'Small', 4, 150000000)`)
		}),
	)
	srv.FlushPersistence(t)
	for id, want := range map[int32]int64{high: 100000000, low: -100000000, inRange: 12345, belowLevel: 150000000} {
		var got int64
		if err := srv.DB.QueryRowContext(context.Background(), `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("read clan %#x score: %v", id, err)
		}
		if got != want {
			t.Errorf("clan %#x stored score after boot = %d, want %d", id, got, want)
		}
	}
	if cl, ok := srv.Clans.Table().Get(high); !ok || cl.Info().Reputation != 100000000 {
		t.Fatal("restored High does not hold 100000000")
	}
}
