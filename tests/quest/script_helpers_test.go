package quest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The drop goldens' items: three stackable quest items.
const (
	dropItem      = 1081
	swordID       = 30
	shadowSwordID = 7884
	arrowID       = 17
)

// questItemTemplates is the shared item catalog plus the stackable quest
// items the suites hand out: Q001's and the drop goldens'.
func questItemTemplates() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range append(slices.Clone(q001Items), dropItem, dropItem+1, dropItem+2) {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// deferredUpdate reports whether frame is an inventory or status update,
// which the reference sends after the helper returns and the goldens
// leave out.
func deferredUpdate(frame []byte) bool {
	return frame[0] == serverpackets.OpcodeInventoryUpdate || frame[0] == serverpackets.OpcodeStatusUpdate
}

// immediateLines renders, as the goldens write packets, every frame the
// server queued to the client since the last read, deferred updates left
// out.
func immediateLines(t *testing.T, srv *gameservertest.Server) []string {
	t.Helper()
	var out []string
	for _, f := range srv.ReadQueued(t, srv.Client) {
		if deferredUpdate(f) {
			continue
		}
		line, err := scriptcontract.Packet(f, nil)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// rolls is a scripted random source: each draw takes the next roll and is
// recorded as bound:value.
type rolls struct {
	mu    sync.Mutex
	next  []int
	draws []string
}

func (r *rolls) intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.next) == 0 {
		panic(fmt.Sprintf("unscripted draw of [0, %d)", n))
	}
	v := r.next[0]
	r.next = r.next[1:]
	r.draws = append(r.draws, fmt.Sprintf("%d:%d", n, v))
	return v
}

// script sets the next rolls and forgets the draws made.
func (r *rolls) script(next []int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next, r.draws = next, nil
}

func (r *rolls) taken() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.draws
}

// bootHelpers boots one character in the world with the journal scripts,
// the quest items and opts.
func bootHelpers(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	srv, objID := bootJournal(t, opts...)
	enterWorld(t, srv)
	return srv, objID
}

// run runs fn as a Q001 hook invocation on objID's queue and fails the
// test when it panics.
func run(t *testing.T, srv *gameservertest.Server, objID int32, fn func(sc *script.Script, p *script.Player)) {
	t.Helper()
	if !srv.RunScript(t, objID, q001, fn) {
		t.Fatal("script invocation panicked")
	}
}

func dropTypeOf(t *testing.T, name string) script.DropType {
	t.Helper()
	switch name {
	case "divmod":
		return script.DropDivmod
	case "fixed_rate":
		return script.DropFixedRate
	case "fixed_count":
		return script.DropFixedCount
	case "fixed_both":
		return script.DropFixedBoth
	}
	t.Fatalf("drop type %q", name)
	return 0
}

func atoi32(t *testing.T, s string) int32 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	return int32(n)
}

// TestScriptDropsMatchReferenceGoldens replays the drop goldens end to
// end, one server per drop rate: before each row the player holds exactly
// the row's items, the random source returns the row's rolls, and the drop
// must draw what the reference drew, give what it gave, answer what it
// answered and send the reference's chat lines and sounds in order.
func TestScriptDropsMatchReferenceGoldens(t *testing.T) {
	t.Parallel()
	tables := []string{"drop.divmod", "drop.fixed_rate", "drop.fixed_count", "drop.fixed_both", "drop.multiple"}
	type golden struct {
		table string
		row   scriptcontract.Row
	}
	byRate := map[string][]golden{}
	var rates []string
	for _, table := range tables {
		for _, r := range scriptcontract.Lookup(t, table).Rows {
			rate := r.Str(t, "rate")
			if byRate[rate] == nil {
				rates = append(rates, rate)
			}
			byRate[rate] = append(byRate[rate], golden{table, r})
		}
	}
	items := []int32{dropItem, dropItem + 1, dropItem + 2}
	for _, rate := range rates {
		t.Run("rate"+rate, func(t *testing.T) {
			t.Parallel()
			f, err := strconv.ParseFloat(rate, 64)
			if err != nil {
				t.Fatal(err)
			}
			rs := &rolls{}
			srv, objID := bootHelpers(t, gameservertest.WithScriptRates(script.Rates{Drop: f, Reward: 1, RewardAdena: 1, XP: 1, SP: 1}),
				gameservertest.WithScriptRand(rs.intn))
			for _, g := range byRate[rate] {
				dropRow(t, srv, objID, rs, items, g.table, g.row)
			}
		})
	}
}

