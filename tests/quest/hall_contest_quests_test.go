package quest

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q504"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q655"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The flows of Competition for the Bandit Stronghold (Q504) and A Grand
// Plan for Taming Wild Beasts (Q655) that the q504_* and q655_* scenarios do
// not play: the Tarlk Bugbear's amulet drop (a kill on the player's queue),
// and the Wild Beast messenger's start event once the hall no longer waits.
// Each boots the quest alone over its shipped pages and the shipped clan
// halls, the two halls' next siege at hallContestSiegeAt.

const (
	q504Name = "Q504_CompetitionForTheBanditStronghold"
	q504Path = "quest." + q504Name
	q655Name = "Q655_AGrandPlanForTamingWildBeasts"
	q655Path = "quest." + q655Name

	banditStronghold   = 35
	banditMessenger    = 35437
	tarlkBugbear       = 20570
	tarlkAmulet        = 4332
	contestCertificate = 4333
	trophyOfAlliance   = 5009

	beastFarmMessenger = 35627
	crystalOfPurity    = 8084
	trainerLicense     = 8293

	// hallContestClan is the clan of the setups; hallContestChief leads it
	// when the Seeker does not.
	hallContestClan  = 7101
	hallContestChief = 900101
	// hallContestSiegeAt is the two halls' stored next siege date
	// (2100-01-01 00:00 UTC).
	hallContestSiegeAt = int64(4102444800000)

	beastFarmRegisterPage = "script/siegablehall/WildBeastReserve/farm_kel_mahum_messenger_1.htm"
)

// hallContestSetup is the Seeker's clan: none at level 0; led by the Seeker
// or by the Chief; owning hall, none at 0.
type hallContestSetup struct {
	clanLevel int
	member    bool
	hall      int32
}

// seed returns the clan seed statements of the setup, with the rows of the
// two contested halls.
func (s hallContestSetup) seed() []string {
	owner := func(id int32) string {
		if s.clanLevel > 0 && s.hall == id {
			return strconv.Itoa(hallContestClan)
		}
		return "0"
	}
	qs := []string{
		"INSERT INTO clanhall (id, ownerId, paid, paidUntil, endDate) VALUES (35, " + owner(banditStronghold) + ", 1, 4102444800000, " + strconv.FormatInt(hallContestSiegeAt, 10) + ")",
		"INSERT INTO clanhall (id, ownerId, paid, paidUntil, endDate) VALUES (63, " + owner(beastFarm) + ", 1, 4102444800000, " + strconv.FormatInt(hallContestSiegeAt, 10) + ")",
	}
	if s.clanLevel == 0 {
		return qs
	}
	clan := strconv.Itoa(hallContestClan)
	qs = append(qs, "UPDATE characters SET clanid = "+clan+" WHERE char_name = 'Seeker'")
	if s.member {
		qs = append(qs,
			`DROP TEMPORARY TABLE IF EXISTS chief`,
			`CREATE TEMPORARY TABLE chief SELECT * FROM characters WHERE char_name = 'Seeker'`,
			"UPDATE chief SET obj_Id = "+strconv.Itoa(hallContestChief)+", account_name = 'chief', char_name = 'Chief'",
			`INSERT INTO characters SELECT * FROM chief`,
			`DROP TEMPORARY TABLE chief`,
			"INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES ("+clan+", 'Seekers', "+strconv.Itoa(s.clanLevel)+", 0, "+strconv.Itoa(hallContestChief)+")",
		)
	} else {
		qs = append(qs, "INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT "+clan+", 'Seekers', "+strconv.Itoa(s.clanLevel)+", 0, obj_Id FROM characters WHERE char_name = 'Seeker'")
	}
	if s.hall != 0 && s.hall != banditStronghold && s.hall != beastFarm {
		qs = append(qs, "INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES ("+strconv.Itoa(int(s.hall))+", "+clan+", 1, 4102444800000)")
	}
	return qs
}

// bootHallContest boots the quest path built by ctor with its messenger
// beside the Seeker, the setup's clan and the shipped clan halls, hall's
// siege waiting for its battle when waiting; before seeds what precedes
// the selection. Every script draw is 0.
func bootHallContest(t *testing.T, path string, ctor func() script.Script, messenger, hall int32, setup hallContestSetup, waiting bool,
	before func(srv *gameservertest.Server, objID int32),
) *dialogWorld {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatal(err)
	}
	w := bootX1Quest(t, 40, path, ctor, []int32{messenger}, []int32{tarlkBugbear}, before,
		x1Items([]int32{tarlkAmulet, contestCertificate, trophyOfAlliance, crystalOfPurity, trainerLicense}),
		gameservertest.WithHTMLPages(map[string]string{beastFarmRegisterPage: datapackPage(t, beastFarmRegisterPage)}),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		clanSeed(t, setup.seed()...),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
	)
	if waiting {
		hallSiegeOf(t, w, hall).SetStatus(siege.StatusRegistrationOver)
	}
	w.spawnFolk(t, "messenger", int(messenger), 10, 0, 0)
	return w
}

// hallSiegeOf returns siegable hall id's siege.
func hallSiegeOf(t *testing.T, w *dialogWorld, id int32) interface{ SetStatus(siege.Status) } {
	t.Helper()
	hs, ok := w.srv.Halls.Siege(id)
	if !ok {
		t.Fatalf("hall %d has no siege", id)
	}
	return hs
}

