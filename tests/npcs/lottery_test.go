package npcs

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// lotterySellerID is the human Lottery Ticket Seller, a plain Folk whose
// pages are data/html/default/30990*.htm.
const lotterySellerID = 30990

// lotteryWorld is a booted server running the lottery seeded by seed, with
// the seller's shipped pages, the player holding adena and tickets, next to
// the seller.
type lotteryWorld struct {
	*folkWorld
	seller *npc.Folk
}

// heldTicket is a lottery ticket the player holds at login.
type heldTicket struct{ round, low, high int32 }

// lotteryTemplates are the fixture item templates plus the Lottery Ticket
// (4400-4499.xml: LOTTO, weight 20, not tradable, dropable, sellable or
// depositable).
func lotteryTemplates() *item.Table {
	templates := append([]*item.Template(nil), gameservertest.ItemTemplates().All()...)
	templates = append(templates, &item.Template{
		ID: lottery.TicketID, Name: "Lottery Ticket", Kind: item.KindEtcItem, Duration: -1,
		Weight: 20, Destroyable: true, EtcItem: &item.EtcItemDetail{Type: item.EtcItemLotto},
	})
	return item.NewTable(templates)
}

// sellerPages are the dialog fixture pages plus the seller's shipped ones.
func sellerPages(t *testing.T) map[string]string {
	t.Helper()
	pages := dialogPages()
	for _, suffix := range []string{"", "-1", "-2", "-3", "-4", "-5"} {
		name := strconv.Itoa(lotterySellerID) + suffix + ".htm"
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "default", name))
		if err != nil {
			t.Fatal(err)
		}
		pages["default/"+name] = string(data)
	}
	return pages
}

// bootLottery boots the lottery seeded by seed (none when nil: the lottery
// is not wired), the player holding adena and tickets, next to the seller.
// It returns the tickets' object ids.
func bootLottery(t *testing.T, seed func(*sql.DB), adena int32, tickets []heldTicket, opts ...lottery.Option) (*lotteryWorld, []int32) {
	t.Helper()
	extra := []gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(sellerPages(t)),
		gameservertest.WithItemTemplates(lotteryTemplates()),
		noBypassReuse,
	}
	if seed != nil {
		extra = append(extra, gameservertest.WithLottery(lottery.DefaultConfig(), seed, opts...))
	}
	srv := gameservertest.Boot(t, extra...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	var ids []int32
	for _, tk := range tickets {
		id := srv.NewObjectID()
		if err := srv.Items.Create(context.Background(), w.player, item.Instance{
			ObjectID: id, TemplateID: lottery.TicketID, OwnerID: w.player, Count: 1,
			EnchantLevel: int(tk.low), CustomType1: int(tk.round), CustomType2: int(tk.high),
			Location: item.LocationInventory,
		}); err != nil {
			t.Fatalf("give ticket: %v", err)
		}
		ids = append(ids, id)
	}
	startInWorld(t, srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return &lotteryWorld{folkWorld: w, seller: w.spawnFolk(t, folkTemplate("Folk", lotterySellerID), 60)}, ids
}

// seedRounds stores rounds as games rows: id, idnr, number1, number2,
// prize, newprize, prize1, prize2, prize3, enddate, finished.
func seedRounds(t *testing.T, rows ...[]any) func(*sql.DB) {
	return func(db *sql.DB) {
		for _, r := range rows {
			if _, err := db.Exec("INSERT INTO games (id, idnr, number1, number2, prize, newprize, prize1, prize2, prize3, enddate, finished) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", r...); err != nil {
				t.Errorf("seed games: %v", err)
			}
		}
	}
}

// sellerPage is the seller's shipped page n, as the page cache holds it,
// with its placeholders set to the given values in order.
func sellerPage(t *testing.T, n int, values ...string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "default", strconv.Itoa(lotterySellerID)+"-"+strconv.Itoa(n)+".htm"))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	for i := 0; i < len(values); i += 2 {
		page = strings.ReplaceAll(page, values[i], values[i+1])
	}
	return page
}

