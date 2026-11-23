package quest

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q636"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// rewardSite is a final-reward event of a quest, sent through the NPC npc
// by a player whose quest is in progress with the extra journal vars
// (var=value) and who holds hold.
type rewardSite struct {
	path  string
	npc   int
	vars  []string
	hold  map[int32]int32
	event string
	// pages are more pages of the quest, each with the events it links.
	pages map[string][]string
	// pays, when set, is an item the event gives.
	pays int32
	// ctor, when set, builds a quest the content catalog does not list.
	ctor func() script.Script
	// zone, when set, is a script zone the quest watches, placed far away.
	zone int
}

// rewardWorld boots the site's quest alone, spawns its NPC and talks to
// it, so the NPC is the player's last quest NPC.
func rewardWorld(t *testing.T, site rewardSite) *dialogWorld {
	t.Helper()
	quest := strings.TrimPrefix(site.path, "quest.")
	items := []int32{site.pays}
	for id := range site.hold {
		items = append(items, id)
	}
	ctor := site.ctor
	if ctor == nil {
		ctor = scenarioScripts[site.path]
	}
	if ctor == nil {
		t.Fatalf("no catalog script %s", site.path)
	}
	links := map[string][]string{site.event: nil}
	for file, events := range site.pages {
		links[file] = events
	}
	pages := chatPages(questPages(quest, links), site.npc)
	opts := []gameservertest.Option{
		x1Items(items),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", site.npc)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{int32(site.npc): script.KindFolk},
			[]script.Listing{{Path: site.path}}, script.Catalog{site.path: ctor}),
	}
	if site.zone != 0 {
		form, err := zone.NewCuboid(-1_000_000, -999_000, -1_000_000, -999_000, -10_000, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		zones := zone.NewIndex()
		zones.Add(zone.NewScript(site.zone, form))
		opts = append(opts, gameservertest.WithZones(zones))
	}
	w := bootSeamQuest(t, 76, pages, func(srv *gameservertest.Server, objID int32) {
		rows := []journalRow{{objID, quest, "<state>", val("STARTED")}}
		for _, v := range site.vars {
			k, x, _ := strings.Cut(v, "=")
			rows = append(rows, journalRow{objID, quest, k, val(x)})
		}
		insertJournal(t, srv.DB, rows...)
		for id, n := range site.hold {
			srv.GiveItem(t, objID, id, n)
		}
	}, opts...)
	w.spawnFolk(t, "npc", site.npc, 0, -40, 0)
	w.talk(t, "npc")
	return w
}

// sendOffered offers the site's event on a page and sends it.
func (w *dialogWorld) sendOffered(t *testing.T, site rewardSite) []string {
	t.Helper()
	quest := strings.TrimPrefix(site.path, "quest.")
	w.offerEvent(t, quest, site.event)
	return w.send(t, encodeBypass("Quest "+quest+" "+site.event))
}

