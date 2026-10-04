package npcs

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// raceManagerID is the shipped race manager, whose pages live in
// data/html/default.
const raceManagerID = 30995

// derbyRunnerTemplates are the 24 runners, named after their npc id.
func derbyRunnerTemplates() []derby.Template {
	var out []derby.Template
	for id := derby.FirstRunnerID; id <= derby.LastRunnerID; id++ {
		out = append(out, derby.Template{NpcID: id, Name: "Runner" + strconv.Itoa(id), CollisionHeight: float64(id%7) + 10.5, CollisionRadius: float64(id%5) + 6})
	}
	return out
}

// laneSpeed is the fixture draw: every random segment of lane i (0 to 7)
// runs at 65 + laneSpeed[i]. Lanes 2 and 5 tie at the top, so the later,
// lane 5, wins and lane 2 is second.
var laneSpeed = [derby.Lanes]int{10, 59, 30, 40, 59, 50, 5, 0}

// fixedDraw keeps the runners in template order and draws laneSpeed.
type fixedDraw struct{ calls int }

func (d *fixedDraw) IntN(int) int {
	lane := d.calls / (20 - 1)
	d.calls++
	return laneSpeed[lane%derby.Lanes]
}

func (d *fixedDraw) Shuffle(int, func(i, j int)) {}

// derbyTicketTemplates are the default items plus the race ticket: an
// unstackable paper slip weighing 20.
func derbyTicketTemplates() *item.Table {
	all := gameservertest.ItemTemplates().All()
	all = append(all, &item.Template{ID: derby.TicketItemID, Name: "Monster Race Ticket - Single", Kind: item.KindEtcItem, Duration: -1, Weight: 20, Destroyable: true, EtcItem: &item.EtcItemDetail{}})
	return item.NewTable(all)
}

// derbyPages are the shipped race manager pages, as the page cache
// holds them.
func derbyPages(t *testing.T) map[string]string {
	t.Helper()
	pages := dialogPages()
	for _, name := range []string{"30995.htm", "30995-1.htm", "30995-2.htm", "30995-3.htm", "30995-4.htm", "30995-5.htm", "30995-6.htm", "30995-7.htm", "30995-8.htm", "30995-9.htm"} {
		pages["default/"+name] = shippedDefaultPage(t, name)
	}
	return pages
}

func shippedDefaultPage(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "default", name))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

// derbyPage is shipped page name with its placeholders set, in order, then
// %objectId% set to f's id.
func derbyPage(t *testing.T, f *npc.Folk, name string, values ...string) string {
	t.Helper()
	page := shippedDefaultPage(t, name)
	for i := 0; i < len(values); i += 2 {
		page = strings.ReplaceAll(page, values[i], values[i+1])
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
}

// runnerNames are the placeholders of the eight runners of the fixture
// race, which runs the first eight templates in order.
func runnerNames() []string {
	var out []string
	for i := range derby.Lanes {
		out = append(out, "Mob"+strconv.Itoa(i+1), "Runner"+strconv.Itoa(derby.FirstRunnerID+i))
	}
	return out
}

type derbyWorld struct {
	*folkWorld
	track   *derby.Track
	manager *npc.Folk
}

// bootDerby enters the world holding adena with the race track wired,
// extra options added.
func bootDerby(t *testing.T, adena int32, extra ...gameservertest.Option) *derbyWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Better", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(derbyPages(t)),
		gameservertest.WithItemTemplates(derbyTicketTemplates()),
		gameservertest.WithDerbyTrack(derbyRunnerTemplates(), &fixedDraw{}),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	startInWorld(t, srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return &derbyWorld{folkWorld: w, track: srv.Derby}
}

// tick runs n seconds of the race countdown.
func (w *derbyWorld) tick(n int) {
	for range n {
		w.track.Tick()
	}
}

// html is the one page among frames.
func html(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("no page among %x", opcodes(frames))
	}
	_, page, _ := htmlMessage(t, frame)
	return page
}