// filled sets the placeholders every lottery page shares.
func (w *lotteryWorld) filled(page string, round, prize int, endDate int64) string {
	for _, kv := range [][2]string{
		{"%objectId%", strconv.Itoa(int(w.seller.ObjectID()))},
		{"%race%", strconv.Itoa(round)},
		{"%adena%", strconv.Itoa(prize)},
		{"%ticket_price%", "2000"},
		{"%enddate%", w.srv.Lottery.FormatDate(endDate)},
	} {
		page = strings.ReplaceAll(page, kv[0], kv[1])
	}
	return page
}

// loto sends the seller's "Loto <n>" command and keeps the frames a dialog
// command is judged by.
func (w *lotteryWorld) loto(t *testing.T, n int) [][]byte {
	t.Helper()
	return w.dialogFrames(t, npcCommand(w.seller, "Loto "+strconv.Itoa(n)))
}

// assertLotteryPage checks frames are html from the seller then the two
// releases, the page's own and the dispatcher's.
func (w *lotteryWorld) assertLotteryPage(t *testing.T, frames [][]byte, html string) {
	t.Helper()
	assertAnswer(t, frames, chatWindowAnswer, w.seller, html)
}

// heldTickets reads the player's lottery tickets: object id, round, enchant
// level and custom type 2, by object id.
func (w *lotteryWorld) heldTickets(t *testing.T) [][4]int32 {
	t.Helper()
	var out [][4]int32
	for _, inst := range w.srv.PlayerInventory(t, w.player).ItemsByTemplateID(lottery.TicketID) {
		st := inst.Snapshot()
		out = append(out, [4]int32{st.ObjectID, int32(st.CustomType1), int32(st.EnchantLevel), int32(st.CustomType2)})
	}
	slices.SortFunc(out, func(a, b [4]int32) int { return int(a[0] - b[0]) })
	return out
}

func (w *lotteryWorld) adena(t *testing.T) int {
	t.Helper()
	return w.srv.PlayerInventory(t, w.player).Adena()
}

