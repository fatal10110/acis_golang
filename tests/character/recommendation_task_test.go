package character

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// recommendationTaskBoot is when the suite's task clock starts; the
// shipped RecommendationUpdate entry (DAILY, 13:00:00) is next due at
// recommendationTaskDue.
var (
	recommendationTaskBoot = time.Date(2026, 10, 6, 12, 59, 0, 0, time.UTC)
	recommendationTaskDue  = time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)
)

// recommendationTaskWorld is the booted server with Giver (level 20,
// holding 7 and 2 left) and Taker (level 1) in the world, Giver selecting
// Taker, and Sleeper (level 45, holding 10, none left, having recommended
// Giver) stored only.
type recommendationTaskWorld struct {
	srv              *gameservertest.Server
	giver, taker     *testsupport.ScriptedClient
	giverID, takerID int32
	sleeperID        int32
}

func bootRecommendationTask(t *testing.T, extra ...gameservertest.Option) *recommendationTaskWorld {
	t.Helper()
	list, err := gamexml.LoadScriptList(datapack.Path(t, "data", "xml", "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatalf("load scripts.xml: %v", err)
	}
	i := slices.IndexFunc(list, func(l script.Listing) bool { return l.Path == "task.RecommendationUpdate" })
	if i < 0 {
		t.Fatal("scripts.xml does not list task.RecommendationUpdate")
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Giver", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithScripts(list[i:i+1], script.Catalog{"task.RecommendationUpdate": scripttask.RecommendationUpdate}),
		gameservertest.WithScheduledTasks(recommendationTaskBoot),
	}, extra...)...)
	w := &recommendationTaskWorld{srv: srv, giver: srv.Client, giverID: srv.SoleObjectID(t)}
	w.takerID = srv.SeedCharacterFor(t, "player2", "Taker", 1, 0).ID
	w.sleeperID = srv.SeedCharacterFor(t, "player3", "Sleeper", 45, 0).ID
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`UPDATE characters SET rec_left = 2, rec_have = 7 WHERE obj_Id = ?`, []any{w.giverID}},
		{`UPDATE characters SET rec_left = 0, rec_have = 10 WHERE obj_Id = ?`, []any{w.sleeperID}},
		{`INSERT INTO character_recommends (char_id, target_id) VALUES (?, ?)`, []any{w.sleeperID, w.giverID}},
	} {
		if _, err := srv.DB.ExecContext(context.Background(), q.query, q.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	w.taker = srv.DialClient(t, "player2", 1)
	enterWorldQuiet(t, w.giver)
	enterWorldQuiet(t, w.taker)
	w.giver.Send(encodeAction(w.takerID, 0, 0, 0, false))
	drainQuiet(t, w.giver)
	drainQuiet(t, w.taker)
	return w
}

// TestRecommendationUpdateRunsDaily: at the shipped 13:00 start, not
// before, each online player gets one UserInfo with its counters reset by
// level (Giver 6 left and 7 - 2 held, Taker 3 and 0), the stored-only
// Sleeper gets 9 left and 10 - 3 held, and character_recommends is
// emptied.
func TestRecommendationUpdateRunsDaily(t *testing.T) {
	w := bootRecommendationTask(t)
	clock := w.srv.ScheduleClock

	clock.Advance(recommendationTaskDue.Sub(clock.Now()) - time.Second)
	w.srv.Settle(t)
	for _, c := range []*testsupport.ScriptedClient{w.giver, w.taker} {
		if frames := quietFrames(t, c); len(frames) != 0 {
			t.Fatalf("frames before 13:00 %x, want none", opcodes(frames))
		}
	}

	clock.Advance(time.Second)
	w.srv.Settle(t)
	w.srv.FlushPersistence(t)
	for _, c := range []*testsupport.ScriptedClient{w.giver, w.taker} {
		if frames := quietFrames(t, c); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeUserInfo {
			t.Fatalf("refresh frames %x, want one UserInfo", opcodes(frames))
		}
	}
	for _, tc := range []struct {
		name       string
		id         int32
		have, left int
		online     bool
	}{
		{"Giver", w.giverID, 5, 6, true},
		{"Taker", w.takerID, 0, 3, true},
		{"Sleeper", w.sleeperID, 7, 9, false},
	} {
		if tc.online {
			if c := onlineChar(t, w.srv, tc.id); c.RecommendationsHave() != tc.have || c.RecommendationsLeft() != tc.left {
				t.Errorf("%s holds %d with %d left, want %d and %d", tc.name, c.RecommendationsHave(), c.RecommendationsLeft(), tc.have, tc.left)
			}
		}
		if have, left := recommendationColumns(t, w.srv, tc.id); have != tc.have || left != tc.left {
			t.Errorf("stored %s rec_have %d rec_left %d, want %d and %d", tc.name, have, left, tc.have, tc.left)
		}
	}
	var rows int
	if err := w.srv.DB.QueryRow(`SELECT COUNT(*) FROM character_recommends`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("character_recommends rows = %d, %v; want 0", rows, err)
	}
}

// TestRecommendationUpdateRacesRecommend: Giver recommends Taker while the
// scheduled refresh runs. Whichever side of the refresh the
// recommendation falls on, what both players hold is what is stored, and
// the stored record matches whether Giver may recommend Taker again.
func TestRecommendationUpdateRacesRecommend(t *testing.T) {
	for _, exec := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(exec.name, func(t *testing.T) { recommendationUpdateRacesRecommend(t, exec.opts...) })
	}
}

func recommendationUpdateRacesRecommend(t *testing.T, extra ...gameservertest.Option) {
	w := bootRecommendationTask(t, extra...)
	clock := w.srv.ScheduleClock
	clock.Advance(recommendationTaskDue.Sub(clock.Now()) - time.Second)
	w.srv.Settle(t)

	w.giver.Send(encodeRequestEvaluate(w.takerID))
	clock.Advance(time.Second)
	w.srv.Settle(t)
	drainQuiet(t, w.giver)
	drainQuiet(t, w.taker)
	w.srv.FlushPersistence(t)

	for _, id := range []int32{w.giverID, w.takerID} {
		c := onlineChar(t, w.srv, id)
		if have, left := recommendationColumns(t, w.srv, id); have != c.RecommendationsHave() || left != c.RecommendationsLeft() {
			t.Errorf("player %d stored %d held, %d left; holds %d, %d left", id, have, left, c.RecommendationsHave(), c.RecommendationsLeft())
		}
	}
	var rows int
	if err := w.srv.DB.QueryRow(`SELECT COUNT(*) FROM character_recommends WHERE char_id = ? AND target_id = ?`, w.giverID, w.takerID).Scan(&rows); err != nil {
		t.Fatalf("read character_recommends: %v", err)
	}
	w.giver.Send(encodeRequestEvaluate(w.takerID))
	again := parseSystemMessage(t, w.giver.Read()).id == 830
	if again == (rows == 1) {
		t.Fatalf("stored giver->taker rows = %d, but recommending again succeeded = %v", rows, again)
	}
}
