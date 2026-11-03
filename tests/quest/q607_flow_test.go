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
)

// What the Prove Your Courage (Q607) scenarios cannot show, since a
// scenario seeds no party: the head of Shadith going to every ally of the
// killer's party and only to them. The offer to a Ketra ally and the trade
// for the totem are q607_prove_your_courage_ally.scenario.

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