// TestFinalRewardsPayOnce: each final reward, sent in the state its page is
// offered in, pays; sent again once the quest is over, it answers nothing
// and pays nothing (#5183).
func TestFinalRewardsPayOnce(t *testing.T) {
	t.Parallel()
	cond := func(n string, more ...string) []string { return append([]string{"<cond>=" + n}, more...) }
	sites := []rewardSite{
		{path: "quest.Q002_WhatWomenWant", npc: 30223, vars: cond("3"), hold: map[int32]int32{1094: 1}, event: "30223-09.htm"},
		{path: "quest.Q006_StepIntoTheFuture", npc: 30006, vars: cond("3"), event: "30006-06.htm"},
		{path: "quest.Q007_ATripBegins", npc: 30146, vars: cond("3"), event: "30146-06.htm"},
		{path: "quest.Q008_AnAdventureBegins", npc: 30134, vars: cond("3"), event: "30134-06.htm"},
		{path: "quest.Q009_IntoTheCityOfHumans", npc: 30576, vars: cond("2"), event: "30576-01.htm"},
		{path: "quest.Q010_IntoTheWorld", npc: 30533, vars: cond("4"), event: "30533-05.htm"},
		{path: "quest.Q011_SecretMeetingWithKetraOrcs", npc: 31371, vars: cond("2"), hold: map[int32]int32{7231: 1}, event: "31371-02.htm"},
		{path: "quest.Q012_SecretMeetingWithVarkaSilenos", npc: 31378, vars: cond("2"), hold: map[int32]int32{7232: 1}, event: "31378-02.htm"},
		{path: "quest.Q013_ParcelDelivery", npc: 31539, vars: cond("1"), hold: map[int32]int32{7263: 1}, event: "31539-1.htm"},
		{path: "quest.Q014_WhereaboutsOfTheArchaeologist", npc: 31538, vars: cond("1"), hold: map[int32]int32{7253: 1}, event: "31538-1.htm"},
		{path: "quest.Q015_SweetWhispers", npc: 31517, vars: cond("2"), event: "31517-01.htm"},
		{path: "quest.Q018_MeetingWithTheGoldenRam", npc: 31555, vars: cond("2"), hold: map[int32]int32{7245: 1}, event: "31555-02.htm"},
		{path: "quest.Q020_BringUpWithLove", npc: 31537, vars: cond("2"), event: "31537-12.htm"},
		{path: "quest.Q024_InhabitantsOfTheForestOfTheDead", npc: 31522, vars: cond("11", "state=14"), event: "31522-21.htm"},
		{path: "quest.Q025_HidingBehindTheTruth", npc: 31522, vars: cond("18", "state=24"), event: "31522-16.htm", pays: 874},
		{path: "quest.Q025_HidingBehindTheTruth", npc: 31532, vars: cond("17", "state=23"), event: "31532-25.htm", pays: 874},
		{path: "quest.Q031_SecretBuriedInTheSwamp", npc: 31555, vars: cond("7"), hold: map[int32]int32{7252: 1}, event: "31555-07.htm"},
		{path: "quest.Q033_MakeAPairOfDressShoes", npc: 30838, vars: cond("5"), event: "30838-7.htm"},
		{path: "quest.Q037_MakeFormalWear", npc: 31520, vars: cond("7"), hold: map[int32]int32{7113: 1}, event: "31520-7.htm"},
		{path: "quest.Q039_RedEyedInvaders", npc: 30332, vars: cond("5"), event: "30332-5.htm"},
		{path: "quest.Q042_HelpTheUncle", npc: 30828, vars: cond("5"), event: "30828-07.htm"},
		{path: "quest.Q043_HelpTheSister", npc: 30829, vars: cond("5"), event: "30829-07.htm"},
		{path: "quest.Q044_HelpTheSon", npc: 30827, vars: cond("5"), event: "30827-07.htm"},
		{path: "quest.Q045_ToTalkingIsland", npc: 30097, vars: cond("6"), hold: map[int32]int32{7566: 1}, event: "30097-12.htm"},
		{path: "quest.Q046_OnceMoreInTheArmsOfTheMotherTree", npc: 30097, vars: cond("6"), hold: map[int32]int32{7566: 1}, event: "30097-12.htm"},
		{path: "quest.Q047_IntoTheDarkForest", npc: 30097, vars: cond("6"), hold: map[int32]int32{7566: 1}, event: "30097-12.htm"},
		{path: "quest.Q048_ToTheImmortalPlateau", npc: 30097, vars: cond("6"), hold: map[int32]int32{7566: 1}, event: "30097-12.htm"},
		{path: "quest.Q049_TheRoadHome", npc: 30097, vars: cond("6"), hold: map[int32]int32{7566: 1}, event: "30097-12.htm"},
		{path: "quest.Q050_LanoscosSpecialBait", npc: 31570, vars: cond("2"), hold: map[int32]int32{7621: 100}, event: "31570-07.htm"},
		{path: "quest.Q051_OFullesSpecialBait", npc: 31572, vars: cond("2"), hold: map[int32]int32{7622: 100}, event: "31572-07.htm"},
		{path: "quest.Q052_WilliesSpecialBait", npc: 31574, vars: cond("2"), hold: map[int32]int32{7623: 100}, event: "31574-07.htm"},
		{path: "quest.Q053_LinnaeusSpecialBait", npc: 31577, vars: cond("2"), hold: map[int32]int32{7624: 100}, event: "31577-07.htm"},
		{path: "quest.Q101_SwordOfSolidarity", npc: 30283, vars: cond("5"), hold: map[int32]int32{739: 1}, event: "30283-06.htm"},
		{path: "quest.Q109_InSearchOfTheNest", npc: 31554, vars: cond("3"), event: "31554-02.htm"},
		{path: "quest.Q111_ElrokianHuntersProof", npc: 32116, vars: cond("12"), hold: map[int32]int32{8773: 1}, event: "32116-007.htm"},
		{path: "quest.Q112_WalkOfFate", npc: 32017, vars: cond("1"), event: "32017-02.htm"},
		{path: "quest.Q113_StatusOfTheBeaconTower", npc: 32016, vars: cond("1"), hold: map[int32]int32{8086: 1}, event: "32016-02.htm"},
		{path: "quest.Q116_BeyondTheHillsOfWinter", npc: 32052, vars: cond("2"), hold: map[int32]int32{8098: 1}, event: "materials"},
		{path: "quest.Q116_BeyondTheHillsOfWinter", npc: 32052, vars: cond("2"), hold: map[int32]int32{8098: 1}, event: "adena"},
		{path: "quest.Q117_TheOceanOfDistantStars", npc: 32054, vars: cond("10"), event: "32054-03.htm"},
		{path: "quest.Q119_LastImperialPrince", npc: 31453, vars: cond("2"), event: "31453-07.htm"},
		{path: "quest.Q120_PavelsResearch", npc: 32041, vars: cond("25"), hold: map[int32]int32{8292: 1}, event: "32041-32.htm"},
		{path: "quest.Q121_PavelTheGiant", npc: 32041, vars: cond("1"), event: "32041-2.htm"},
		{path: "quest.Q122_OminousNews", npc: 32017, vars: cond("1"), event: "32017-02.htm"},
		{path: "quest.Q126_TheNameOfEvil_2", npc: 32114, vars: cond("23"), event: "32114-09.htm"},
		{path: "quest.Q156_MillenniumLove", npc: 30369, vars: cond("1"), hold: map[int32]int32{1022: 1}, event: "30369-03.htm"},
		{path: "quest.Q167_DwarvenKinship", npc: 30255, vars: cond("1"), hold: map[int32]int32{1076: 1}, event: "30255-04.htm"},
		{path: "quest.Q167_DwarvenKinship", npc: 30210, vars: cond("2"), hold: map[int32]int32{1106: 1}, event: "30210-02.htm"},
		{path: "quest.Q216_TrialOfTheGuildsman", npc: 30103, vars: cond("6"), hold: map[int32]int32{3139: 7, 3122: 1}, event: "30103-09a.htm", pays: 3119},
		{path: "quest.Q233_TestOfTheWarSpirit", npc: 30649, vars: cond("5"), event: "30649-03.htm", pays: 2879},
		{path: "quest.Q241_PossessorOfAPreciousSoul", npc: 31740, vars: cond("21"), event: "31740-05.htm"},
		{path: "quest.Q247_PossessorOfAPreciousSoul", npc: 31745, vars: cond("2"), event: "31745-05.htm", pays: 7694},
		{path: "quest.Q636_TruthBeyondTheGate", npc: 32010, vars: cond("1"), event: "32010-02.htm", pays: 8064, ctor: q636.New, zone: templeGateID},
	}
	for _, site := range sites {
		t.Run(strings.TrimPrefix(site.path, "quest.")+"/"+site.event, func(t *testing.T) {
			t.Parallel()
			w := rewardWorld(t, site)
			if got := w.sendOffered(t, site); len(got) == 0 {
				t.Fatal("the reward answered nothing")
			}
			paid := 0
			if site.pays != 0 {
				if paid = w.srv.PlayerItemCount(t, w.player, site.pays); paid == 0 {
					t.Fatalf("the reward paid no %d", site.pays)
				}
			}
			if got := w.sendOffered(t, site); len(got) != 0 {
				t.Fatalf("the reward again = %q, want nothing", got)
			}
			if site.pays != 0 {
				if n := w.srv.PlayerItemCount(t, w.player, site.pays); n != paid {
					t.Fatalf("%d after the reward again = %d, want %d", site.pays, n, paid)
				}
			}
		})
	}
}

