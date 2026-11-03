package quest

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q646"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q651"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the scenarios of the second proof-quest group cannot show: the
// 180th cursed doll of Signs Of Revolt (Q646) going to a party member at
// random, its reward, and Ivan of Runaway Youth (Q651) moving three
// seconds after he gets his scroll.

const (
	q646Name       = "Q646_SignsOfRevolt"
	q646Path       = "quest.Q646_SignsOfRevolt"
	torrantID      = 32016
	ragnaOrcID     = 22029
	cursedDollID   = 8087
	q651Name       = "Q651_RunawayYouth"
	q651Path       = "quest.Q651_RunawayYouth"
	ivanID         = 32014
	scrollEscapeID = 736
)

// proofQuests2Items is the shared item catalog plus the cursed doll and
// the Scroll of Escape.
func proofQuests2Items() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{cursedDollID, scrollEscapeID} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

func q646Scripts() gameservertest.Option {
	return gameservertest.WithNPCScripts(
		map[int32]script.NPCKind{torrantID: script.KindFolk, ragnaOrcID: script.KindHostile},
		[]script.Listing{{Path: q646Path}},
		script.Catalog{q646Path: q646.New},
	)
}

// TestQ646DollGoesToARandomPartyMember: Leader and Mate, in one party,
// both at cond 1, Mate holding 179 dolls. Leader kills a ragna; the member
// draw picks Mate, who gets the 180th doll and moves to cond 2. Leader
// gets nothing (Q646_SignsOfRevolt.onMyDying, Quest.getRandomPartyMember,
// Quest.dropItems).
func TestQ646DollGoesToARandomPartyMember(t *testing.T) {
	t.Parallel()
	rs := &rolls{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithScriptRand(rs.intn),
		q646Scripts(),
		proofQuests2Items(),
	)
	leader := srv.SoleObjectID(t)
	mate := srv.SeedCharacterFor(t, "mate", "Mate", 40, 0).ID
	for _, id := range []int32{leader, mate} {
		insertJournal(t, srv.DB,
			journalRow{id, q646Name, "<state>", val("STARTED")},
			journalRow{id, q646Name, "<cond>", val("1")},
		)
	}
	srv.GiveItem(t, mate, cursedDollID, 179)
	enterWorld(t, srv)
	mateClient := srv.DialClient(t, "mate", 1)
	mateClient.Send(encodeRequestGameStart(0))
	readUntil(t, mateClient, serverpackets.OpcodeCharSelected)
	mateClient.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, mateClient)
	srv.ReadQueued(t, srv.Client)
	srv.Client.Send(encodeJoinParty("Mate"))
	srv.ReadQueued(t, mateClient)
	mateClient.Send(encodeAnswerJoinParty(1))
	srv.ReadQueued(t, mateClient)
	srv.ReadQueued(t, srv.Client)

	x, y, z := srv.PlayerPosition(t, leader)
	ragna := srv.SpawnHostileNPCTemplateAt(t, proofMonster(ragnaOrcID), location.Location{X: x + 40, Y: y, Z: z})
	rs.script([]int{1, 0})
	killOnPlayerQueue(t, srv, leader, ragna)

	if got := rs.taken(); !slices.Equal(got, []string{"2:1", "1000000:0"}) {
		t.Fatalf("draws = %v, want the member draw of two, then the drop roll", got)
	}
	if n := srv.PlayerItemCount(t, mate, cursedDollID); n != 180 {
		t.Fatalf("Mate's dolls = %d, want 180", n)
	}
	if n := srv.PlayerItemCount(t, leader, cursedDollID); n != 0 {
		t.Fatalf("Leader's dolls = %d, want 0", n)
	}
	if vars := srv.QuestVars(t, mate, q646Name); vars["<cond>"] != "2" {
		t.Fatalf("Mate's Q646 = %v, want cond 2", vars)
	}
	if vars := srv.QuestVars(t, leader, q646Name); vars["<cond>"] != "1" {
		t.Fatalf("Leader's Q646 = %v, want cond 1", vars)
	}
	// Mate's 180th doll completes the count: the middle sound, not the
	// item-get one.
	var sounds []string
	for _, line := range scriptLines(t, srv.ReadQueued(t, mateClient)) {
		if len(line) > 11 && line[:11] == "S PlaySound" {
			sounds = append(sounds, line)
		}
	}
	if want := []string{"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0"}; !slices.Equal(sounds, want) {
		t.Fatalf("Mate's sounds = %q, want %q", sounds, want)
	}
}