// dropRow replays one drop golden row on srv's player objID.
func dropRow(t *testing.T, srv *gameservertest.Server, objID int32, rs *rolls, items []int32, table string, r scriptcontract.Row) {
	t.Run(table+"/"+r.ID, func(t *testing.T) {
		have := map[int32]int32{}
		switch {
		case table != "drop.multiple":
			have[dropItem] = int32(r.Int(t, "have"))
		default:
			for _, h := range r.List(t, "have") {
				id, n, _ := strings.Cut(h, ":")
				have[atoi32(t, id)] = atoi32(t, n)
			}
		}
		run(t, srv, objID, func(sc *script.Script, p *script.Player) {
			for _, id := range items {
				sc.TakeItems(p, id, -1)
				sc.GiveItems(p, id, have[id])
			}
		})
		srv.ReadQueued(t, srv.Client)

		var next []int
		for _, v := range r.List(t, "rolls") {
			next = append(next, int(atoi32(t, v)))
		}
		rs.script(next)
		typ := dropTypeOf(t, r.Str(t, "type"))
		var result bool
		run(t, srv, objID, func(sc *script.Script, p *script.Player) {
			if table == "drop.multiple" {
				var drops []script.DropInfo
				for _, info := range r.List(t, "infos") {
					f := strings.Split(info, ":")
					drops = append(drops, script.DropInfo{ItemID: atoi32(t, f[0]), Count: atoi32(t, f[1]), Needed: atoi32(t, f[2]), Chance: atoi32(t, f[3])})
				}
				result = sc.DropMultipleItems(p, drops, typ)
				return
			}
			count, needed, chance := int32(r.Int(t, "count")), int32(r.Int(t, "needed")), int32(r.Int(t, "chance"))
			if typ == script.DropFixedRate && chance == script.MaxChance {
				result = sc.DropItemsAlways(p, dropItem, count, needed)
				return
			}
			result = sc.DropItemsAs(p, dropItem, count, needed, chance, typ)
		})

		if got := immediateLines(t, srv); !slices.Equal(got, r.Lines) {
			t.Fatalf("packets:\n got %q\nwant %q", got, r.Lines)
		}
		if got, want := rs.taken(), r.List(t, "draws"); !slices.Equal(got, want) {
			t.Fatalf("draws = %v, want %v", got, want)
		}
		if result != r.Bool(t, "result") {
			t.Fatalf("result = %v, want %v", result, r.Bool(t, "result"))
		}
		given := func(id int32) int32 { return int32(srv.PlayerItemCount(t, objID, id)) - have[id] }
		if table != "drop.multiple" {
			if got := given(dropItem); got != int32(r.Int(t, "given")) {
				t.Fatalf("given = %d, want %d", got, r.Int(t, "given"))
			}
			return
		}
		for _, g := range r.List(t, "given") {
			id, n, _ := strings.Cut(g, ":")
			if got := given(atoi32(t, id)); got != atoi32(t, n) {
				t.Fatalf("given of %s = %d, want %s", id, got, n)
			}
		}
	})
}

// TestScriptGiveTakeAndRewards drives the give, take, reward and sound
// helpers on the inline and the pool executors: each sends the reference's
// chat line, in order, and leaves the inventory and its rows as the
// reference does.
func TestScriptGiveTakeAndRewards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			giveTakeAndRewards(t, tc.opts...)
		})
	}
}