// TestFinalRewardsRefuseOffState: a final reward sent on a cond its page is
// not offered at, or without the item it takes, answers nothing and pays
// nothing (#5183).
func TestFinalRewardsRefuseOffState(t *testing.T) {
	t.Parallel()
	sites := []rewardSite{
		{path: "quest.Q216_TrialOfTheGuildsman", npc: 30103, vars: []string{"<cond>=5"}, hold: map[int32]int32{3139: 7, 3122: 1}, event: "30103-09a.htm", pays: 3119},
		{path: "quest.Q216_TrialOfTheGuildsman", npc: 30103, vars: []string{"<cond>=6"}, hold: map[int32]int32{3139: 6, 3122: 1}, event: "30103-09b.htm", pays: 3119},
		{path: "quest.Q233_TestOfTheWarSpirit", npc: 30649, vars: []string{"<cond>=4"}, event: "30649-03.htm", pays: 2879},
		{path: "quest.Q025_HidingBehindTheTruth", npc: 31522, vars: []string{"<cond>=17", "state=23"}, event: "31522-16.htm", pays: 874},
		{path: "quest.Q025_HidingBehindTheTruth", npc: 31532, vars: []string{"<cond>=18", "state=24"}, event: "31532-25.htm", pays: 874},
		{path: "quest.Q247_PossessorOfAPreciousSoul", npc: 31745, vars: []string{"<cond>=1"}, event: "31745-05.htm", pays: 7694},
		{path: "quest.Q644_GraveRobberAnnihilation", npc: 32017, vars: []string{"<cond>=1"}, hold: map[int32]int32{8088: 120}, event: "0", pays: 1865},
		{path: "quest.Q644_GraveRobberAnnihilation", npc: 32017, vars: []string{"<cond>=2"}, hold: map[int32]int32{8088: 120}, event: "6"},
		{path: "quest.Q645_GhostsOfBatur", npc: 32017, vars: []string{"<cond>=1"}, hold: map[int32]int32{8089: 180}, event: "0", pays: 1878},
		{path: "quest.Q646_SignsOfRevolt", npc: 32016, vars: []string{"<cond>=1"}, hold: map[int32]int32{8087: 180}, event: "0", pays: 1880},
		{path: "quest.Q617_GatherTheFlames", npc: 32049, vars: []string{"<cond>=1"}, hold: map[int32]int32{7264: 1200}, event: "57", pays: 57},
		{
			path: "quest.Q234_FatesWhisper", npc: 31002, vars: []string{"<cond>=10", "bypass=1", "weaponId=79"}, hold: map[int32]int32{79: 1},
			pages: map[string][]string{"31002-AGradeList.htm": {"selectAGrade_80"}, "31002-12.htm": nil}, event: "selectAGrade_8", pays: 8,
		},
		{
			path: "quest.Q234_FatesWhisper", npc: 31002, vars: []string{"<cond>=9", "bypass=1", "weaponId=79"}, hold: map[int32]int32{79: 1},
			pages: map[string][]string{"31002-AGradeList.htm": {"selectAGrade_80"}, "31002-12.htm": nil}, event: "selectAGrade_80", pays: 80,
		},
		{path: "quest.Q234_FatesWhisper", npc: 31002, vars: []string{"<cond>=9"}, pages: map[string][]string{"31002-13.htm": nil}, event: "selectBGrade_79"},
	}
	for _, site := range sites {
		t.Run(strings.TrimPrefix(site.path, "quest.")+"/"+strings.Join(site.vars, ",")+"/"+site.event, func(t *testing.T) {
			t.Parallel()
			w := rewardWorld(t, site)
			if got := w.sendOffered(t, site); len(got) != 0 {
				t.Fatalf("the reward = %q, want nothing", got)
			}
			if site.pays != 0 {
				if n := w.srv.PlayerItemCount(t, w.player, site.pays); n != 0 {
					t.Fatalf("%d after the refused reward = %d, want none", site.pays, n)
				}
			}
			for id, n := range site.hold {
				if got := w.srv.PlayerItemCount(t, w.player, id); got != int(n) {
					t.Fatalf("%d after the refused reward = %d, want %d", id, got, n)
				}
			}
		})
	}
}