// TestLotterySellerSellsAndPaysTickets walks Npc.onBypassFeedback's Loto
// commands at a lottery seller from its shipped pages, round 3 on sale with
// a 60,000 jackpot and round 2 drawn as 3 7 17 18 20 (first place 30,000,
// third 3,000): Loto 0 opens the intro (-1), Loto 25 the instructions (-2)
// with each place's share as a percentage, Loto 21 the form (-5), where
// each pressed number shows checked and five of them turn the return link
// into the purchase link; Loto 22 takes 2,000 adena, raises the jackpot to
// 62,000 and hands over a ticket keeping the round in custom type 1 and the
// numbers 1 to 5 in its enchant level, then opens the jackpot page (-3).
// Loto 24 lists the tickets of past rounds with what they won (-4), the
// running round's ticket left out, and following a listed ticket's link
// destroys it and pays its prize, with no page. A ticket of the running
// round, or an object that is no ticket, is left alone.
func TestLotterySellerSellsAndPaysTickets(t *testing.T) {
	t.Parallel()
	endDate := time.Now().Add(48 * time.Hour).UnixMilli()
	w, ids := bootLottery(t, seedRounds(t,
		[]any{2, 68, 11, 55000, 60000, 30000, 7000, 3000, endDate - 7*24*3600*1000, 1},
		[]any{3, 0, 0, 60000, 60000, 0, 0, 0, endDate, 0},
	), 10000, []heldTicket{{2, 68, 11}, {2, 7, 3}, {3, 68, 11}})
	first, third, current := ids[0], ids[1], ids[2]
	w.talkTo(t, w.seller)

	w.assertLotteryPage(t, w.loto(t, 0), w.filled(sellerPage(t, 1), 3, 60000, endDate))
	w.assertLotteryPage(t, w.loto(t, 25), w.filled(sellerPage(t, 2,
		"%prize5%", "60.0", "%prize4%", "20.0", "%prize3%", "20.0", "%prize2%", "200"), 3, 60000, endDate))
	w.loto(t, 0)
	form := sellerPage(t, 5)
	w.assertLotteryPage(t, w.loto(t, 21), w.filled(form, 3, 60000, endDate))
	for n := 1; n <= 5; n++ {
		button := "0" + strconv.Itoa(n)
		form = strings.ReplaceAll(form,
			`fore="L2UI.lottoNum`+button+`" back="L2UI.lottoNum`+button+`a_check"`,
			`fore="L2UI.lottoNum`+button+`a_check" back="L2UI.lottoNum`+button+`"`)
		want := form
		if n == 5 {
			want = strings.ReplaceAll(form, `0">Return`, `22">The winner selected the numbers above.`)
		}
		w.assertLotteryPage(t, w.loto(t, n), w.filled(want, 3, 60000, endDate))
	}

	frames := w.loto(t, 22)
	assertFrames(t, "buy", frames[:2],
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(2000)),
		sysMsg(serverpackets.SystemMessageEarnedItemS1, itemNameParam(lottery.TicketID)))
	assertAnswer(t, frames[2:], chatWindowAnswer, w.seller, w.filled(sellerPage(t, 3), 3, 62000, endDate))
	if got := w.adena(t); got != 8000 {
		t.Fatalf("adena after buying = %d, want 8000", got)
	}
	held := w.heldTickets(t)
	if len(held) != 4 {
		t.Fatalf("tickets after buying = %v, want 4", held)
	}
	if bought := held[3]; bought[1] != 3 || bought[2] != 1|2|4|8|16 || bought[3] != 0 {
		t.Fatalf("bought ticket = %v, want round 3, enchant 31, custom type 2 0", bought)
	}
	w.srv.FlushItems(t)
	var prize, newPrize int
	if err := w.srv.DB.QueryRow("SELECT prize, newprize FROM games WHERE id = 1 AND idnr = 3").Scan(&prize, &newPrize); err != nil || prize != 62000 || newPrize != 62000 {
		t.Fatalf("round 3 row = %d/%d (%v), want jackpot 62000/62000", prize, newPrize, err)
	}
	var round, enchant, type2 int
	if err := w.srv.DB.QueryRow("SELECT custom_type1, enchant_level, custom_type2 FROM items WHERE object_id = ?", held[3][0]).Scan(&round, &enchant, &type2); err != nil || round != 3 || enchant != 31 || type2 != 0 {
		t.Fatalf("ticket row = %d/%d/%d (%v), want 3/31/0", round, enchant, type2, err)
	}

	w.talkTo(t, w.seller)
	oid := strconv.Itoa(int(w.seller.ObjectID()))
	list := `<a action="bypass -h npc_` + oid + `_Loto ` + strconv.Itoa(int(third)) + `">2 Event Number 1 2 3 17 18 - 3th Prize 3000a.</a><br>` +
		`<a action="bypass -h npc_` + oid + `_Loto ` + strconv.Itoa(int(first)) + `">2 Event Number 3 7 17 18 20 - 1st Prize 30000a.</a><br>`
	w.assertLotteryPage(t, w.loto(t, 24), w.filled(sellerPage(t, 4, "%result%", list), 3, 62000, endDate))

	assertFrames(t, "claim first prize", w.loto(t, int(first)),
		sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(lottery.TicketID)),
		sysMsg(serverpackets.SystemMessageEarnedS1Adena, numberParam(30000)),
		[]byte{serverpackets.OpcodeActionFailed})
	if got := w.adena(t); got != 38000 {
		t.Fatalf("adena after the claim = %d, want 38000", got)
	}

	w.openAnyNpcPage(t)
	adenaID := w.srv.PlayerInventory(t, w.player).ItemsByTemplateID(item.AdenaID)[0].Snapshot().ObjectID
	for _, id := range []int32{current, adenaID, first} {
		assertFrames(t, "claim of "+strconv.Itoa(int(id)), w.loto(t, int(id)), []byte{serverpackets.OpcodeActionFailed})
	}
	if held := w.heldTickets(t); len(held) != 3 || held[0][0] != third || held[1][0] != current {
		t.Fatalf("tickets after the claims = %v, want the third-place, running-round and bought ones", held)
	}
}

