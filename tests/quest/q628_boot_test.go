package quest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q628"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Hunt of the Golden Ram Mercenary Force (Q628_HuntOfTheGoldenRamMercenaryForce):
// Abercrombie's multisell event and the party draw of the monsters' dying
// hook. A scenario shows neither: its frames leave out the multisell list
// (the format renders no MultiSellList) and its party member line cannot hold
// a quest of its own.
const (
	q628Name = "Q628_HuntOfTheGoldenRamMercenaryForce"
	q628Path = "quest.Q628_HuntOfTheGoldenRamMercenaryForce"

	q628Abercrombie = 31555
	q628Splinter    = 7248
	q628Badge       = 7246
	q628Soldier     = 7247
)

// q628Items is the shared item catalog plus the quest's items and those of
// the shipped lists Abercrombie offers.
func q628Items(t *testing.T) (*item.Table, gameservertest.Option) {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{7246, 7247, 7248, 7249, 7251, 7562, 7636, 7637, 7725} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item", Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	table := item.NewTable(templates)
	return table, gameservertest.WithItemTemplates(table)
}

// q628Boot boots a level 66 character with the quest STARTED at cond,
// holding badge, with the shipped lists 6281 and 6282 loaded.
func q628Boot(t *testing.T, cond string, badge int32) *dialogWorld {
	t.Helper()
	items, itemOpt := q628Items(t)
	dir := t.TempDir()
	for _, list := range []string{"6281.xml", "6282.xml"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "xml", "multisell", list))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, list), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lists, err := gamexml.LoadMultiSellLists(dir, items)
	if err != nil {
		t.Fatal(err)
	}
	w := bootSeamQuest(t, 66, chatPages(shippedQuestPages(t, q628Name), q628Abercrombie), func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q628Name, "<state>", val("STARTED")},
			journalRow{objID, q628Name, "<cond>", val(cond)})
		srv.GiveItem(t, objID, badge, 1)
	},
		itemOpt,
		gameservertest.WithMultisells(lists),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", q628Abercrombie)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{q628Abercrombie: script.KindFolk},
			[]script.Listing{{Path: q628Path}}, script.Catalog{q628Path: q628.New}),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "abercrombie", q628Abercrombie, 40, 0, 0)
	return w
}

// q628Opcodes returns the opcodes of frames.
func q628Opcodes(frames [][]byte) []byte {
	var out []byte
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

// TestQ628AbercrombieOffersItsListToASoldierOnly: at cond 3, with the badge of
// a soldier, the multisell event shows the list it names; at cond 2 the event
// shows nothing, the list as well as a page.
func TestQ628AbercrombieOffersItsListToASoldierOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cond  string
		badge int32
		event string
		list  bool
	}{
		{"soldier", "3", q628Soldier, "gmultisell 6282", true},
		{"recruit", "2", q628Badge, "gmultisell 6281", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q628Boot(t, tc.cond, tc.badge)
			w.interact(t, w.npcs["abercrombie"])
			w.srv.Client.Send(encodeBypass("Quest " + q628Name + " " + tc.event))
			frames := w.srv.ReadQueued(t, w.srv.Client)
			if got := slices.Contains(q628Opcodes(frames), serverpackets.OpcodeMultiSellList); got != tc.list {
				t.Fatalf("MultiSellList sent = %v, want %v (opcodes %x)", got, tc.list, q628Opcodes(frames))
			}
			if !tc.list {
				if got := w.render(t, frames); len(got) != 0 {
					t.Fatalf("a recruit's event answered %v, want nothing", got)
				}
			}
		})
	}
}

// TestQ628SwampMonstersDropToAPartyMemberWithTheQuest: a monster killed by a
// character without the quest drops its splinter to the party member who
// has it, and to nobody else.
func TestQ628SwampMonstersDropToAPartyMemberWithTheQuest(t *testing.T) {
	t.Parallel()
	_, itemOpt := q628Items(t)
	var mate int32
	w := bootSeamQuest(t, 66, nil, func(srv *gameservertest.Server, objID int32) {
		mate = srv.SeedCharacterFor(t, "mate", "Mate", 66, 0).ID
		insertJournal(t, srv.DB,
			journalRow{mate, q628Name, "<state>", val("STARTED")},
			journalRow{mate, q628Name, "<cond>", val("1")})
	},
		itemOpt,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(21508)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{21508: script.KindHostile},
			[]script.Listing{{Path: q628Path}}, script.Catalog{q628Path: q628.New}),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
	)
	fc := w.srv.DialClient(t, "mate", 1)
	fc.Send(encodeRequestGameStart(0))
	readUntil(t, fc, serverpackets.OpcodeCharSelected)
	fc.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	w.srv.ReadQueued(t, fc)
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Client.Send(encodeJoinParty("Mate"))
	w.srv.ReadQueued(t, fc)
	fc.Send(encodeAnswerJoinParty(1))
	w.srv.ReadQueued(t, fc)
	w.srv.ReadQueued(t, w.srv.Client)

	x, y, z := w.srv.PlayerPosition(t, w.player)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(21508), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, w.srv, w.player, h)
	if n := w.srv.PlayerItemCount(t, mate, q628Splinter); n != 1 {
		t.Fatalf("the member with the quest holds %d splinters, want 1", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q628Splinter); n != 0 {
		t.Fatalf("the killer without the quest holds %d splinters, want 0", n)
	}
}