// TestQ234PicksAnOfferedAGradeWeapon: on cond 10, with the chosen weapon
// confirmed and held, an A-grade weapon the list offers is given for it.
func TestQ234PicksAnOfferedAGradeWeapon(t *testing.T) {
	t.Parallel()
	site := rewardSite{
		path: "quest.Q234_FatesWhisper", npc: 31002, vars: []string{"<cond>=10", "bypass=1", "weaponId=79"}, hold: map[int32]int32{79: 1},
		pages: map[string][]string{"31002-AGradeList.htm": {"selectAGrade_80"}, "31002-12.htm": nil}, event: "selectAGrade_80", pays: 80,
	}
	w := rewardWorld(t, site)
	if got := w.sendOffered(t, site); len(got) == 0 {
		t.Fatal("the pick answered nothing")
	}
	if n, held := w.srv.PlayerItemCount(t, w.player, 80), w.srv.PlayerItemCount(t, w.player, 79); n != 1 || held != 0 {
		t.Fatalf("after the pick: %d of the A-grade weapon and %d of the B-grade one, want 1 and 0", n, held)
	}
}

// TestQ221BoxOpensOnlyForTheKey: on cond 8 the Box of Titan's opening
// takes the Key of Titan for the tablet fragment; without the key it
// answers the box's page for a player without it and gives nothing
// (#5139).
func TestQ221BoxOpensOnlyForTheKey(t *testing.T) {
	t.Parallel()
	const keyOfTitan, fragment = 3030, 3245
	for _, key := range []int32{0, 1} {
		t.Run("key="+strconv.Itoa(int(key)), func(t *testing.T) {
			t.Parallel()
			site := rewardSite{
				path: "quest.Q221_TestimonyOfProsperity", npc: 30622, vars: []string{"<cond>=8"}, hold: map[int32]int32{keyOfTitan: key},
				pages: map[string][]string{"30622-05.htm": nil}, event: "30622-04.htm", pays: fragment,
			}
			if key == 0 {
				site.hold = nil
			}
			w := rewardWorld(t, site)
			got := w.sendOffered(t, site)
			n := w.srv.PlayerItemCount(t, w.player, fragment)
			switch {
			case key == 0 && (!showsText(got, "30622-05.htm") || n != 0):
				t.Fatalf("without the key: %q and %d fragments, want the box's page and none", got, n)
			case key == 1 && (!showsText(got, "30622-04.htm") || n != 1 || w.srv.PlayerItemCount(t, w.player, keyOfTitan) != 0):
				t.Fatalf("with the key: %q and %d fragments, want the opening, one fragment and the key taken", got, n)
			}
		})
	}
}