// assertPage checks frames are page from the manager, its release and the
// dispatcher's.
func (w *derbyWorld) assertPage(t *testing.T, what string, frames [][]byte, want string) {
	t.Helper()
	wantOps := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	if got := opcodes(frames); !bytes.Equal(got, wantOps) {
		t.Fatalf("%s: answer = %x, want %x", what, got, wantOps)
	}
	if objectID, got, _ := htmlMessage(t, frames[0]); objectID != w.manager.ObjectID() || got != want {
		t.Fatalf("%s: page (object %d) =\n%q\nwant\n%q", what, objectID, got, want)
	}
}

// assertChat checks frames are lead, then the manager's first chat page,
// its release and the dispatcher's.
func (w *derbyWorld) assertChat(t *testing.T, what string, frames [][]byte, lead ...[]byte) {
	t.Helper()
	if len(frames) != len(lead)+3 {
		t.Fatalf("%s: answer = %x, want %d messages then the chat page", what, opcodes(frames), len(lead))
	}
	for i, want := range lead {
		if !bytes.Equal(frames[i], want) {
			t.Fatalf("%s: frame %d = %x, want %x", what, i, frames[i], want)
		}
	}
	w.assertPage(t, what, frames[len(lead):], derbyPage(t, w.manager, "30995.htm"))
}

// dialog sends the manager command and returns its answer without the
// inventory updates.
func (w *derbyWorld) dialog(t *testing.T, command string) [][]byte {
	t.Helper()
	var out [][]byte
	for _, f := range w.bypass(t, npcCommand(w.manager, command)) {
		if f[0] != serverpackets.OpcodeInventoryUpdate && f[0] != serverpackets.OpcodeStatusUpdate {
			out = append(out, f)
		}
	}
	return out
}

func (w *derbyWorld) tickets(t *testing.T) []*item.Instance {
	t.Helper()
	return w.srv.PlayerInventory(t, w.player).ItemsByTemplateID(derby.TicketItemID)
}

// TestDerbyRaceManagerShowsTheRaceAndHidesItsRunners pins the race manager's
// sight: a player coming to know it is shown the race ahead of the manager
// itself, and one losing it sees the eight runners removed ahead of the
// manager.
func TestDerbyRaceManagerShowsTheRaceAndHidesItsRunners(t *testing.T) {
	w := bootDerby(t, 0)
	w.tick(1)
	race, ok := w.track.RacePacket()
	if !ok {
		t.Fatal("no race after the first step")
	}
	f := w.srv.SpawnFolkNPCAt(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z})
	frames := drainFrames(t, w.c)
	if len(frames) < 2 || frames[0][0] != serverpackets.OpcodeMonRaceInfo || frames[1][0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("manager shown with %x, want MonRaceInfo then NpcInfo", opcodes(frames))
	}
	if want := framePayload(serverpackets.FrameMonRaceInfo(race)); !bytes.Equal(frames[0], want) {
		t.Fatalf("MonRaceInfo = %x, want %x", frames[0], want)
	}
	if got := int32(binaryLE(frames[0][1:5])); got != -1 {
		t.Fatalf("MonRaceInfo first code = %d, want -1 before the start", got)
	}

	w.srv.State.Despawn(f)
	frames = drainFrames(t, w.c)
	var gone []int32
	for _, fr := range frames {
		if fr[0] == serverpackets.OpcodeDeleteObject {
			gone = append(gone, int32(binaryLE(fr[1:5])))
		}
	}
	var want []int32
	for _, r := range w.track.Runners() {
		want = append(want, r.ObjectID)
	}
	want = append(want, f.ObjectID())
	if !slices.Equal(gone, want) {
		t.Fatalf("DeleteObject ids = %v, want the runners then the manager %v", gone, want)
	}
}