// TestLotterySellerRefusals pins the refusals: with no round running a
// number, the form or a purchase is refused with
// NO_LOTTERY_TICKETS_CURRENT_SOLD, and once sales have closed with
// NO_LOTTERY_TICKETS_AVAILABLE, each with no page; a purchase the talker
// cannot pay says so and sells nothing, one whose form misses a number
// answers only the release, and a negative command answers nothing at all.
func TestLotterySellerRefusals(t *testing.T) {
	t.Parallel()
	t.Run("no round", func(t *testing.T) {
		t.Parallel()
		w, _ := bootLottery(t, nil, 0, nil)
		w.openAnyNpcPage(t)
		for _, n := range []int{1, 21, 22} {
			assertFrames(t, "Loto "+strconv.Itoa(n), w.loto(t, n),
				sysMsg(serverpackets.SystemMessageNoLotteryTicketsCurrentSold), []byte{serverpackets.OpcodeActionFailed})
		}
	})
	t.Run("sales closed", func(t *testing.T) {
		t.Parallel()
		endDate := time.Now().Add(5 * time.Minute).UnixMilli()
		w, _ := bootLottery(t, seedRounds(t, []any{4, 0, 0, 60000, 60000, 0, 0, 0, endDate, 0}), 0, nil)
		if st := w.srv.Lottery.Status(); !st.Started || st.Selling {
			t.Fatalf("status = %+v, want a round started and not selling", st)
		}
		w.openAnyNpcPage(t)
		for _, n := range []int{1, 21, 22} {
			assertFrames(t, "Loto "+strconv.Itoa(n), w.loto(t, n),
				sysMsg(serverpackets.SystemMessageNoLotteryTicketsAvailable), []byte{serverpackets.OpcodeActionFailed})
		}
	})
	t.Run("unpaid and incomplete", func(t *testing.T) {
		t.Parallel()
		endDate := time.Now().Add(48 * time.Hour).UnixMilli()
		w, _ := bootLottery(t, seedRounds(t, []any{4, 0, 0, 60000, 60000, 0, 0, 0, endDate, 0}), 1999, nil)
		w.openAnyNpcPage(t)
		w.loto(t, 7)
		w.openAnyNpcPage(t)
		assertFrames(t, "incomplete form", w.loto(t, 22), []byte{serverpackets.OpcodeActionFailed})
		w.openAnyNpcPage(t)
		if frames := w.bypass(t, npcCommand(w.seller, "Loto -1")); len(frames) != 0 {
			t.Fatalf("Loto -1 = %x, want nothing", opcodes(frames))
		}
		w.openAnyNpcPage(t)
		for _, n := range []int{8, 9, 10, 11} {
			w.loto(t, n)
		}
		assertFrames(t, "unpaid", w.loto(t, 22),
			sysMsg(serverpackets.SystemMessageYouNotEnoughAdena), []byte{serverpackets.OpcodeActionFailed})
		if got, held := w.adena(t), w.heldTickets(t); got != 1999 || len(held) != 0 {
			t.Fatalf("after the unpaid purchase: adena %d, tickets %v; want 1999 and none", got, held)
		}
		if st := w.srv.Lottery.Status(); st.Prize != 60000 {
			t.Fatalf("jackpot after the unpaid purchase = %d, want 60000", st.Prize)
		}
	})
}

