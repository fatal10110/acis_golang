package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Monsters of soulCrystals.xml that stage crystals of levels 10 and 11 for
// a whole party (Lilith 25283, chance 100 of 1000, any crystal skill) or for
// one random member of it (Tyrannosaurus 22215, chance 100 of 1000).
const (
	lilithID        = 25283
	tyrannosaurusID = 22215
	partyMonsterLvl = 80
)

// q350PartyMonster is a monster of the given soul crystal id, at a level no
// party member outlevels.
func q350PartyMonster(id int) *npc.Template {
	tmpl := proofMonster(id)
	tmpl.Level = partyMonsterLvl
	return tmpl
}

// q350Party boots Leader and Mate (level 75) and Bystander, who is in the
// party without the quest, each holding a level 10 Red Soul Crystal, one
// party; draws serve the script's random draws.
func q350Party(t *testing.T, draws *scriptDraws, monsterID int32) (srv *gameservertest.Server, ids map[string]int32, at location.Location) {
	t.Helper()
	srv = gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	}, append(q350FixtureFor(t, 0, monsterID), gameservertest.WithScriptRand(draws.rand))...)...)
	ids, _ = formParty(t, srv, "Leader", []partyMate{{"mate", "Mate", 75}, {"bystander", "Bystander", 75}}, func(id int32, name string) {
		if name != "Bystander" {
			seedJournal(t, srv, id, []string{q350Name + ":STARTED"})
		}
		srv.GiveItem(t, id, redLevel10, 1)
	})
	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	return srv, ids, location.Location{X: x + 40, Y: y, Z: z}
}

// staged tells which of the party hold the crystal of the next level.
func staged(t *testing.T, srv *gameservertest.Server, ids map[string]int32) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for name, id := range ids {
		out[name] = srv.PlayerItemCount(t, id, redLevel11) == 1 && srv.PlayerItemCount(t, id, redLevel10) == 0
		if !out[name] && srv.PlayerItemCount(t, id, redLevel10) != 1 {
			t.Fatalf("%s holds %d level 10 and %d level 11 crystals, want one of either", name,
				srv.PlayerItemCount(t, id, redLevel10), srv.PlayerItemCount(t, id, redLevel11))
		}
	}
	return out
}

// TestQ350FullPartyAbsorbStagesEveryStartedMember: a monster that absorbs
// for the full party stages the crystal of every party member who started
// the quest, on the one roll of the death, not only the killer's; a member
// without the quest keeps the crystal.
func TestQ350FullPartyAbsorbStagesEveryStartedMember(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	srv, ids, at := q350Party(t, draws, lilithID)
	draws.queue(50)
	killOnPlayerQueue(t, srv, ids["Leader"], srv.SpawnHostileNPCTemplateAt(t, q350PartyMonster(lilithID), at))
	want := map[string]bool{"Leader": true, "Mate": true, "Bystander": false}
	if got := staged(t, srv, ids); !mapsEqual(got, want) {
		t.Fatalf("crystals staged after the death = %v, want %v", got, want)
	}
	if got := draws.taken(); !slices.Equal(got, []int{1000}) {
		t.Fatalf("draws of the death = %v, want the one roll of 1000", got)
	}
}