func giveTakeAndRewards(t *testing.T, opts ...gameservertest.Option) {
	srv, objID := bootHelpers(t, append(opts, gameservertest.WithScriptRates(script.Rates{Drop: 1, Reward: 1.5, RewardAdena: 2, XP: 1, SP: 1}))...)
	step := func(name string, fn func(sc *script.Script, p *script.Player), want ...string) {
		t.Helper()
		run(t, srv, objID, fn)
		if got := immediateLines(t, srv); !slices.Equal(got, want) {
			t.Fatalf("%s packets:\n got %q\nwant %q", name, got, want)
		}
	}
	count := func(id int32) int { return srv.PlayerItemCount(t, objID, id) }

	step("give adena", func(sc *script.Script, p *script.Player) { sc.GiveItems(p, item.AdenaID, 1000) },
		"S SystemMessage id=52 6:1000")
	step("give one", func(sc *script.Script, p *script.Player) { sc.GiveItems(p, dropItem, 1) },
		"S SystemMessage id=54 3:1081")
	step("give a stack", func(sc *script.Script, p *script.Player) { sc.GiveItems(p, dropItem, 5) },
		"S SystemMessage id=53 3:1081 6:5")
	step("give nothing", func(sc *script.Script, p *script.Player) {
		sc.GiveItems(p, dropItem, 0)
		sc.GiveItems(p, dropItem, -3)
		sc.GiveItems(p, 999999, 1)
	})
	step("give non-stackables", func(sc *script.Script, p *script.Player) { sc.GiveItems(p, swordID, 3) },
		"S SystemMessage id=53 3:30 6:3")
	if count(dropItem) != 6 || count(swordID) != 3 {
		t.Fatalf("held %d of %d and %d of %d, want 6 and 3", count(dropItem), dropItem, count(swordID), swordID)
	}
	step("give enchanted", func(sc *script.Script, p *script.Player) { sc.GiveItemsEnchanted(p, shadowSwordID, 1, 4) },
		"S SystemMessage id=54 3:7884")
	srv.FlushItems(t)
	if got := itemEnchants(t, srv, objID, shadowSwordID); !slices.Equal(got, []int{4}) {
		t.Fatalf("shadow sword enchants = %v, want [4]", got)
	}

	step("take part of a stack", func(sc *script.Script, p *script.Player) { sc.TakeItems(p, dropItem, 2) },
		"S SystemMessage id=301 3:1081 6:2")
	step("take none of a stack", func(sc *script.Script, p *script.Player) { sc.TakeItems(p, dropItem, 0) },
		"S SystemMessage id=302 3:1081")
	step("take more than held", func(sc *script.Script, p *script.Player) { sc.TakeItems(p, dropItem, 50) },
		"S SystemMessage id=301 3:1081 6:4")
	step("take what is not held", func(sc *script.Script, p *script.Player) {
		sc.TakeItems(p, dropItem, -1)
		sc.TakeItems(p, 999999, -1)
	})
	step("take adena", func(sc *script.Script, p *script.Player) {
		sc.TakeItems(p, item.AdenaID, 300)
		sc.TakeItems(p, item.AdenaID, 0)
	}, "S SystemMessage id=672 1:300")
	step("take two instances", func(sc *script.Script, p *script.Player) { sc.TakeItems(p, swordID, 2) },
		"S SystemMessage id=302 3:30", "S SystemMessage id=302 3:30")
	step("take every instance", func(sc *script.Script, p *script.Player) {
		sc.TakeItems(p, swordID, -1)
		sc.TakeItems(p, shadowSwordID, -1)
	}, "S SystemMessage id=302 3:30", "S SystemMessage id=1982 3:7884")
	if count(dropItem) != 0 || count(swordID) != 0 || count(shadowSwordID) != 0 || count(item.AdenaID) != 700 {
		t.Fatalf("held %d, %d, %d and %d adena after the takes", count(dropItem), count(swordID), count(shadowSwordID), count(item.AdenaID))
	}

	step("reward adena", func(sc *script.Script, p *script.Player) { sc.RewardItems(p, item.AdenaID, 150) },
		"S SystemMessage id=52 6:300")
	step("reward items", func(sc *script.Script, p *script.Player) { sc.RewardItems(p, dropItem, 3) },
		"S SystemMessage id=53 3:1081 6:4")
	step("reward rounding to nothing", func(sc *script.Script, p *script.Player) { sc.RewardItems(p, dropItem, 0) })
	step("sound", func(sc *script.Script, p *script.Player) { sc.PlaySound(p, script.SoundAccept) },
		"S PlaySound type=0 file=ItemSound.quest_accept bind=0 obj=0 loc=0,0,0 delay=0")

	srv.FlushItems(t)
	if got := itemRows(t, srv, objID); !maps.Equal(got, map[int32]int{item.AdenaID: 1000, dropItem: 4}) {
		t.Fatalf("item rows = %v", got)
	}
}

// TestScriptRewardExpAndSpScalesByRates rewards experience and skill points
// through the rates, truncating each product, with the earned message.
func TestScriptRewardExpAndSpScalesByRates(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, gameservertest.WithScriptRates(script.Rates{Drop: 1, Reward: 1, RewardAdena: 1, XP: 1.5, SP: 0.5}))
	before := progression(t, srv, objID)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) { sc.RewardExpAndSp(p, 101, 11) })
	frames := srv.ReadQueued(t, srv.Client)
	after := progression(t, srv, objID)
	if after.Exp-before.Exp != 151 || after.SP-before.SP != 5 {
		t.Fatalf("gained %d exp and %d sp, want 151 and 5", after.Exp-before.Exp, after.SP-before.SP)
	}
	want := fmt.Sprintf("S SystemMessage id=%d ", serverpackets.SystemMessageYouEarnedS1ExpAndS2SP)
	if !slices.ContainsFunc(frames, func(f []byte) bool {
		line, err := scriptcontract.Packet(f, nil)
		return err == nil && strings.HasPrefix(line, want)
	}) {
		t.Fatal("no earned experience and SP message")
	}
}

// progression reads objID's experience and skill points on its queue.
func progression(t *testing.T, srv *gameservertest.Server, objID int32) player.Progression {
	t.Helper()
	var out player.Progression
	srv.RunQuest(t, objID, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) { out = c.ProgressionValues() })
	return out
}