// datedPage is the rendered shipped quest page file shown by the messenger,
// its placeholder filled with the stored next siege date as the pages
// print it.
func datedPage(t *testing.T, quest, file, placeholder string) string {
	t.Helper()
	date := time.UnixMilli(hallContestSiegeAt).Format("2006-01-02 15:04:05")
	page := datapackPage(t, "script/quest/"+quest+"/"+file)
	if !strings.Contains(page, placeholder) {
		t.Fatalf("%s has no %s", file, placeholder)
	}
	return "S NpcHtmlMessage obj=messenger item=0 html=" + scriptcontract.Quote(strings.ReplaceAll(page, placeholder, date))
}

// give has the quest named quest give the player count of itemID.
func (w *dialogWorld) give(t *testing.T, quest string, itemID, count int32) {
	t.Helper()
	w.srv.RunScript(t, w.player, quest, func(s *script.Script, p *script.Player) { s.GiveItems(p, itemID, count) })
	w.srv.ReadQueued(t, w.srv.Client)
}

func wantLine(t *testing.T, what string, got []string, want string) {
	t.Helper()
	if !slices.Contains(got, want) {
		t.Fatalf("%s = %q, want %s", what, got, want)
	}
}

// TestQ504AmuletsForTheTrophy plays the quest for a level 4 clan's
// leader: the start gives the contest certificate, a Tarlk Bugbear drops an
// amulet (the draw 0 is under its 60% chance), and thirty amulets trade,
// with the certificate, for the Trophy of Alliance, ending the repeatable
// quest. The trophy then turns the leader away.
func TestQ504AmuletsForTheTrophy(t *testing.T) {
	t.Parallel()
	w := bootHallContest(t, q504Path, q504.New, banditMessenger, banditStronghold, hallContestSetup{clanLevel: 4}, true, nil)
	wantLine(t, "the offer", w.talk(t, "messenger"), w.page(t, "messenger", q504Name, "35437-01.htm"))

	got := w.send(t, encodeBypass("Quest "+q504Name+" 35437-02.htm"))
	wantLine(t, "the start", got, w.page(t, "messenger", q504Name, "35437-02.htm"))
	wantLine(t, "the start", got, "S PlaySound type=0 file=ItemSound.quest_accept bind=0 obj=0 loc=0,0,0 delay=0")
	if vars := w.srv.QuestVars(t, w.player, q504Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "1" {
		t.Fatalf("Q504 after the start = %v, want started at cond 1", vars)
	}
	if n := w.srv.PlayerItemCount(t, w.player, contestCertificate); n != 1 {
		t.Fatalf("certificates after the start = %d, want 1", n)
	}
	wantLine(t, "no amulets", w.talk(t, "messenger"), w.page(t, "messenger", q504Name, "35437-07.htm"))

	mob := w.spawnHostile(t, "bugbear", tarlkBugbear, "Monster", 30)
	w.killAndDie(t, mob, func() bool { return w.srv.PlayerItemCount(t, w.player, tarlkAmulet) == 1 })
	w.give(t, q504Name, tarlkAmulet, 29)

	got = w.talk(t, "messenger")
	wantLine(t, "thirty amulets", got, w.page(t, "messenger", q504Name, "35437-08.htm"))
	wantLine(t, "thirty amulets", got, "S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0")
	for id, n := range map[int32]int{tarlkAmulet: 0, contestCertificate: 0, trophyOfAlliance: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the trade = %d, want %d", id, c, n)
		}
	}
	if vars := w.srv.QuestVars(t, w.player, q504Name); vars["<state>"] == "STARTED" {
		t.Fatalf("Q504 after the trade = %v, want it ended", vars)
	}
	wantLine(t, "the trophy holder", w.talk(t, "messenger"), w.page(t, "messenger", q504Name, "35437-07a.htm"))
}

// TestQ655MessengerWaitsForTheSiege: until the Wild Beast Reserve waits for
// its battle, the messenger answers a clan's leader with the next siege's
// date. The start's event, sent once the hall no longer waits, starts
// nothing. A player in no clan gets no quest.
func TestQ655MessengerWaitsForTheSiege(t *testing.T) {
	t.Parallel()
	w := bootHallContest(t, q655Path, q655.New, beastFarmMessenger, beastFarm, hallContestSetup{clanLevel: 4}, false, nil)
	wantLine(t, "the messenger before the registrations close", w.talk(t, "messenger"), datedPage(t, q655Name, "35627-02.htm", "%next_siege%"))

	hallSiegeOf(t, w, beastFarm).SetStatus(siege.StatusRegistrationOver)
	wantLine(t, "the offer", w.talk(t, "messenger"), w.page(t, "messenger", q655Name, "35627-01.htm"))
	hallSiegeOf(t, w, beastFarm).SetStatus(siege.StatusRegistrationOpened)
	wantLine(t, "the start once the hall no longer waits", w.send(t, encodeBypass("Quest "+q655Name+" 35627-06.htm")),
		w.page(t, "messenger", q655Name, "35627-06.htm"))
	if vars := w.srv.QuestVars(t, w.player, q655Name); vars["<state>"] != "CREATED" {
		t.Fatalf("Q655 after the start before the registrations close = %v, want created", vars)
	}

	t.Run("no clan", func(t *testing.T) {
		t.Parallel()
		w := bootHallContest(t, q655Path, q655.New, beastFarmMessenger, beastFarm, hallContestSetup{}, true, nil)
		wantLine(t, "no clan", w.talk(t, "messenger"), "S NpcHtmlMessage obj=messenger item=0 html="+scriptcontract.Quote(script.NoQuestMsg()))
	})
}