// TestLotteryDrawingReachesOnlinePlayers runs a resumed round to its
// drawing on the driven clock, its tickets read from items: the two
// tickets matching every drawn number share 60% of the 70,000 jackpot,
// 21,000 each, every player online is told
// (AMOUNT_FOR_WINNER_S1_IS_S2_ADENA_WE_HAVE_S3_PRIZE_WINNER), and the round
// is stored drawn. The next round's jackpot is the 50,000 base plus the
// jackpot less one winner's share, 99,000, as the reference counts it; it
// goes on sale a minute later with an announcement.
func TestLotteryDrawingReachesOnlinePlayers(t *testing.T) {
	t.Parallel()
	endDate := time.Now().Add(150 * time.Second).UnixMilli()
	rolls := []int{2, 6, 16, 17, 19}
	seed := func(db *sql.DB) {
		seedRounds(t, []any{5, 0, 0, 70000, 70000, 0, 0, 0, endDate, 0})(db)
		for i, n := range []lottery.Numbers{{Low: 68, High: 11}, {Low: 68, High: 11}, {Low: 1, High: 0}} {
			if _, err := db.Exec("INSERT INTO items (owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time) VALUES (?, ?, ?, 1, ?, 'WAREHOUSE', 0, 5, ?, -1, 0)",
				990001, 990001+i, lottery.TicketID, n.Low, n.High); err != nil {
				t.Errorf("seed ticket: %v", err)
			}
		}
	}
	w, _ := bootLottery(t, seed, 0, nil, lottery.WithRoll(func(int) int {
		r := rolls[0]
		rolls = rolls[1:]
		return r
	}))
	if !w.srv.DrivesClock() {
		t.Skip("waiting out a drawing needs the driven clock")
	}
	// The driven clock started at boot, a little behind the wall clock the
	// drawing was seeded from.
	w.srv.Advance(t, time.Until(time.UnixMilli(endDate))+10*time.Second)
	w.srv.AdvanceUntil(t, "the drawing", func() bool { return w.srv.Lottery.Status().Round == 6 })
	drawn := drainFrames(t, w.c)
	if frame, ok := firstOpcode(drawn, serverpackets.OpcodeSystemMessage); !ok ||
		string(frame) != string(sysMsg(serverpackets.SystemMessageAmountForWinnerS1IsS2AdenaWeHaveS3PrizeWinner, numberParam(5), numberParam(70000), numberParam(2))) {
		t.Fatalf("drawing frames = %x, want the winners' notice", opcodes(drawn))
	}
	w.srv.FlushPersistence(t)
	var finished, number1, number2, prize1, newPrize int
	if err := w.srv.DB.QueryRow("SELECT finished, number1, number2, prize1, newprize FROM games WHERE id = 1 AND idnr = 5").Scan(&finished, &number1, &number2, &prize1, &newPrize); err != nil ||
		finished != 1 || number1 != 68 || number2 != 11 || prize1 != 21000 || newPrize != 99000 {
		t.Fatalf("round 5 row = finished %d numbers %d/%d prize1 %d newprize %d (%v), want 1, 68/11, 21000, 99000", finished, number1, number2, prize1, newPrize, err)
	}

	w.srv.Advance(t, time.Minute)
	frame, ok := firstOpcode(drainFrames(t, w.c), serverpackets.OpcodeCreatureSay)
	if !ok {
		t.Fatal("the next round's sale was not announced")
	}
	r := wire.NewReader(frame[1:])
	if speaker, kind, name, text := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString(); speaker != 0 || kind != 10 || name != "" ||
		text != "Lottery tickets are now available for Lucky Lottery #6." {
		t.Fatalf("announcement = %d/%d/%q/%q, want the round 6 sale on the announcement channel", speaker, kind, name, text)
	}
	w.srv.FlushPersistence(t)
	var prize, rowEnd int64
	if err := w.srv.DB.QueryRow("SELECT prize, enddate FROM games WHERE id = 1 AND idnr = 6 AND finished = 0").Scan(&prize, &rowEnd); err != nil || prize != 99000 || rowEnd <= endDate {
		t.Fatalf("round 6 row = jackpot %d drawing %d (%v), want 99000 after %d", prize, rowEnd, err, endDate)
	}
	if st := w.srv.Lottery.Status(); !st.Started || !st.Selling || st.Prize != 99000 || st.EndDate != rowEnd {
		t.Fatalf("status = %+v, want round 6 selling with 99000 until %d", st, rowEnd)
	}
}