// TestScriptTakeUnequipsAWornItem takes a worn stack and a worn weapon:
// each is taken off first with no chat line of its own, the wearer's look
// is resent, and then the take names it.
func TestScriptTakeUnequipsAWornItem(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) { sc.GiveItems(p, swordID, 1) })
	srv.ReadQueued(t, srv.Client)
	sword := srv.PlayerInventory(t, objID).ItemByTemplateID(swordID)
	srv.Client.Send(encodeUseItem(sword.ObjectID))
	srv.ReadQueued(t, srv.Client)
	if !sword.Equipped() {
		t.Fatal("the sword was not put on")
	}

	run(t, srv, objID, func(sc *script.Script, p *script.Player) { sc.TakeItems(p, swordID, 1) })
	var opcodes []byte
	var lines []string
	for _, f := range srv.ReadQueued(t, srv.Client) {
		opcodes = append(opcodes, f[0])
		if f[0] == serverpackets.OpcodeSystemMessage {
			line, err := scriptcontract.Packet(f, nil)
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, line)
		}
	}
	if !slices.Equal(lines, []string{"S SystemMessage id=302 3:30"}) {
		t.Fatalf("chat lines = %q, want only the take's", lines)
	}
	ui := slices.Index(opcodes, serverpackets.OpcodeUserInfo)
	msg := slices.Index(opcodes, serverpackets.OpcodeSystemMessage)
	if ui < 0 || ui > msg {
		t.Fatalf("opcodes %x: want UserInfo before the take's message", opcodes)
	}
	if srv.PlayerItemCount(t, objID, swordID) != 0 || sword.Equipped() {
		t.Fatal("the worn sword is still held or worn")
	}
}

// TestScriptItemsRefusedWhileDetaching keeps a player's handle past its
// detach and gives and takes through it: nothing changes, nothing is sent,
// and the next selection loads the items the player left with.
func TestScriptItemsRefusedWhileDetaching(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	var (
		left *script.Player
		sc   *script.Script
	)
	run(t, srv, objID, func(s *script.Script, p *script.Player) {
		s.GiveItems(p, dropItem, 3)
		sc, left = s, p
	})
	srv.ReadQueued(t, srv.Client)
	restart(t, srv)
	srv.FlushItems(t)

	sc.GiveItems(left, dropItem, 5)
	sc.GiveItems(left, swordID, 1)
	sc.TakeItems(left, dropItem, -1)
	sc.RewardItems(left, item.AdenaID, 100)
	if !sc.DropItemsAlways(left, dropItem, 10, 3) {
		t.Fatal("a drop toward a count already held did not report it reached")
	}
	srv.FlushItems(t)
	srv.Client.ExpectNoFrame()
	if got := itemRows(t, srv, objID); !maps.Equal(got, map[int32]int{dropItem: 3}) {
		t.Fatalf("item rows after the detached calls = %v", got)
	}
	enterWorld(t, srv)
	if got := srv.PlayerItemCount(t, objID, dropItem); got != 3 {
		t.Fatalf("reselected with %d of %d, want 3", got, dropItem)
	}
}

// TestQuestAbortTakesQuestItems aborts a quest whose items the player
// holds: the quest window comes first, then each item is named as it goes.
func TestQuestAbortTakesQuestItems(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, q001, "<cond>", val("1")},
	)
	srv.GiveItem(t, objID, q001Items[0], 1)
	srv.GiveItem(t, objID, q001Items[2], 4)
	enterWorld(t, srv)
	srv.Client.Send(encodeRequestQuestAbort(1))
	want := []string{"S QuestList", "S SystemMessage id=302 3:687", "S SystemMessage id=301 3:1079 6:4"}
	if got := immediateLines(t, srv); !slices.Equal(got, want) {
		t.Fatalf("packets:\n got %q\nwant %q", got, want)
	}
	srv.FlushItems(t)
	if got := itemRows(t, srv, objID); len(got) != 0 {
		t.Fatalf("item rows after the abort = %v", got)
	}
}

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

// itemRows returns ownerID's inventory rows as item id to count, summed.
func itemRows(t *testing.T, srv *gameservertest.Server, ownerID int32) map[int32]int {
	t.Helper()
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT item_id, count FROM items WHERE owner_id=? AND loc='INVENTORY'", ownerID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int32]int{}
	for rows.Next() {
		var id int32
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		out[id] += n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// itemEnchants returns the enchant level of each of ownerID's rows of
// itemID.
func itemEnchants(t *testing.T, srv *gameservertest.Server, ownerID, itemID int32) []int {
	t.Helper()
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT enchant_level FROM items WHERE owner_id=? AND item_id=?", ownerID, itemID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