// TestDerbyRaceCycleAnnouncesToTheTrackZone runs one whole race and checks
// what a player standing on the track hears, second by second, then the
// stored record and the next race's opening.
func TestDerbyRaceCycleAnnouncesToTheTrackZone(t *testing.T) {
	form, err := zone.NewCuboid(-1000, 1000, -1000, 1000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewDerbyTrack(1, form))
	w := bootDerby(t, 0, gameservertest.WithZones(zones))
	drainFrames(t, w.c)

	w.tick(cycleSteps)
	var got [][]byte
	for _, f := range drainFrames(t, w.c) {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound, serverpackets.OpcodeMonRaceInfo, serverpackets.OpcodeDeleteObject:
			got = append(got, f)
		}
	}
	want := expectedRaceCycle(w.track, 1)
	if len(got) != len(want) {
		t.Fatalf("heard %d frames, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("frame %d = %x, want %x", i, got[i], want[i])
		}
	}

	h, ok := w.track.HistoryOf(1)
	if !ok || h != (derby.History{RaceID: 1, First: 4, Second: 1}) {
		t.Fatalf("race 1 record = %+v, %v; want lane 5 first, lane 2 second, no odds", h, ok)
	}
	var first, second int
	var odd float64
	if err := w.srv.DB.QueryRow("SELECT first, second, odd_rate FROM mdt_history WHERE race_id = 1").Scan(&first, &second, &odd); err != nil {
		t.Fatal(err)
	}
	if first != 4 || second != 1 || odd != 0 {
		t.Fatalf("stored race 1 = %d, %d, %v; want 4, 1, 0", first, second, odd)
	}

	w.tick(1)
	opening := drainFrames(t, w.c)
	msg, ok := firstOpcode(opening, serverpackets.OpcodeSystemMessage)
	if !ok || !bytes.Equal(msg, sysMsg(816, numberParam(2))) {
		t.Fatalf("next race opening = %x, want tickets available for race 2", opening)
	}
}

// cycleSteps is one race: countdown steps 0 to 1200.
const cycleSteps = 1201

// expectedRaceCycle is what a player on the track hears over race number
// race, step by step from the schedule.
func expectedRaceCycle(track *derby.Track, race int32) [][]byte {
	runners := track.Runners()
	var out [][]byte
	msg := func(id int32, numbers ...int32) {
		var params [][]byte
		for _, n := range numbers {
			params = append(params, numberParam(n))
		}
		out = append(out, sysMsg(id, params...))
	}
	for c := 0; c <= 1200; c++ {
		switch {
		case c == 0:
			out = append(out, monRaceInfo(runners, -1, 0))
			msg(816, race)
		case c == 300 || c == 600 || c == 840:
			msg(817, race)
			msg(818, map[int]int32{300: 10, 600: 5, 840: 1}[c])
		case c >= 30 && c <= 870 && c%30 == 0:
			msg(817, race)
		case c == 900:
			msg(817, race)
			msg(819)
		case c == 960:
			msg(820, 2)
		case c == 1020:
			msg(820, 1)
		case c == 1050:
			msg(821)
		case c == 1070:
			msg(822)
		case c >= 1075 && c <= 1079:
			msg(823, int32(1080-c))
		case c == 1080:
			msg(824)
			out = append(out, playSound(1, "S_Race"), playSound(0, "ItemSound2.race_start"), monRaceInfo(runners, 0, 15322))
		case c == 1085:
			out = append(out, monRaceInfo(runners, 13765, -1))
		case c == 1115:
			msg(826, 5, 2)
			msg(825, race)
		case c == 1140:
			for _, r := range runners {
				out = append(out, append([]byte{serverpackets.OpcodeDeleteObject}, le32(r.ObjectID, 1)...))
			}
		}
	}
	return out
}

