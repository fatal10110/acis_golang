package character

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// recommendationTrio boots two level-20 givers and a level-1 taker, each on
// its own persistence lane, all in the world.
type recommendationTrio struct {
	srv                    *gameservertest.Server
	first, second, taker   *testsupport.ScriptedClient
	firstID, secondID, tID int32
}

func bootRecommendationTrio(t *testing.T) recommendationTrio {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("First", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	r := recommendationTrio{srv: srv, first: srv.Client, firstID: srv.SoleObjectID(t)}
	r.secondID = srv.SeedCharacterFor(t, "player2", "Second", 20, 0).ID
	r.tID = srv.SeedCharacterFor(t, "player3", "Taker", 1, 0).ID
	lanes := map[uint32]bool{}
	for _, id := range []int32{r.firstID, r.secondID, r.tID} {
		lanes[persist.LaneIndex(id)] = true
	}
	if len(lanes) != 3 {
		t.Fatalf("ids %d, %d, %d share a persistence lane; the scenarios need distinct lanes", r.firstID, r.secondID, r.tID)
	}
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET rec_left = 3 WHERE obj_Id IN (?, ?)`, r.firstID, r.secondID); err != nil {
		t.Fatalf("seed counters: %v", err)
	}
	r.second = srv.DialClient(t, "player2", 1)
	r.taker = srv.DialClient(t, "player3", 1)
	enterWorldQuiet(t, r.first)
	enterWorldQuiet(t, r.second)
	enterWorldQuiet(t, r.taker)
	r.drain(t)
	return r
}

func (r recommendationTrio) drain(t *testing.T) {
	t.Helper()
	drainQuiet(t, r.first)
	drainQuiet(t, r.second)
	drainQuiet(t, r.taker)
}

// recommendTaker has giver select the taker and recommend it, expecting
// the 830 acknowledgement.
func (r recommendationTrio) recommendTaker(t *testing.T, giver *testsupport.ScriptedClient) {
	t.Helper()
	giver.Send(encodeAction(r.tID, 0, 0, 0, false))
	r.drain(t)
	giver.Send(encodeRequestEvaluate(r.tID))
	if got := parseSystemMessage(t, giver.Read()); got.id != 830 {
		t.Fatalf("recommend message %d, want 830", got.id)
	}
	r.drain(t)
}

func recommendationRows(t *testing.T, srv *gameservertest.Server, giverID, targetID int32) int {
	t.Helper()
	var n int
	if err := srv.DB.QueryRow(`SELECT COUNT(*) FROM character_recommends WHERE char_id = ? AND target_id = ?`, giverID, targetID).Scan(&n); err != nil {
		t.Fatalf("count character_recommends: %v", err)
	}
	return n
}

// Two givers on different lanes recommend the same taker while the first
// giver's lane is stalled: whichever order the lanes then run in, the stored
// count matches what the taker holds, so its next login loses nothing.
func TestRecommendationsFromStalledLanesKeepStoredCount(t *testing.T) {
	r := bootRecommendationTrio(t)
	release := r.srv.HoldPersistenceLane(t, r.firstID)

	r.recommendTaker(t, r.first)
	r.recommendTaker(t, r.second)
	release()
	r.srv.FlushPersistence(t)

	if held := onlineChar(t, r.srv, r.tID).RecommendationsHave(); held != 2 {
		t.Fatalf("taker holds %d, want 2", held)
	}
	if have, _ := recommendationColumns(t, r.srv, r.tID); have != 2 {
		t.Fatalf("stored taker rec_have = %d, want 2 (what the taker holds)", have)
	}
	if _, left := recommendationColumns(t, r.srv, r.firstID); left != 2 {
		t.Fatalf("stored first giver rec_left = %d, want 2", left)
	}
}

// The taker relogs while the giver's lane is stalled behind its
// recommendation: the login still reads the recommendation it was given.
func TestRecommendedTakerRelogWhileGiverLaneStalled(t *testing.T) {
	r := bootRecommendationTrio(t)
	release := r.srv.HoldPersistenceLane(t, r.firstID)

	r.recommendTaker(t, r.first)
	r.taker.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	r.drain(t)
	enterWorldQuiet(t, r.taker)
	r.drain(t)

	if held := onlineChar(t, r.srv, r.tID).RecommendationsHave(); held != 1 {
		t.Fatalf("taker holds %d after relog, want 1", held)
	}
	release()
	r.srv.FlushPersistence(t)
	if have, _ := recommendationColumns(t, r.srv, r.tID); have != 1 {
		t.Fatalf("stored taker rec_have = %d, want 1", have)
	}
}

// A recommendation still queued when the daily refresh runs is stored before
// the refresh resets the tables, and one given once the refresh has applied
// online is stored after it: the stored record and counters end equal to
// what the players hold, so a relog neither restores a spent recommendation
// nor forgets one given after the refresh.
func TestRecommendationQueuedAcrossDailyRefresh(t *testing.T) {
	r := bootRecommendationTrio(t)
	release := r.srv.HoldPersistenceLane(t, r.firstID)

	// Before the refresh: the first giver's record and remaining count are
	// stuck behind its stalled lane.
	r.recommendTaker(t, r.first)

	wait := r.srv.StartDailyRecommendationRefresh(t)
	first := onlineChar(t, r.srv, r.firstID)
	deadline := time.Now().Add(10 * time.Second)
	for first.RecommendationsLeft() != 6 {
		if time.Now().After(deadline) {
			t.Fatalf("online refresh never applied: first giver left %d, want 6", first.RecommendationsLeft())
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.drain(t)

	// After the online refresh, while the stored refresh still waits for
	// the stalled lane: the first giver recommends the taker again.
	r.recommendTaker(t, r.first)
	release()
	wait()
	r.drain(t)
	r.srv.FlushPersistence(t)

	if rows := recommendationRows(t, r.srv, r.firstID, r.tID); rows != 1 {
		t.Fatalf("character_recommends rows first->taker = %d, want 1 (the post-refresh one)", rows)
	}
	if left := first.RecommendationsLeft(); left != 5 {
		t.Fatalf("first giver left %d, want 5", left)
	}
	if _, left := recommendationColumns(t, r.srv, r.firstID); left != 5 {
		t.Fatalf("stored first giver rec_left = %d, want 5 (what it holds)", left)
	}
	held := onlineChar(t, r.srv, r.tID).RecommendationsHave()
	if held != 1 {
		t.Fatalf("taker holds %d, want 1 (1, refreshed to 0, then 1)", held)
	}
	if have, _ := recommendationColumns(t, r.srv, r.tID); have != held {
		t.Fatalf("stored taker rec_have = %d, want %d (what it holds)", have, held)
	}
}