// TestQ646TorrantPaysTheChosenReward: at cond 2 with 180 dolls, Torrant
// takes every doll for the adena reward the player picks, and the
// repeatable quest ends (Q646_SignsOfRevolt.onAdvEvent).
func TestQ646TorrantPaysTheChosenReward(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 40, chatPages(questPages(q646Name, map[string][]string{
		"32016-05.htm": {"32016-06.htm"}, "32016-06.htm": {"0", "1", "2", "3"}, "32016-07.htm": nil,
	}), torrantID), func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q646Name, "<state>", val("STARTED")},
			journalRow{objID, q646Name, "<cond>", val("2")},
		)
		srv.GiveItem(t, objID, cursedDollID, 180)
	}, q646Scripts(), proofQuests2Items())
	torrant := w.spawnFolk(t, "torrant", torrantID, 0, 30, 0).ObjectID()

	if got := w.questWindow(t, torrant); !shows(got, "32016-05.htm") {
		t.Fatalf("Torrant's window = %q, want 32016-05.htm", got)
	}
	if got := w.bypass(t, "Quest "+q646Name+" 32016-06.htm"); !shows(got, "32016-06.htm") {
		t.Fatalf("the dolls link = %q, want 32016-06.htm", got)
	}
	got := w.bypass(t, "Quest "+q646Name+" 3")
	want := []string{
		"S SystemMessage id=301 3:8087 6:180",
		"S SystemMessage id=52 6:21600",
		"S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0",
		"S QuestList",
	}
	if len(got) < len(want) || !slices.Equal(got[:len(want)], want) || !shows(got, "32016-07.htm") {
		t.Fatalf("reward 3 = %q, want it to start %q and show 32016-07.htm", got, want)
	}
	if n := w.srv.PlayerItemCount(t, w.player, cursedDollID); n != 0 {
		t.Fatalf("dolls after the reward = %d, want 0", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, item.AdenaID); n != 21600 {
		t.Fatalf("adena after the reward = %d, want 21600", n)
	}
	if vars := w.srv.QuestVars(t, w.player, q646Name); len(vars) != 0 {
		t.Fatalf("Q646 after the reward = %v, want no state", vars)
	}
}

// TestQ651IvanMovesAfterTheScroll: Ivan takes the player's Scroll of
// Escape and starts the quest; three seconds later he leaves the world and
// stands up at one of his three spots other than the last one he moved
// to, drawn again while it is the same (Q651_RunawayYouth.onTimer). The
// first draw, 0, is the last spot of a fresh script, so it is drawn again.
func TestQ651IvanMovesAfterTheScroll(t *testing.T) {
	t.Parallel()
	rs := &rolls{}
	w := bootSeamQuest(t, 26, chatPages(questPages(q651Name, map[string][]string{
		"32014-02.htm": {"32014-04.htm", "32014-02a.htm"}, "32014-03.htm": nil,
	}), ivanID), func(srv *gameservertest.Server, objID int32) {
		srv.GiveItem(t, objID, scrollEscapeID, 2)
	},
		gameservertest.WithScriptRand(rs.intn),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", ivanID)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{ivanID: script.KindFolk},
			[]script.Listing{{Path: q651Path}},
			script.Catalog{q651Path: q651.New},
		),
		proofQuests2Items(),
	)
	ivan := w.spawnFolk(t, "ivan", ivanID, 0, 30, 0).ObjectID()

	if got := w.questWindow(t, ivan); !shows(got, "32014-02.htm") {
		t.Fatalf("Ivan's window = %q, want 32014-02.htm", got)
	}
	got := w.bypass(t, "Quest "+q651Name+" 32014-04.htm")
	if !shows(got, "32014-03.htm") {
		t.Fatalf("the scroll link = %q, want 32014-03.htm", got)
	}
	if n := w.srv.PlayerItemCount(t, w.player, scrollEscapeID); n != 1 {
		t.Fatalf("scrolls after the offer = %d, want 1", n)
	}
	if vars := w.srv.QuestVars(t, w.player, q651Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "1" {
		t.Fatalf("Q651 after the offer = %v, want started at cond 1", vars)
	}

	rs.script([]int{0, 2})
	w.srv.Advance(t, 2900*time.Millisecond)
	if left := npcsOf(w.srv, ivanID); len(left) != 1 || left[0].ObjectID() != ivan {
		t.Fatalf("Ivans before three seconds = %d, want the one talked to", len(left))
	}
	w.srv.Advance(t, 200*time.Millisecond)
	if got := rs.taken(); !slices.Equal(got, []string{"3:0", "3:2"}) {
		t.Fatalf("draws = %v, want 0 drawn again, then 2", got)
	}
	left := npcsOf(w.srv, ivanID)
	if len(left) != 1 || left[0].ObjectID() == ivan {
		t.Fatalf("Ivans after three seconds = %d (the one talked to still there: %v), want one new", len(left), len(left) > 0 && left[0].ObjectID() == ivan)
	}
	// Spot 2, on the ground below it: the spawner drops a script spawn
	// from 20 above, and with no geodata the ground is the spot itself.
	if at := left[0].SpawnLocation(); at.X != 123254 || at.Y != -148126 {
		t.Fatalf("Ivan stands at %+v, want spot 2 (123254, -148126)", at)
	}
}