// monRaceInfo lays the fixture race out as the reference's MonRaceInfo
// does; the speeds show only in the start phase.
func monRaceInfo(runners []derby.Runner, code1, code2 int32) []byte {
	b := append([]byte{serverpackets.OpcodeMonRaceInfo}, le32(code1, code2, 8)...)
	for i, r := range runners {
		y := int32(181875 + 58*(7-i))
		b = append(b, le32(r.ObjectID, int32(r.NpcID)+1000000, 14107, y, -3566, 12080, y, -3566)...)
		w := wire.NewPacketWriter(0)
		w.WriteFloat64(r.CollisionHeight)
		w.WriteFloat64(r.CollisionRadius)
		b = append(b, w.Bytes()[1:]...)
		b = append(b, le32(120)...)
		for j := range 20 {
			switch {
			case code1 != 0:
				b = append(b, 0)
			case j == 19:
				b = append(b, 100)
			default:
				b = append(b, byte(65+laneSpeed[i]))
			}
		}
		b = append(b, le32(0)...)
	}
	return b
}

func playSound(soundType int32, file string) []byte {
	w := wire.NewPacketWriter(serverpackets.OpcodePlaySound)
	w.WriteInt32(soundType)
	w.WriteString(file)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

// TestDerbyTicketPurchaseOddsAndPayout walks a better through the race:
// two tickets bought step by step while sales run, the refusals once they
// close, the odds, the result, the tickets traded in, the history, and the
// records and stakes a restarted track reads back.
func TestDerbyTicketPurchaseOddsAndPayout(t *testing.T) {
	w := bootDerby(t, 5000)
	w.tick(1)
	w.manager = w.spawnFolk(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), 60)
	w.talkTo(t, w.manager)
	names := runnerNames()

	w.assertChat(t, "odds while selling", w.dialog(t, "ShowOdds"), sysMsg(1044))
	w.assertPage(t, "lane list", w.dialog(t, "BuyTicket 0"), derbyPage(t, w.manager, "30995-2.htm", append(names, "No1", "", "1race", "1")...))
	w.assertPage(t, "lane 5", w.dialog(t, "BuyTicket 5"), derbyPage(t, w.manager, "30995-2.htm", append(names, "No1", "5", "1race", "1")...))
	w.assertPage(t, "price list", w.dialog(t, "BuyTicket 10"), derbyPage(t, w.manager, "30995-3.htm", "0place", "5", "Mob1", "Runner31007", "0adena", "", "1race", "1"))
	w.assertPage(t, "price 1000", w.dialog(t, "BuyTicket 13"), derbyPage(t, w.manager, "30995-3.htm", "0place", "5", "Mob1", "Runner31007", "0adena", "1000", "1race", "1"))
	w.assertPage(t, "summary", w.dialog(t, "BuyTicket 20"), derbyPage(t, w.manager, "30995-4.htm", "0place", "5", "Mob1", "Runner31007", "0adena", "1000", "0tax", "0", "0total", "1000", "1race", "1"))
	w.assertChat(t, "buy lane 5", w.dialog(t, "BuyTicket 21"),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1000)),
		sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID)))

	// The second ticket: lane 2 at 100 adena.
	for _, step := range []string{"BuyTicket 0", "BuyTicket 2", "BuyTicket 10", "BuyTicket 11", "BuyTicket 20"} {
		html(t, w.dialog(t, step))
	}
	w.assertChat(t, "buy lane 2", w.dialog(t, "BuyTicket 21"),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(100)),
		sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID)))

	if got := w.held(t, item.AdenaID); got != 3900 {
		t.Fatalf("adena after two tickets = %d, want 3900", got)
	}
	tickets := w.tickets(t)
	if len(tickets) != 2 {
		t.Fatalf("held %d tickets, want 2", len(tickets))
	}
	byLane := map[int]*item.Instance{}
	for _, ticket := range tickets {
		st := ticket.Snapshot()
		byLane[st.CustomType1] = ticket
		if st.EnchantLevel != 1 || st.CustomType2 != map[int]int{5: 10, 2: 1}[st.CustomType1] {
			t.Fatalf("ticket = race %d lane %d price %d00", st.EnchantLevel, st.CustomType1, st.CustomType2)
		}
	}
	rows := saved(t, w.srv, w.player)
	stored, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatal(err)
	}
	for _, inst := range stored {
		if inst.TemplateID == derby.TicketItemID && (inst.EnchantLevel != 1 || inst.CustomType2*100 != map[int]int{5: 1000, 2: 100}[inst.CustomType1]) {
			t.Fatalf("stored ticket = %+v", inst)
		}
	}
	if len(rows) == 0 {
		t.Fatal("no stored items")
	}
	if stakes := storedStakes(t, w.srv); stakes[5] != 1000 || stakes[2] != 100 || stakes[1] != 0 {
		t.Fatalf("stored stakes = %v, want lane 5 1000 and lane 2 100", stakes)
	}

	// Sales close at 15 minutes and the odds come out: 70% of 1100 over
	// each lane's stake, at least 1.25.
	w.tick(900)
	drainFrames(t, w.c)
	w.assertChat(t, "buy once closed", w.dialog(t, "BuyTicket 0"), sysMsg(1046))
	oddsPage := append(names, "Odd1", "&$804;", "Odd2", "7.7", "Odd3", "&$804;", "Odd4", "&$804;", "Odd5", "1.3", "Odd6", "&$804;", "Odd7", "&$804;", "Odd8", "&$804;", "1race", "1")
	w.assertPage(t, "odds", w.dialog(t, "ShowOdds"), derbyPage(t, w.manager, "30995-5.htm", oddsPage...))
	w.assertPage(t, "runners", w.dialog(t, "ShowInfo"), derbyPage(t, w.manager, "30995-6.htm", names...))
	w.dialog(t, "Chat 0")

	// The race ends at 18 minutes 35 seconds: lane 5 wins, lane 2 second.
	w.tick(1115 - 901 + 1)
	drainFrames(t, w.c)
	if got := w.track.RaceNumber(); got != 2 {
		t.Fatalf("race number after the race = %d, want 2", got)
	}

	win, lose := byLane[5].ObjectID, byLane[2].ObjectID
	listed := `<tr><td><a action="bypass -h npc_%objectId%_ShowTicket ` + strconv.Itoa(int(win)) + `">1 Race Number</a></td><td align=right><font color="LEVEL">5</font> Number</td><td align=right><font color="LEVEL">1000</font> Adena</td></tr>`
	other := `<tr><td><a action="bypass -h npc_%objectId%_ShowTicket ` + strconv.Itoa(int(lose)) + `">1 Race Number</a></td><td align=right><font color="LEVEL">2</font> Number</td><td align=right><font color="LEVEL">100</font> Adena</td></tr>`
	page := html(t, w.dialog(t, "ShowTickets"))
	if page != derbyPage(t, w.manager, "30995-7.htm", "%tickets%", listed+other) && page != derbyPage(t, w.manager, "30995-7.htm", "%tickets%", other+listed) {
		t.Fatalf("ticket list =\n%q", page)
	}
	w.assertPage(t, "winning ticket", w.dialog(t, "ShowTicket "+strconv.Itoa(int(win))), derbyPage(t, w.manager, "30995-8.htm",
		"%raceId%", "1", "%lane%", "5", "%bet%", "1000", "%firstLane%", "5", "%odd%", "1.25", "%ticketObjectId%", strconv.Itoa(int(win))))
	html(t, w.dialog(t, "ShowTickets"))
	w.assertPage(t, "losing ticket", w.dialog(t, "ShowTicket "+strconv.Itoa(int(lose))), derbyPage(t, w.manager, "30995-8.htm",
		"%raceId%", "1", "%lane%", "2", "%bet%", "100", "%firstLane%", "5", "%odd%", "0.01", "%ticketObjectId%", strconv.Itoa(int(lose))))
	html(t, w.dialog(t, "ShowTickets"))
	html(t, w.dialog(t, "ShowTicket "+strconv.Itoa(int(win))))
	w.assertChat(t, "cash the winner", w.dialog(t, "CalculateWin "+strconv.Itoa(int(win))),
		sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(derby.TicketItemID)),
		sysMsg(serverpackets.SystemMessageEarnedS1Adena, numberParam(1250)))
	html(t, w.dialog(t, "ShowTickets"))
	html(t, w.dialog(t, "ShowTicket "+strconv.Itoa(int(lose))))
	w.assertChat(t, "cash the loser", w.dialog(t, "CalculateWin "+strconv.Itoa(int(lose))),
		sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(derby.TicketItemID)),
		sysMsg(serverpackets.SystemMessageEarnedS1Adena, numberParam(1)))
	w.assertPage(t, "no ticket left", w.dialog(t, "ShowTickets"), derbyPage(t, w.manager, "30995-7.htm", "%tickets%", ""))
	w.dialog(t, "Chat 0")
	if got := w.held(t, item.AdenaID); got != 3900+1250+1 {
		t.Fatalf("adena after cashing = %d, want %d", got, 3900+1250+1)
	}
	if len(w.tickets(t)) != 0 {
		t.Fatal("tickets left after cashing both")
	}

	history := `<tr><td><font color="LEVEL">1</font> th</td><td><font color="LEVEL">5</font> Lane </td><td><font color="LEVEL">2</font> Lane</td><td align=right><font color=00ffff>1.25</font> Times</td></tr>`
	w.assertPage(t, "history", w.dialog(t, "ViewHistory"), derbyPage(t, w.manager, "30995-9.htm", "%infos%", history))

	if stakes := storedStakes(t, w.srv); stakes[5] != 0 || stakes[2] != 0 {
		t.Fatalf("stored stakes after the race = %v, want all 0", stakes)
	}
	restarted, err := derby.New(context.Background(), gamesql.NewDerbyStore(w.srv.DB), derbyRunnerTemplates(), &countingIDs{}, nil, &fixedDraw{}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.RaceNumber(); got != 2 {
		t.Fatalf("restarted race number = %d, want 2", got)
	}
	if h, ok := restarted.HistoryOf(1); !ok || h != (derby.History{RaceID: 1, First: 4, Second: 1, OddRate: 1.25}) {
		t.Fatalf("restarted race 1 = %+v, %v", h, ok)
	}
	if got := restarted.Stakes(); got != ([derby.Lanes]int64{}) {
		t.Fatalf("restarted stakes = %v, want none", got)
	}
}

