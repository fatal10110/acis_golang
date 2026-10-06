package task

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestTasksMatchReferenceFingerprints compares the package with the
// reference's task classes: literals, engine calls and hook shapes.
//
// Exempt: the "task" description every task passes its base class, which a
// Go script has no field for; and the end hooks, which no task reacts to
// and the runner does not build.
func TestTasksMatchReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"task.CastleTaxRefresh", "task.ClanLadderRefresh", "task.ClanLeaderTransfer",
		"task.RaidPointReset", "task.RecommendationUpdate", "task.SevenSignsUpdate",
	},
		`string "task"`, "hook onEnd none",
		// The recommendation refresh's body (its level bands, SQL, log
		// line and per-player calls) is Server.RefreshRecommendations, a
		// server routine that predates the task.
		"number 20", "number 40",
		`string "Couldn't clear players recommendations."`,
		`string "SELECT obj_Id, level, rec_have FROM characters"`,
		`string "TRUNCATE character_recommends"`,
		`string "UPDATE characters SET rec_left=?, rec_have=? WHERE obj_Id=?"`,
		`string "level"`, `string "obj_Id"`, `string "rec_have"`,
		"call CLogger.error", "call ConnectionPool.getConnection", "call Player.getStatus",
		"call Player.sendPacket", "call PlayerStatus.getLevel", "call UserInfo.new",
		// The raid point reset reads a clan's level and credits its
		// reputation through Server.MemberClan and Server.AddClanReputation;
		// these rows are shared with quests, which map them on their own
		// handles.
		"call Clan.getLevel", "call Clan.addReputationScore")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// server records the Server calls a task makes, in order.
type server struct {
	sealValidation bool
	winners        []int32
	clans          map[int32][2]int32 // member object id: clan id, clan level
	calls          []string
}

func (s *server) UpdateCastleTaxes()         { s.calls = append(s.calls, "UpdateCastleTaxes") }
func (s *server) SealValidationPeriod() bool { return s.sealValidation }
func (s *server) SaveFestivalScores(context.Context) {
	s.calls = append(s.calls, "SaveFestivalScores")
}
func (s *server) SaveSevenSigns(context.Context) { s.calls = append(s.calls, "SaveSevenSigns") }
func (s *server) TransferClanLeaders()           { s.calls = append(s.calls, "TransferClanLeaders") }
func (s *server) RefreshClanLadder()             { s.calls = append(s.calls, "RefreshClanLadder") }
func (s *server) RefreshRecommendations(context.Context) {
	s.calls = append(s.calls, "RefreshRecommendations")
}
func (s *server) RaidPointWinners() []int32 { return s.winners }
func (s *server) MemberClan(objectID int32) (int32, int, bool) {
	c, ok := s.clans[objectID]
	return c[0], int(c[1]), ok
}

func (s *server) AddClanReputation(clanID int32, points int) {
	s.calls = append(s.calls, fmt.Sprintf("AddClanReputation %d %d", clanID, points))
}
func (s *server) CleanUpRaidPoints() { s.calls = append(s.calls, "CleanUpRaidPoints") }

// TestTaskStarts pins what each task's start does, in order.
func TestTaskStarts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		ctor           func() script.Script
		sealValidation bool
		want           []string
	}{
		{"CastleTaxRefresh", CastleTaxRefresh, false, []string{"UpdateCastleTaxes"}},
		{"ClanLeaderTransfer", ClanLeaderTransfer, false, []string{"TransferClanLeaders"}},
		{"SevenSignsUpdate", SevenSignsUpdate, false, []string{"SaveFestivalScores", "SaveSevenSigns"}},
		{"SevenSignsUpdate in seal validation", SevenSignsUpdate, true, []string{"SaveSevenSigns"}},
		{"ClanLadderRefresh", ClanLadderRefresh, false, []string{"RefreshClanLadder"}},
		{"RecommendationUpdate", RecommendationUpdate, false, []string{"RefreshRecommendations"}},
		{"RaidPointReset with no winners", RaidPointReset, false, []string{"CleanUpRaidPoints"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &server{sealValidation: tc.sealValidation}
			s := tc.ctor()
			s.Hooks.Start(&s, script.Start{Ctx: context.Background(), Server: srv})
			if !slices.Equal(srv.calls, tc.want) {
				t.Fatalf("start calls %v, want %v", srv.calls, tc.want)
			}
		})
	}
}

// TestRaidPointResetRewardsPlaces pins the place-to-reputation table and
// the clan gate: each clan of level 5 or more gains the sum of its members'
// places, in the order its first member placed; a clan below level 5 and a
// player in no clan earn nothing; then the points are wiped.
func TestRaidPointResetRewardsPlaces(t *testing.T) {
	winners := make([]int32, 0, 100)
	for id := int32(1); id <= 100; id++ {
		winners = append(winners, id)
	}
	clans := map[int32][2]int32{}
	// Clan 7, level 5: places 1-10 and 11.
	for id := int32(1); id <= 11; id++ {
		clans[id] = [2]int32{7, 5}
	}
	// Clan 8, level 6: places 50 and 51, then 100.
	clans[50], clans[51], clans[100] = [2]int32{8, 6}, [2]int32{8, 6}, [2]int32{8, 6}
	// Clan 9, level 4: place 12.
	clans[12] = [2]int32{9, 4}
	srv := &server{winners: winners, clans: clans}
	s := RaidPointReset()
	s.Hooks.Start(&s, script.Start{Ctx: context.Background(), Server: srv})
	top10 := 1250 + 900 + 700 + 600 + 450 + 350 + 300 + 200 + 150 + 100
	want := []string{
		fmt.Sprintf("AddClanReputation 7 %d", top10+25),
		fmt.Sprintf("AddClanReputation 8 %d", 25+12+12),
		"CleanUpRaidPoints",
	}
	if !slices.Equal(srv.calls, want) {
		t.Fatalf("calls %v, want %v", srv.calls, want)
	}
}
