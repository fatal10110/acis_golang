package quest

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q605"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q608"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// What the Slay the Enemy Commander (Q608) and Alliance with Ketra Orcs
// (Q605) scenarios cannot show, since a scenario seeds a party member
// neither a standing nor a quest row: the head of Mos going to every ally of
// the killer's party near him and only to them (Q608_SlayTheEnemyCommander.
// onMyDying, Quest.getPartyMembers), and the badge of a Varka warrior going
// to one random started member of the killer's party, whoever dealt the blow
// (Q605_AllianceWithKetraOrcs.onMyDying, Quest.getRandomPartyMemberState).
// The offers, the trades and the drops of one character are
// q608_slay_the_enemy_commander_*.scenario and q605_alliance_with_ketra_orcs_*.scenario.

const (
	q608Name = "Q608_SlayTheEnemyCommander"
	q608Path = "quest.Q608_SlayTheEnemyCommander"
	q605Name = "Q605_AllianceWithKetraOrcs"
	q605Path = "quest.Q605_AllianceWithKetraOrcs"

	mosID            = 25312
	headOfMosID      = 7236
	ketraAlliance4ID = 7214
	varkaSoldierID   = 21350
	soldierBadgeID   = 7216
)

// varkaItemTemplates is the shared item catalog plus a stackable quest item
// of each id.
func varkaItemTemplates(ids ...int32) gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range ids {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// partyMate is a character that joins the party of the first one.
type partyMate struct {
	account, name string
	level         int
}

// formParty seeds the leader (named leaderName) and each mate, calls seed
// with each one's object id and name before the world opens, and enters the
// world: the leader first, then the mates, each invited by the leader and
// accepting. It returns the leader's and the mates' object ids by name and
// every member's client.
func formParty(t *testing.T, srv *gameservertest.Server, leaderName string, mates []partyMate, seed func(id int32, name string)) (ids map[string]int32, clients map[int32]*testsupport.ScriptedClient) {
	t.Helper()
	leader := srv.SoleObjectID(t)
	ids = map[string]int32{leaderName: leader}
	seed(leader, leaderName)
	for _, m := range mates {
		id := srv.SeedCharacterFor(t, m.account, m.name, m.level, 0).ID
		seed(id, m.name)
		ids[m.name] = id
	}
	enterWorld(t, srv)
	clients = map[int32]*testsupport.ScriptedClient{leader: srv.Client}
	for _, m := range mates {
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
		clients[ids[m.name]] = c
	}
	for _, c := range clients {
		srv.ReadQueued(t, c)
	}
	return ids, clients
}

// questSoundsAndItems keeps the quest sounds and the earned-item messages of
// the rendered lines.
func questSoundsAndItems(lines []string, itemID int32) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, "S PlaySound") || strings.HasPrefix(l, itemMessage(54, itemID)) {
			out = append(out, l)
		}
	}
	return out
}

// TestQ608MosCreditsEveryKetraAllyOfTheParty: Leader (Ketra 4, mark), Mate
// (Ketra 3, mark), Third (Ketra 5, mark) and Fourth (Ketra 4, no mark) are
// one party, all at cond 1 beside Mos. Leader kills him: Leader and Third,
// the allies of level 4 or more holding the mark, each get a head and move
// to cond 2, with the middle sound before the head; Mate and Fourth get
// nothing.
func TestQ608MosCreditsEveryKetraAllyOfTheParty(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{mosID: script.KindHostile},
			[]script.Listing{{Path: q608Path}},
			script.Catalog{q608Path: q608.New},
		),
		varkaItemTemplates(headOfMosID, ketraAlliance4ID),
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
		itemMessage(54, headOfMosID),
	}
	want := map[string]member{
		"Leader": {4, true, 1, "2", credited},
		"Mate":   {3, true, 0, "1", nil},
		"Third":  {5, true, 1, "2", credited},
		"Fourth": {4, false, 0, "1", nil},
	}
	mates := []partyMate{{"mate", "Mate", 75}, {"third", "Third", 75}, {"fourth", "Fourth", 75}}
	ids, clients := formParty(t, srv, "Leader", mates, func(id int32, name string) {
		insertJournal(t, srv.DB,
			journalRow{id, q608Name, "<state>", val("STARTED")},
			journalRow{id, q608Name, "<cond>", val("1")},
		)
		seedStanding(t, srv, id, want[name].standing)
		if want[name].mark {
			srv.GiveItem(t, id, ketraAlliance4ID, 1)
		}
	})

	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	mos := srv.SpawnHostileNPCTemplateAt(t, proofMonster(mosID), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, srv, ids["Leader"], mos)

	for name, w := range want {
		id := ids[name]
		if got := questSoundsAndItems(scriptLines(t, srv.ReadQueued(t, clients[id])), headOfMosID); !slices.Equal(got, w.lines) {
			t.Errorf("%s's quest sounds and messages = %q, want %q", name, got, w.lines)
		}
		if n := srv.PlayerItemCount(t, id, headOfMosID); n != w.head {
			t.Errorf("%s's heads = %d, want %d", name, n, w.head)
		}
		if vars := srv.QuestVars(t, id, q608Name); vars["<cond>"] != w.cond {
			t.Errorf("%s's Q608 = %v, want cond %s", name, vars, w.cond)
		}
	}
}

// TestQ605BadgeGoesToOneRandomStartedMemberOfTheParty: Leader (no Q605), Mate
// (cond 3) and Third (cond 2) are one party beside a Varka soldier. Leader
// kills it with the draws picking the first started member, then the
// second: the badge goes to Mate, then to Third, and never to Leader, who
// has not started the quest.
func TestQ605BadgeGoesToOneRandomStartedMemberOfTheParty(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{varkaSoldierID: script.KindHostile},
			[]script.Listing{{Path: q605Path}},
			script.Catalog{q605Path: q605.New},
		),
		varkaItemTemplates(soldierBadgeID),
	)
	conds := map[string]string{"Mate": "3", "Third": "2"}
	mates := []partyMate{{"mate", "Mate", 75}, {"third", "Third", 75}}
	ids, _ := formParty(t, srv, "Leader", mates, func(id int32, name string) {
		if cond, ok := conds[name]; ok {
			insertJournal(t, srv.DB,
				journalRow{id, q605Name, "<state>", val("STARTED")},
				journalRow{id, q605Name, "<cond>", val(cond)},
			)
		}
	})
	badges := func() map[string]int {
		out := map[string]int{}
		for name, id := range ids {
			out[name] = srv.PlayerItemCount(t, id, soldierBadgeID)
		}
		return out
	}

	x, y, z := srv.PlayerPosition(t, ids["Leader"])
	// The pick, then the chance of the drop: 0 is a success.
	draws.queue(0, 0)
	killOnPlayerQueue(t, srv, ids["Leader"], srv.SpawnHostileNPCTemplateAt(t, proofMonster(varkaSoldierID), location.Location{X: x + 40, Y: y, Z: z}))
	if got, want := badges(), map[string]int{"Leader": 0, "Mate": 1, "Third": 0}; !maps.Equal(got, want) {
		t.Fatalf("badges after the first kill = %v, want %v", got, want)
	}
	draws.queue(1, 0)
	killOnPlayerQueue(t, srv, ids["Leader"], srv.SpawnHostileNPCTemplateAt(t, proofMonster(varkaSoldierID), location.Location{X: x + 40, Y: y, Z: z}))
	if got, want := badges(), map[string]int{"Leader": 0, "Mate": 1, "Third": 1}; !maps.Equal(got, want) {
		t.Fatalf("badges after the second kill = %v, want %v", got, want)
	}
}
