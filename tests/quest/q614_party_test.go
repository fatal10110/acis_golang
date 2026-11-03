package quest

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q614"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Slay the Enemy Commander (Q614) scenarios cannot show, since a
// scenario seeds a party member neither a standing nor a quest row: the head
// of Tayr going to every Varka ally of the killer's party near him and only
// to them (Q614_SlayTheEnemyCommander.onMyDying, Quest.getPartyMembers). The
// offers and the trades of one character are
// q614_slay_the_enemy_commander_*.scenario.

const (
	q614Name = "Q614_SlayTheEnemyCommander"
	q614Path = "quest.Q614_SlayTheEnemyCommander"

	tayrID           = 25302
	headOfTayrID     = 7241
	varkaAlliance4ID = 7224
)

// TestQ614TayrCreditsEveryVarkaAllyOfTheParty: Leader (Varka 4, mark), Mate
// (Varka 3, mark), Third (Varka 5, mark), Fourth (Varka 4, no mark) and Fifth
// (Ketra 4, mark) are one party, all at cond 1 beside Tayr. Leader kills him:
// Leader and Third, the Varka allies of level 4 or more holding the mark, each
// get a head and move to cond 2, with the middle sound before the head; the
// others get nothing.
func TestQ614TayrCreditsEveryVarkaAllyOfTheParty(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{tayrID: script.KindHostile},
			[]script.Listing{{Path: q614Path}},
			script.Catalog{q614Path: q614.New},
		),
		varkaItemTemplates(headOfTayrID, varkaAlliance4ID),
	)
	type member struct {
		standing int
		mark     bool
		head     int
		cond     string
		lines    []string
	}
	credited := []string{
		"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0",
		itemMessage(54, headOfTayrID),
	}
	want := map[string]member{
		"Leader": {-4, true, 1, "2", credited},
		"Mate":   {-3, true, 0, "1", nil},
		"Third":  {-5, true, 1, "2", credited},
		"Fourth": {-4, false, 0, "1", nil},
		"Fifth":  {4, true, 0, "1", nil},
	}
	mates := []partyMate{{"mate", "Mate", 75}, {"third", "Third", 75}, {"fourth", "Fourth", 75}, {"fifth", "Fifth", 75}}
	ids, clients := formParty(t, srv, "Leader", mates, func(id int32, name string) {
		insertJournal(t, srv.DB,
			journalRow{id, q614Name, "<state>", val("STARTED")},
			journalRow{id, q614Name, "<cond>", val("1")},
		)
		seedStanding(t, srv, id, want[name].standing)
		if want[name].mark {
			srv.GiveItem(t, id, varkaAlliance4ID, 1)
		}
	})

	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	tayr := srv.SpawnHostileNPCTemplateAt(t, proofMonster(tayrID), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, srv, ids["Leader"], tayr)

	for name, w := range want {
		id := ids[name]
		if got := questSoundsAndItems(scriptLines(t, srv.ReadQueued(t, clients[id])), headOfTayrID); !slices.Equal(got, w.lines) {
			t.Errorf("%s's quest sounds and messages = %q, want %q", name, got, w.lines)
		}
		if n := srv.PlayerItemCount(t, id, headOfTayrID); n != w.head {
			t.Errorf("%s's heads = %d, want %d", name, n, w.head)
		}
		if vars := srv.QuestVars(t, id, q614Name); vars["<cond>"] != w.cond {
			t.Errorf("%s's Q614 = %v, want cond %s", name, vars, w.cond)
		}
	}
}
