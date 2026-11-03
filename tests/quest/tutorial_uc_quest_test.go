package quest

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	featuretutorial "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/tutorial"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q101"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTutorialSecondMarkSeesAStartedNewbieQuest: a newbie whose tutorial
// stands at its second question mark (ucMemo 2) enters the world. Holding a
// state of Sword of Solidarity, one of the eight newbie quests the step
// looks for, it is shown question mark 6 and the tutorial moves to ucMemo
// 5; without one it is shown question mark 2 and the tutorial stays.
func TestTutorialSecondMarkSeesAStartedNewbieQuest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		quest  bool
		mark   int32
		ucMemo string
	}{
		{name: "quest started", quest: true, mark: 6, ucMemo: "5"},
		{name: "no quest", mark: 2, ucMemo: "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q101Path, tutorialPath := "quest.Q101_SwordOfSolidarity", "script.feature.Tutorial"
			srv := bootNewbie(t, true, true, gameservertest.WithScripts(
				[]script.Listing{{Path: q101Path}, {Path: tutorialPath}},
				script.Catalog{q101Path: q101.New, tutorialPath: featuretutorial.New}))
			objID := createNewbie(t, srv)
			srv.FlushPersistence(t)
			rows := [][3]string{{tutorial, "ucMemo", "2"}}
			if tc.quest {
				rows = append(rows, [3]string{"Q101_SwordOfSolidarity", "<state>", "STARTED"}, [3]string{"Q101_SwordOfSolidarity", "<cond>", "1"})
			}
			for _, r := range rows {
				if _, err := srv.DB.Exec("INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", objID, r[0], r[1], r[2]); err != nil {
					t.Fatal(err)
				}
			}

			burstTail(t, enterBurst(t, srv),
				serverpackets.FrameTutorialShowQuestionMark(tc.mark),
				serverpackets.FramePlaySound(script.SoundTutorial),
				serverpackets.FrameTutorialEnableClientEvent(0))
			want := map[string]string{"<state>": "STARTED", "ucMemo": tc.ucMemo}
			if got := srv.QuestVars(t, objID, tutorial); !reflect.DeepEqual(got, want) {
				t.Fatalf("tutorial after the enter = %v, want %v", got, want)
			}
		})
	}
}
