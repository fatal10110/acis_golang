package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q634"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the scenarios of In Search of Fragments of Dimension (Q634) cannot
// show: a scenario's monsters are all level 1, and the number of fragments a
// monster drops grows with its level.

const (
	q634Name         = "Q634_InSearchOfFragmentsOfDimension"
	q634Path         = "quest.Q634_InSearchOfFragmentsOfDimension"
	dimensionFragID  = 7079
	q634Mob          = 21208
	q634GatekeeperID = 31494
)

// TestQ634FragmentsGrowWithTheMonsterLevel: with the draw under the chance, a
// level 20 monster drops 4 fragments and a level 77 one drops 13 (the level
// times 0.15, plus 1.6, truncated).
func TestQ634FragmentsGrowWithTheMonsterLevel(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 40, nil, func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q634Name + ":STARTED"})
	},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q634Mob), gameservertest.FolkTemplate("Folk", q634GatekeeperID),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q634Mob: script.KindHostile, q634GatekeeperID: script.KindFolk},
			[]script.Listing{{Path: q634Path}},
			script.Catalog{q634Path: q634.New},
		),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
			ID: dimensionFragID, Name: "Dimension Fragment", Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		}))),
	)

	total := 0
	for i, level := range []int{20, 77} {
		tmpl := proofMonster(q634Mob)
		tmpl.Level = level
		h := w.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: w.at.X + 40*(i+1), Y: w.at.Y, Z: w.at.Z})
		killOnPlayerQueue(t, w.srv, w.player, h)
		total += []int{4, 13}[i]
		if got := w.srv.PlayerItemCount(t, w.player, dimensionFragID); got != total {
			t.Fatalf("after the level %d monster: %d fragments, want %d", level, got, total)
		}
	}
}
