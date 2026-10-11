package quest

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q607"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// What the Prove Your Courage (Q607) scenario cannot show, since a
// scenario seeds neither the Ketra standing nor an item: Kadun's offer to
// a Ketra ally, the head of Shadith going to every ally of the killer's
// party and only to them, and the head traded for the totem.

const (
	q607Name         = "Q607_ProveYourCourage"
	q607Path         = "quest.Q607_ProveYourCourage"
	kadunID          = 31370
	shadithID        = 25309
	headOfShadithID  = 7235
	totemOfValorID   = 7219
	ketraAlliance3ID = 7213
)

// q607Items is the shared item catalog plus the head, the totem and the
// alliance mark.
func q607Items() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{headOfShadithID, totemOfValorID, ketraAlliance3ID} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

func q607Scripts() gameservertest.Option {
	return gameservertest.WithNPCScripts(
		map[int32]script.NPCKind{kadunID: script.KindFolk, shadithID: script.KindHostile},
		[]script.Listing{{Path: q607Path}},
		script.Catalog{q607Path: q607.New},
	)
}

// q607LevelExp is the experience each level adds in q607Levels.
const q607LevelExp = 1_000_000

// q607Levels reaches level L at (L-1) x q607LevelExp experience, so the
// 10000 the totem pays keeps a level 75 character at 75.
func q607Levels(t *testing.T) *player.LevelTable {
	t.Helper()
	levels := make(map[int]player.Level, 80)
	for lvl := 1; lvl <= 80; lvl++ {
		levels[lvl] = player.Level{RequiredExpToLevelUp: int64(lvl-1) * q607LevelExp}
	}
	table, err := player.NewLevelTable(levels)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// seedStanding stores objID's Ketra/Varka standing before it enters the
// world.
func seedStanding(t *testing.T, srv *gameservertest.Server, objID int32, standing int) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET varka_ketra_ally = ? WHERE obj_Id = ?`, standing, objID); err != nil {
		t.Fatalf("seed varka_ketra_ally of %d: %v", objID, err)
	}
}

// questLines keeps the quest sound and item messages of lines: the
// rendered PlaySound packets and the item earned message.
func questLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, "S PlaySound") || strings.HasPrefix(l, itemMessage(54, headOfShadithID)) {
			out = append(out, l)
		}
	}
	return out
}

// TestQ607ShadithCreditsEveryKetraAllyOfTheParty: Leader (Ketra 3, mark),
// Mate (Ketra 2, mark), Third (Ketra 4, mark) and Fourth (Ketra 5, no mark)
// are one party, all at cond 1 beside Shadith. Leader kills him: Leader and
// Third, the allies of level 3 or more holding the mark, each get a head
// and move to cond 2, with the middle sound before the head; Mate and
// Fourth get nothing (Q607_ProveYourCourage.onMyDying,
// Quest.getPartyMembers).
func TestQ607ShadithCreditsEveryKetraAllyOfTheParty(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		q607Scripts(),
		q607Items(),
	)
	leader := srv.SoleObjectID(t)
	mate := srv.SeedCharacterFor(t, "mate", "Mate", 75, 0).ID
	third := srv.SeedCharacterFor(t, "third", "Third", 75, 0).ID
	fourth := srv.SeedCharacterFor(t, "fourth", "Fourth", 75, 0).ID
	for _, m := range []struct {
		id       int32
		standing int
		mark     bool
	}{{leader, 3, true}, {mate, 2, true}, {third, 4, true}, {fourth, 5, false}} {
		insertJournal(t, srv.DB,
			journalRow{m.id, q607Name, "<state>", val("STARTED")},
			journalRow{m.id, q607Name, "<cond>", val("1")},
		)
		seedStanding(t, srv, m.id, m.standing)
		if m.mark {
			srv.GiveItem(t, m.id, ketraAlliance3ID, 1)
		}
	}
	enterWorld(t, srv)
	members := map[int32]*testsupport.ScriptedClient{leader: srv.Client}
	for _, m := range []struct {
		id            int32
		account, name string
	}{{mate, "mate", "Mate"}, {third, "third", "Third"}, {fourth, "fourth", "Fourth"}} {
		c := srv.DialClient(t, m.account, 1)
		c.Send(encodeRequestGameStart(0))
		readUntil(t, c, serverpackets.OpcodeCharSelected)
		c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
		srv.Client.Send(encodeJoinParty(m.name))
		srv.ReadQueued(t, c)
		c.Send(encodeAnswerJoinParty(1))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
		members[m.id] = c
	}
	for _, c := range members {
		srv.ReadQueued(t, c)
	}

	x, y, z := srv.PlayerPosition(t, leader)
	shadith := srv.SpawnHostileNPCTemplateAt(t, proofMonster(shadithID), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, srv, leader, shadith)

	credited := []string{
		"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0",
		itemMessage(54, headOfShadithID),
	}
	want := map[int32]struct {
		cond  string
		heads int
		lines []string
	}{
		leader: {"2", 1, credited},
		mate:   {"1", 0, nil},
		third:  {"2", 1, credited},
		fourth: {"1", 0, nil},
	}
	names := map[int32]string{leader: "Leader", mate: "Mate", third: "Third", fourth: "Fourth"}
	for id, w := range want {
		if got := questLines(scriptLines(t, srv.ReadQueued(t, members[id]))); !slices.Equal(got, w.lines) {
			t.Errorf("%s's quest sounds and messages = %q, want %q", names[id], got, w.lines)
		}
		if n := srv.PlayerItemCount(t, id, headOfShadithID); n != w.heads {
			t.Errorf("%s's heads = %d, want %d", names[id], n, w.heads)
		}
		if vars := srv.QuestVars(t, id, q607Name); vars["<cond>"] != w.cond {
			t.Errorf("%s's Q607 = %v, want cond %s", names[id], vars, w.cond)
		}
	}
}

// TestQ607KadunTradesTheHeadForTheTotem: a level 75 Ketra ally of level 3
// holding the mark is offered the quest, takes it, kills Shadith alone,
// and trades the head for the Totem of Valor, 10000 experience and the end
// of the repeatable quest. Holding the totem, Kadun no longer offers it
// (Q607_ProveYourCourage.onTalk, onAdvEvent).
func TestQ607KadunTradesTheHeadForTheTotem(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 75, chatPages(questPages(q607Name, map[string][]string{
		"31370-01.htm": {"31370-04.htm"}, "31370-02.htm": nil, "31370-04.htm": nil,
		"31370-05.htm": {"31370-07.htm"}, "31370-06.htm": nil, "31370-07.htm": nil,
	}), kadunID), func(srv *gameservertest.Server, objID int32) {
		seedStanding(t, srv, objID, 3)
		srv.GiveItem(t, objID, ketraAlliance3ID, 1)
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET exp = ? WHERE obj_Id = ?", int64(74)*q607LevelExp, objID); err != nil {
			t.Fatalf("seed experience: %v", err)
		}
	}, q607Scripts(), q607Items(), gameservertest.WithLevels(q607Levels(t)))
	kadun := w.spawnFolk(t, "kadun", kadunID, 0, 30, 0).ObjectID()

	if got := w.questWindow(t, kadun); !shows(got, "31370-01.htm") {
		t.Fatalf("Kadun's window = %q, want 31370-01.htm", got)
	}
	if got := w.bypass(t, "Quest "+q607Name+" 31370-04.htm"); !shows(got, "31370-04.htm") {
		t.Fatalf("the accept link = %q, want 31370-04.htm", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q607Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "1" {
		t.Fatalf("Q607 after the accept = %v, want started at cond 1", vars)
	}

	shadith := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(shadithID), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	killOnPlayerQueue(t, w.srv, w.player, shadith)
	w.srv.ReadQueued(t, w.srv.Client)
	if vars := w.srv.QuestVars(t, w.player, q607Name); vars["<cond>"] != "2" {
		t.Fatalf("Q607 after Shadith = %v, want cond 2", vars)
	}

	if got := w.questWindow(t, kadun); !shows(got, "31370-05.htm") {
		t.Fatalf("Kadun's window with the head = %q, want 31370-05.htm", got)
	}
	// The experience also refreshes the player's own info, which the
	// rendered dialog lines leave out.
	w.srv.Client.Send(encodeBypass("Quest " + q607Name + " 31370-07.htm"))
	var got []string
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound, serverpackets.OpcodeQuestList,
			serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed:
			line, err := scriptcontract.Packet(f, w.roles)
			if err != nil {
				t.Fatalf("frame %x: %v", f, err)
			}
			got = append(got, line)
		}
	}
	want := []string{
		// One head: S1_DISAPPEARED, not S2_S1_DISAPPEARED; experience
		// alone: EARNED_S1_EXPERIENCE (Player.destroyItemByItemId,
		// PlayerStatus.addExpAndSp).
		"S SystemMessage id=302 3:7235",
		"S SystemMessage id=54 3:7219",
		"S SystemMessage id=45 1:10000",
		"S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0",
		"S QuestList",
		`S NpcHtmlMessage obj=kadun item=0 html="<html><body>31370-07.htm</body></html>\n"`,
		"S ActionFailed",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the trade = %q, want %q", got, want)
	}
	for id, n := range map[int32]int{headOfShadithID: 0, totemOfValorID: 1, ketraAlliance3ID: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the trade = %d, want %d", id, c, n)
		}
	}
	if vars := w.srv.QuestVars(t, w.player, q607Name); len(vars) != 0 {
		t.Fatalf("Q607 after the trade = %v, want no state", vars)
	}

	if got := w.questWindow(t, kadun); !shows(got, "31370-02.htm") {
		t.Fatalf("Kadun's window with the totem = %q, want 31370-02.htm", got)
	}
}
