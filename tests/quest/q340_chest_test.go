package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q340"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The chest of Bifrons in Subjugation of Lizardmen
// (Q340_SubjugationOfLizardmen onMyDying, Quest.addSpawn): the death of the
// Serpent Demon Bifrons leaves the chest where the boss fell, for 30 s, for
// a character whose quest is started. A scenario cannot show a script-spawned
// NPC appearing (#4020 item 5), so this Boot test pins the call; the
// scenarios play the dialogs and the lizardmen's drops.
const (
	q340Name    = "Q340_SubjugationOfLizardmen"
	q340Path    = "quest.Q340_SubjugationOfLizardmen"
	q340Bifrons = 25146
	q340Chest   = 30989
)

func TestQ340BifronsLeavesItsChestFor30Seconds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		started bool
		chests  int
	}{
		{"started", true, 1},
		{"no quest", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSeamQuest(t, 20, nil, func(srv *gameservertest.Server, objID int32) {
				if tc.started {
					seedJournal(t, srv, objID, []string{q340Name + ":STARTED"})
				}
			},
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
					proofMonster(q340Bifrons), gameservertest.FolkTemplate("Folk", q340Chest),
				})),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(
					map[int32]script.NPCKind{q340Bifrons: script.KindHostile, q340Chest: script.KindFolk},
					[]script.Listing{{Path: q340Path}},
					script.Catalog{q340Path: q340.New},
				),
			)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q340Bifrons), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			hx, hy, _ := h.Position()
			killOnPlayerQueue(t, w.srv, w.player, h)

			chests := npcsOf(w.srv, q340Chest)
			if len(chests) != tc.chests {
				t.Fatalf("%d chests after the death, want %d", len(chests), tc.chests)
			}
			if tc.chests == 0 {
				return
			}
			if at := chests[0].SpawnLocation(); at.X != hx || at.Y != hy {
				t.Fatalf("the chest stands at (%d, %d), want where the boss fell (%d, %d)", at.X, at.Y, hx, hy)
			}
			// The kill lets the dying hook's seconds pass: the chest has
			// stood for between 0 and 3.5 s by now.
			w.srv.Advance(t, 25*time.Second)
			if n := len(npcsOf(w.srv, q340Chest)); n != 1 {
				t.Fatalf("%d chests 25 s after the death, want 1", n)
			}
			w.srv.Advance(t, 6*time.Second)
			if n := len(npcsOf(w.srv, q340Chest)); n != 0 {
				t.Fatalf("%d chests after the 30 s, want 0", n)
			}
		})
	}
}