// TestDerbyStakesSurviveARestartMidSale pins a stake stored while tickets
// sell: a restarted track reads it back on its lane.
func TestDerbyStakesSurviveARestartMidSale(t *testing.T) {
	w := bootDerby(t, 1000)
	w.tick(1)
	w.manager = w.spawnFolk(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), 60)
	w.talkTo(t, w.manager)
	for _, step := range []string{"BuyTicket 0", "BuyTicket 3", "BuyTicket 10", "BuyTicket 12", "BuyTicket 20", "BuyTicket 21"} {
		w.dialog(t, step)
	}
	restarted, err := derby.New(context.Background(), gamesql.NewDerbyStore(w.srv.DB), derbyRunnerTemplates(), &countingIDs{}, nil, &fixedDraw{}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Stakes(); got != ([derby.Lanes]int64{2: 500}) {
		t.Fatalf("restarted stakes = %v, want 500 on lane 3", got)
	}
	if got := restarted.RaceNumber(); got != 1 {
		t.Fatalf("restarted race number = %d, want 1: the unfinished race has no record", got)
	}
}

func storedStakes(t *testing.T, srv *gameservertest.Server) map[int]int64 {
	t.Helper()
	rows, err := srv.DB.Query("SELECT lane_id, bet FROM mdt_bets")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]int64{}
	for rows.Next() {
		var lane int
		var bet int64
		if err := rows.Scan(&lane, &bet); err != nil {
			t.Fatal(err)
		}
		out[lane] = bet
	}
	return out
}

type countingIDs struct{ next int32 }

func (c *countingIDs) NextID() (int32, error) {
	c.next++
	return 0x70000000 + c.next, nil
}

// framePayload is frame's packet without its two-byte length header.
func framePayload(frame wire.Frame) []byte {
	defer frame.Release()
	return append([]byte(nil), frame.Bytes()[2:]...)
}

func binaryLE(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
