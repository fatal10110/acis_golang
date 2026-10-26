package quest

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q611"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q612"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q613"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Prove your courage! (Q613) and Alliance with Varka Silenos (Q611)
// scenarios cannot show, since a scenario seeds a party member neither a
// standing nor a quest row: the head of Hekaton going to every Varka ally of
// the killer's party near him and only to them (Q613_ProveYourCourage.
// onMyDying, Quest.getPartyMembers), and the badge or the molar of a Ketra
// warrior going to one random started member of the killer's party, the molar
// only to one that also started War with Ketra Orcs and only from a warrior
// that drops molars (Q611_AllianceWithVarkaSilenos.onMyDying,
// Quest.getRandomPartyMemberState). The offers, the trades and the drops of
// one character are q613_prove_your_courage_*.scenario,
// q611_alliance_with_varka_silenos_*.scenario and q612_war_with_ketra_orcs_*.scenario.

const (
	q611Name = "Q611_AllianceWithVarkaSilenos"
	q611Path = "quest.Q611_AllianceWithVarkaSilenos"
	q612Name = "Q612_WarWithKetraOrcs"
	q612Path = "quest.Q612_WarWithKetraOrcs"
	q613Name = "Q613_ProveYourCourage"
	q613Path = "quest.Q613_ProveYourCourage"

	hekatonID        = 25299
	headOfHekatonID  = 7240
	varkaAlliance3ID = 7223
	ketraSoldierID   = 21324
	ketraSoldier2ID  = 21325
	ketraBadgeID     = 7226
	ketraMolarID     = 7234
)

// TestQ613HekatonCreditsEveryVarkaAllyOfTheParty: Leader (Varka 3, mark),
// Mate (Varka 2, mark), Third (Varka 5, mark), Fourth (Varka 3, no mark) and
// Fifth (Ketra 3, mark) are one party, all at cond 1 beside Hekaton. Leader
// kills him: Leader and Third, the Varka allies of level 3 or more holding
// the mark, each get a head and move to cond 2, with the middle sound before
// the head; the others get nothing.
func TestQ613HekatonCreditsEveryVarkaAllyOfTheParty(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{hekatonID: script.KindHostile},
			[]script.Listing{{Path: q613Path}},
			script.Catalog{q613Path: q613.New},
		),
		varkaItemTemplates(headOfHekatonID, varkaAlliance3ID),
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
		itemMessage(54, headOfHekatonID),
	}
	want := map[string]member{
		"Leader": {-3, true, 1, "2", credited},
		"Mate":   {-2, true, 0, "1", nil},
		"Third":  {-5, true, 1, "2", credited},
		"Fourth": {-3, false, 0, "1", nil},
		"Fifth":  {3, true, 0, "1", nil},
	}
	mates := []partyMate{{"mate", "Mate", 75}, {"third", "Third", 75}, {"fourth", "Fourth", 75}, {"fifth", "Fifth", 75}}
	ids, clients := formParty(t, srv, "Leader", mates, func(id int32, name string) {
		insertJournal(t, srv.DB,
			journalRow{id, q613Name, "<state>", val("STARTED")},
			journalRow{id, q613Name, "<cond>", val("1")},
		)
		seedStanding(t, srv, id, want[name].standing)
		if want[name].mark {
			srv.GiveItem(t, id, varkaAlliance3ID, 1)
		}
	})

	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	hekaton := srv.SpawnHostileNPCTemplateAt(t, proofMonster(hekatonID), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, srv, ids["Leader"], hekaton)

	for name, w := range want {
		id := ids[name]
		if got := questSoundsAndItems(scriptLines(t, srv.ReadQueued(t, clients[id])), headOfHekatonID); !slices.Equal(got, w.lines) {
			t.Errorf("%s's quest sounds and messages = %q, want %q", name, got, w.lines)
		}
		if n := srv.PlayerItemCount(t, id, headOfHekatonID); n != w.head {
			t.Errorf("%s's heads = %d, want %d", name, n, w.head)
		}
		if vars := srv.QuestVars(t, id, q613Name); vars["<cond>"] != w.cond {
			t.Errorf("%s's Q613 = %v, want cond %s", name, vars, w.cond)
		}
	}
}

// TestQ611DropGoesToOneRandomStartedMemberOfTheParty: Leader (Q611 at cond 1
// and Q612 started) and Mate (Q611 at cond 1 only) are one party beside Ketra
// soldiers. A drop is for the member the first draw picks. Draws are the
// pick, then, for a member that started Q612, the coin of the molar, then the
// chance of the drop (0 succeeds; a coin of 0 is heads).
//
//	soldier 21324, pick Leader, heads:  Leader gets a molar
//	soldier 21324, pick Mate:           Mate gets a badge, without a coin
//	soldier 21324, pick Leader, tails:  Leader gets a badge
//	soldier 21325, pick Leader, heads:  Leader gets a badge, 21325 dropping no molar
func TestQ611DropGoesToOneRandomStartedMemberOfTheParty(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{ketraSoldierID: script.KindHostile, ketraSoldier2ID: script.KindHostile},
			[]script.Listing{{Path: q611Path}, {Path: q612Path}},
			script.Catalog{q611Path: q611.New, q612Path: q612.New},
		),
		varkaItemTemplates(ketraBadgeID, ketraMolarID),
	)
	mates := []partyMate{{"mate", "Mate", 75}}
	ids, _ := formParty(t, srv, "Leader", mates, func(id int32, name string) {
		quests := []string{q611Name}
		if name == "Leader" {
			quests = append(quests, q612Name)
		}
		for _, q := range quests {
			insertJournal(t, srv.DB,
				journalRow{id, q, "<state>", val("STARTED")},
				journalRow{id, q, "<cond>", val("1")},
			)
		}
	})
	held := func() map[string]int {
		out := map[string]int{}
		for name, id := range ids {
			out[name+" badges"] = srv.PlayerItemCount(t, id, ketraBadgeID)
			out[name+" molars"] = srv.PlayerItemCount(t, id, ketraMolarID)
		}
		return out
	}
	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	kill := func(npcID int32, queue ...int) {
		draws.queue(queue...)
		killOnPlayerQueue(t, srv, ids["Leader"], srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(npcID)), location.Location{X: x + 40, Y: y, Z: z}))
	}
	for i, step := range []struct {
		npc   int32
		draws []int
		want  map[string]int
	}{
		{ketraSoldierID, []int{0, 0, 0}, map[string]int{"Leader badges": 0, "Leader molars": 1, "Mate badges": 0, "Mate molars": 0}},
		{ketraSoldierID, []int{1, 0}, map[string]int{"Leader badges": 0, "Leader molars": 1, "Mate badges": 1, "Mate molars": 0}},
		{ketraSoldierID, []int{0, 1, 0}, map[string]int{"Leader badges": 1, "Leader molars": 1, "Mate badges": 1, "Mate molars": 0}},
		{ketraSoldier2ID, []int{0, 0, 0}, map[string]int{"Leader badges": 2, "Leader molars": 1, "Mate badges": 1, "Mate molars": 0}},
	} {
		kill(step.npc, step.draws...)
		if got := held(); !maps.Equal(got, step.want) {
			t.Fatalf("holdings after kill %d = %v, want %v", i+1, got, step.want)
		}
	}
}
