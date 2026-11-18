package quest

import (
	"maps"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q354"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q355"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Conquest of Alligator Island (Q354) and Family Honor (Q355)
// scenarios cannot show, since a scenario seeds a party member no quest row:
// the drop of a monster killed by a party going to one random member of it
// who started the quest, never one who did not
// (Q354_ConquestOfAlligatorIsland.onMyDying, Q355_FamilyHonor.onMyDying:
// getRandomPartyMemberState). The drops of one character are
// q354_conquest_of_alligator_island*.scenario and q355_family_honor*.scenario.
const (
	q354QuestName = "Q354_ConquestOfAlligatorIsland"
	q354Path      = "quest." + q354QuestName
	q355QuestName = "Q355_FamilyHonor"
	q355Path      = "quest." + q355QuestName

	crokianLadID   = 20804
	alligatorTooth = 5863
	tornMapFrag    = 5864
	timakLeaderID  = 20767
	galibredoBust  = 4252
	workOfBerona   = 4350
)

// randomPartyQuestWorld is Leader and Mate, who started the quest, and
// Bystander, who did not, in one party beside the monster the quest's drops
// come from.
func randomPartyQuestWorld(t *testing.T, name, path string, newQuest func() script.Script, monster int32, items ...int32) (srv *gameservertest.Server, ids map[string]int32, draws *scriptDraws, kill func(pick int, rolls ...int)) {
	t.Helper()
	draws = &scriptDraws{}
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{monster: script.KindHostile},
			[]script.Listing{{Path: path}},
			script.Catalog{path: newQuest},
		),
		varkaItemTemplates(items...),
	)
	ids, _ = formParty(t, srv, "Leader", []partyMate{{"mate", "Mate", 75}, {"bystander", "Bystander", 75}}, func(id int32, name2 string) {
		if name2 != "Bystander" {
			insertJournal(t, srv.DB,
				journalRow{id, name, "<state>", val("STARTED")},
				journalRow{id, name, "<cond>", val("1")},
			)
		}
	})
	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	kill = func(pick int, rolls ...int) {
		draws.queue(append([]int{pick}, rolls...)...)
		killOnPlayerQueue(t, srv, ids["Leader"], srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(monster)), location.Location{X: x + 40, Y: y, Z: z}))
	}
	return srv, ids, draws, kill
}

// held counts the items of every party member.
func held(t *testing.T, srv *gameservertest.Server, ids map[string]int32, items ...int32) map[string]int {
	t.Helper()
	out := map[string]int{}
	for name, id := range ids {
		for _, it := range items {
			out[name+" "+itemKey(it)] = srv.PlayerItemCount(t, id, it)
		}
	}
	return out
}

func itemKey(id int32) string {
	switch id {
	case alligatorTooth:
		return "teeth"
	case tornMapFrag:
		return "fragments"
	case galibredoBust:
		return "busts"
	case workOfBerona:
		return "works"
	}
	return "items"
}

// TestQ354CrokianDropGoesToOneRandomStartedMember: a Crokian Lad killed by
// the party drops its tooth and fragment, every draw succeeding, to the one
// started member the pick names, whose draw is among two (Leader and Mate),
// never to Bystander.
func TestQ354CrokianDropGoesToOneRandomStartedMember(t *testing.T) {
	t.Parallel()
	srv, ids, draws, kill := randomPartyQuestWorld(t, q354QuestName, q354Path, q354.New, crokianLadID, alligatorTooth, tornMapFrag)
	for i, step := range []struct {
		pick int
		want map[string]int
	}{
		{0, map[string]int{"Leader teeth": 1, "Leader fragments": 1, "Mate teeth": 0, "Mate fragments": 0}},
		{1, map[string]int{"Leader teeth": 1, "Leader fragments": 1, "Mate teeth": 1, "Mate fragments": 1}},
	} {
		kill(step.pick)
		want := map[string]int{"Bystander teeth": 0, "Bystander fragments": 0}
		maps.Copy(want, step.want)
		if got := held(t, srv, ids, alligatorTooth, tornMapFrag); !maps.Equal(got, want) {
			t.Fatalf("holdings after kill %d = %v, want %v", i+1, got, want)
		}
		if got := draws.taken(); len(got) == 0 || got[0] != 2 {
			t.Fatalf("draws of kill %d = %v, want the pick among 2 first", i+1, got)
		}
	}
}

// TestQ355TimakDropGoesToOneRandomStartedMember: a Timak Orc Troop Leader
// (bust below 44, work of Berona below 54, of a roll of 100) drops to the
// started member the pick names, never to Bystander.
//
//	pick Mate, roll 0:     Mate gets a bust
//	pick Leader, roll 50:  Leader gets a work of Berona
//	pick Mate, roll 99:    nothing
func TestQ355TimakDropGoesToOneRandomStartedMember(t *testing.T) {
	t.Parallel()
	srv, ids, draws, kill := randomPartyQuestWorld(t, q355QuestName, q355Path, q355.New, timakLeaderID, galibredoBust, workOfBerona)
	for i, step := range []struct {
		pick, roll int
		want       map[string]int
	}{
		{1, 0, map[string]int{"Leader busts": 0, "Leader works": 0, "Mate busts": 1, "Mate works": 0}},
		{0, 50, map[string]int{"Leader busts": 0, "Leader works": 1, "Mate busts": 1, "Mate works": 0}},
		{1, 99, map[string]int{"Leader busts": 0, "Leader works": 1, "Mate busts": 1, "Mate works": 0}},
	} {
		kill(step.pick, step.roll)
		want := map[string]int{"Bystander busts": 0, "Bystander works": 0}
		maps.Copy(want, step.want)
		if got := held(t, srv, ids, galibredoBust, workOfBerona); !maps.Equal(got, want) {
			t.Fatalf("holdings after kill %d = %v, want %v", i+1, got, want)
		}
		if got := draws.taken(); len(got) < 2 || got[0] != 2 || got[1] != 100 {
			t.Fatalf("draws of kill %d = %v, want a pick among 2 then a roll of 100", i+1, got)
		}
	}
}