// TestQ350PartyOneRandomAbsorbStagesOneMember: a monster that absorbs for
// one random party member stages only the crystal of the member the
// draw picks among those who started the quest, never the bystander's.
func TestQ350PartyOneRandomAbsorbStagesOneMember(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		pick int
		want map[string]bool
	}{
		{"first", 0, map[string]bool{"Leader": true, "Mate": false, "Bystander": false}},
		{"second", 1, map[string]bool{"Leader": false, "Mate": true, "Bystander": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			srv, ids, at := q350Party(t, draws, tyrannosaurusID)
			draws.queue(50, tc.pick)
			killOnPlayerQueue(t, srv, ids["Leader"], srv.SpawnHostileNPCTemplateAt(t, q350PartyMonster(tyrannosaurusID), at))
			if got := staged(t, srv, ids); !mapsEqual(got, tc.want) {
				t.Fatalf("crystals staged after the death = %v, want %v", got, tc.want)
			}
			if got := draws.taken(); !slices.Equal(got, []int{1000, 2}) {
				t.Fatalf("draws of the death = %v, want the roll of 1000 then a pick among 2", got)
			}
		})
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// TestQ350RefusesWhatTheMonsterDoesNotStage: a crystal charged on a
// registered absorber is refused with message 978, and stays, when its level
// is not one the monster stages (the Timak Orc Archer stages levels 0 and 1)
// or when the player stands more than 8 levels above the monster; 8 levels
// above it stages.
func TestQ350RefusesWhatTheMonsterDoesNotStage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		level   int
		crystal int32
		want    []string
		held    int32
	}{
		{"level not staged by the monster", timakLevel, redLevel2, []string{"S SystemMessage id=978"}, redLevel2},
		{"gap of 9 levels", timakLevel + 9, redCrystal, []string{"S SystemMessage id=978"}, redCrystal},
		{"gap of 8 levels", timakLevel + 8, redCrystal, []string{
			itemMessage(serverpackets.SystemMessageS1Disappeared, redCrystal),
			"S SystemMessage id=974",
			itemMessage(serverpackets.SystemMessageEarnedItemS1, redStage1),
			"S PlaySound type=0 file=ItemSound.quest_itemget bind=0 obj=0 loc=0,0,0 delay=0",
		}, redStage1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSeamQuest(t, tc.level, nil, func(srv *gameservertest.Server, objID int32) {
				seedJournal(t, srv, objID, []string{q350Name + ":STARTED"})
				srv.GiveItem(t, objID, tc.crystal, 1)
			}, q350Fixture(t, 50)...)
			mob := w.chargeCrystal(t, heldObject(t, w, tc.crystal), false)
			if ai, ok := mob.Absorber(w.player); !ok || !ai.Registered {
				t.Fatalf("absorber = %+v, %v; want registered", ai, ok)
			}
			lines := w.killAndDie(t, mob, func() bool { return mob.Dead() })
			w.srv.Advance(t, 3500*time.Millisecond)
			lines = append(lines, scriptLines(t, w.srv.ReadQueued(t, w.srv.Client))...)
			if !slices.Equal(lines, tc.want) {
				t.Fatalf("death sent %q, want %q", lines, tc.want)
			}
			if n := w.srv.PlayerItemCount(t, w.player, tc.held); n != 1 {
				t.Fatalf("item %d after the death = %d, want 1", tc.held, n)
			}
		})
	}
}

// TestQ350MasterLinks plays the master's other links on a started quest:
// the green and blue crystal choices of the crystal page give their crystal
// with the middle sound, and the exit link ends the quest.
func TestQ350MasterLinks(t *testing.T) {
	t.Parallel()
	pages := chatPages(questPages(q350Name, map[string][]string{
		"30115-21.htm":   {"30115-08.htm", "30115-exit.htm"},
		"30115-08.htm":   {"30115-09.htm", "30115-10.htm", "30115-11.htm"},
		"30115-exit.htm": nil, "30115-10.htm": nil, "30115-11.htm": nil,
	}), masterID)
	for _, tc := range []struct {
		name, event string
		crystal     int32
	}{
		{"green crystal", "30115-10.htm", greenCrystal},
		{"blue crystal", "30115-11.htm", blueCrystal},
		{"exit", "30115-exit.htm", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSeamQuest(t, timakLevel, pages, func(srv *gameservertest.Server, objID int32) {
				seedJournal(t, srv, objID, []string{q350Name + ":STARTED"})
			}, q350Fixture(t, 50)...)
			master := w.spawnFolk(t, "master", masterID, 0, 30, 0).ObjectID()
			if got := w.questWindow(t, master); !shows(got, "30115-21.htm") {
				t.Fatalf("master's window without a crystal = %q, want 30115-21.htm", got)
			}
			if tc.crystal != 0 {
				if got := w.bypass(t, "Quest "+q350Name+" 30115-08.htm"); !shows(got, "30115-08.htm") {
					t.Fatalf("crystal page = %q, want 30115-08.htm", got)
				}
			}
			got := w.bypass(t, "Quest "+q350Name+" "+tc.event)
			if !shows(got, tc.event) {
				t.Fatalf("%s = %q, want its page", tc.event, got)
			}
			if tc.crystal == 0 {
				if vars := w.srv.QuestVars(t, w.player, q350Name); vars != nil {
					t.Fatalf("Q350 after the exit = %v, want no state", vars)
				}
				return
			}
			if !slices.Contains(got, itemMessage(serverpackets.SystemMessageEarnedItemS1, tc.crystal)) {
				t.Fatalf("%s sent %q, want the crystal %d", tc.event, got, tc.crystal)
			}
			if n := w.srv.PlayerItemCount(t, w.player, tc.crystal); n != 1 {
				t.Fatalf("crystal %d held = %d, want 1", tc.crystal, n)
			}
		})
	}
}
