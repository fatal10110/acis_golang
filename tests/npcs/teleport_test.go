package npcs

import (
	"context"
	"encoding/binary"
	"maps"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The gatekeeper these scenarios talk to and its destinations: a standard
// one for adena, a newbie one for a travel token, which the list leaves
// out but whose index the standard ones after it still count, and a free
// standard one.
const (
	gatekeeperID  = 30080
	travelTokenID = 8542
	tripPrice     = 1000
)

var (
	paidSpot  = location.Location{X: 15000, Y: 25000, Z: -3000}
	tokenSpot = location.Location{X: 9716, Y: 15502, Z: -4500}
	freeSpot  = location.Location{X: -12000, Y: 120000, Z: -3600}
	// Instant destinations of the same NPC.
	instantSpot = location.Location{X: 40000, Y: -50000, Z: -1000}
)

// gatekeeperPage is a gatekeeper's first page: its teleport list link.
const gatekeeperPage = `<html><body>Gatekeeper:<br>` +
	`<a action="bypass -h npc_%objectId%_teleport_request">Teleport</a></body></html>`

func gatekeeperDestinations() (travel.TeleportTable, travel.InstantTable) {
	return travel.TeleportTable{
		gatekeeperID: {
			{Location: paidSpot, Description: "Town of Dion", Kind: travel.KindStandard, PriceID: int(item.AdenaID), PriceCount: tripPrice},
			{Location: tokenSpot, Description: "Dark Elf Village", Kind: travel.KindNewbieToken, PriceID: travelTokenID, PriceCount: 1},
			{Location: freeSpot, Description: "Free Spot", Kind: travel.KindStandard, PriceID: int(item.AdenaID)},
		},
	}, travel.InstantTable{
		gatekeeperID: {instantSpot},
	}
}

// teleportClock is a settable wall clock, a Wednesday noon until set.
type teleportClock struct{ at atomic.Int64 }

func newTeleportClock() *teleportClock {
	c := &teleportClock{}
	c.set(time.Date(2026, time.July, 8, 12, 0, 0, 0, time.Local))
	return c
}

func (c *teleportClock) set(at time.Time) { c.at.Store(at.UnixNano()) }
func (c *teleportClock) now() time.Time   { return time.Unix(0, c.at.Load()) }

// travelTemplates is the behavior catalog plus the newbie travel token
// and ancient adena.
func travelTemplates() *item.Table {
	currency := func(id int32, name string) *item.Template {
		return &item.Template{
			ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		currency(travelTokenID, "Newbie Travel Token"), currency(item.AncientAdenaID, "Ancient Adena")))
}

// bootGatekeeperWorld enters the world holding adena and tokens travel
// tokens, next to the fixture gatekeeper.
func bootGatekeeperWorld(t *testing.T, adena, tokens int32, free bool, clock *teleportClock) (*folkWorld, *npc.Folk) {
	t.Helper()
	teleports, instants := gatekeeperDestinations()
	return bootTravelWorld(t, teleports, instants, free, clock, map[int32]int32{item.AdenaID: adena, travelTokenID: tokens})
}

// bootTravelWorld enters the world holding held (template id to count),
// next to the fixture gatekeeper offering teleports and instants.
func bootTravelWorld(t *testing.T, teleports travel.TeleportTable, instants travel.InstantTable, free bool, clock *teleportClock, held map[int32]int32) (*folkWorld, *npc.Folk) {
	t.Helper()
	pages := dialogPages()
	pages["gatekeeper/30080.htm"] = gatekeeperPage
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithItemTemplates(travelTemplates()),
		gameservertest.WithTeleports(teleports, instants, free, clock.now),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	for _, id := range slices.Sorted(maps.Keys(held)) {
		if held[id] > 0 {
			srv.GiveItem(t, w.player, id, held[id])
		}
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w, w.spawnFolk(t, folkTemplate("Gatekeeper", gatekeeperID), 50)
}

// tripFrames sends command and keeps the frames a dialog teleport is
// judged by: pages, messages, the player's own jump and releases.
func (w *folkWorld) tripFrames(t *testing.T, command string) [][]byte {
	t.Helper()
	var out [][]byte
	for _, f := range w.bypass(t, command) {
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeExtended:
			out = append(out, f)
		case serverpackets.OpcodeTeleportToLocation:
			if int32(binary.LittleEndian.Uint32(f[1:5])) == w.player {
				out = append(out, f)
			}
		}
	}
	return out
}

// landing returns the TeleportToLocation among frames.
func landing(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	jump, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation)
	if !ok {
		t.Fatalf("no TeleportToLocation among %x", opcodes(frames))
	}
	return jump
}

// assertLandedNear checks frame is the player's jump to within the 20-unit
// scatter of spot.
func assertLandedNear(t *testing.T, frame []byte, spot location.Location) {
	t.Helper()
	r := wire.NewReader(frame[5:])
	x, y, z := int(r.ReadInt32()), int(r.ReadInt32()), int(r.ReadInt32())
	if x < spot.X-20 || x > spot.X+20 || y < spot.Y-20 || y > spot.Y+20 || z != spot.Z {
		t.Fatalf("teleported to %d,%d,%d, want within 20 of %+v", x, y, z, spot)
	}
}

// savedCount flushes item writes and returns the persisted count of
// templateID.
func (w *folkWorld) savedCount(t *testing.T, templateID int32) int {
	t.Helper()
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	n := 0
	for _, row := range rows {
		if row.TemplateID == templateID {
			n += row.Count
		}
	}
	return n
}

// teleportWindow is the list of standard destinations, built by hand from
// the reference's showTeleportWindow: the &$556; title, then per standard
// destination a link to "teleport <index>" (index counting every
// destination of the NPC) asking msg 811 about it, with " - <price>
// &#<item>;" for a positive price unless teleports are free.
func teleportWindow(f *npc.Folk, paidPrice, freePrice int) string {
	id := strconv.Itoa(int(f.ObjectID()))
	price := func(n int) string {
		if n <= 0 {
			return ""
		}
		return " - " + strconv.Itoa(n) + " &#57;"
	}
	return `<html><body>&$556;<br><br>` +
		`<a action="bypass -h npc_` + id + `_teleport 0" msg="811;Town of Dion">Town of Dion` + price(paidPrice) + `</a><br1>` +
		`<a action="bypass -h npc_` + id + `_teleport 2" msg="811;Free Spot">Free Spot` + price(freePrice) + `</a><br1>` +
		`</body></html>`
}

var (
	pageAnswer = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	// jumped is the player's teleport, opened by the releases of its own
	// stops: Creature.teleportTo's abortAll (Creature.java:386-429,
	// 1298-1306) stops the attack and the cast with the player already
	// teleporting, and each stop answers ActionFailed twice, its refused
	// tryToIdle's (PlayableAI.java:354-360) and its own (PlayerAttack.java:
	// 58-63, PlayerCast.java:381-387). The abort's target reset answers one
	// more even with nothing selected (Player.setTarget(null),
	// Player.java:2497-2499).
	jumped = [][]byte{
		{serverpackets.OpcodeActionFailed},
		{serverpackets.OpcodeActionFailed},
		{serverpackets.OpcodeActionFailed},
		{serverpackets.OpcodeActionFailed},
		{serverpackets.OpcodeActionFailed},
		{serverpackets.OpcodeTeleportToLocation},
	}
	// releasedTwice is a trip's own release, then the dispatcher's.
	releasedTwice = [][]byte{{serverpackets.OpcodeActionFailed}, {serverpackets.OpcodeActionFailed}}
)

// trip is a paid or free teleport's answer: its messages, the jump, then
// both releases.
func trip(messages ...[]byte) [][]byte {
	out := append(append([][]byte{}, messages...), jumped...)
	return append(out, releasedTwice...)
}

// TestBypassTeleportRequestListsStandardDestinations pins teleport_request:
// the list of the gatekeeper's standard destinations opens, released once
// by the dispatcher. Prices are halved, rounding down but never below 1,
// on Saturday and Sunday from 20:00, so the free destination then shows a
// price of 1, though taking it stays free.
func TestBypassTeleportRequestListsStandardDestinations(t *testing.T) {
	t.Parallel()
	clock := newTeleportClock()
	w, gk := bootGatekeeperWorld(t, 0, 0, false, clock)

	w.talkTo(t, gk)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, teleportWindow(gk, tripPrice, 0))

	clock.set(time.Date(2026, time.July, 11, 20, 0, 0, 0, time.Local)) // Saturday
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, teleportWindow(gk, tripPrice/2, 1))
	// The talk left the gatekeeper selected, and the jump's own deselection
	// adds frames of its own; only the trip's part is pinned here.
	frames := w.tripFrames(t, npcCommand(gk, "teleport 2"))
	jump, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation)
	if _, paid := firstOpcode(frames, serverpackets.OpcodeSystemMessage); !ok || paid {
		t.Fatalf("free trip at half price = %x, want a jump and no payment", opcodes(frames))
	}
	assertLandedNear(t, jump, freeSpot)
	assertFrames(t, "free trip releases", frames[len(frames)-2:], releasedTwice...)
}

// TestBypassTeleportFreeLeavesOutPrices pins FreeTeleport: the list shows
// no price and a priced destination moves a player holding nothing.
func TestBypassTeleportFreeLeavesOutPrices(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, 0, 0, true, newTeleportClock())

	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, teleportWindow(gk, 0, 0))
	frames := w.tripFrames(t, npcCommand(gk, "teleport 0"))
	assertFrames(t, "free teleport", frames, trip()...)
	assertLandedNear(t, landing(t, frames), paidSpot)
}

// TestBypassTeleportPaysThenMoves pins a listed destination: its price is
// taken (S1_DISAPPEARED_ADENA), the player jumps there, scattered by up to
// 20, and is released twice, the trip's own ActionFailed then the
// dispatcher's. The adena left is persisted.
func TestBypassTeleportPaysThenMoves(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, 1500, 0, false, newTeleportClock())
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, teleportWindow(gk, tripPrice, 0))

	frames := w.tripFrames(t, npcCommand(gk, "teleport 0"))
	assertFrames(t, "teleport 0", frames, trip(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(tripPrice)))...)
	assertLandedNear(t, landing(t, frames), paidSpot)
	if got := w.held(t, item.AdenaID); got != 500 {
		t.Fatalf("adena held = %d, want 500", got)
	}
	if got := w.savedCount(t, item.AdenaID); got != 500 {
		t.Fatalf("adena saved = %d, want 500", got)
	}
}

// TestBypassTeleportRefusesWhatCannotBePaid pins a destination the player
// cannot pay for: YOU_NOT_ENOUGH_ADENA, or NOT_ENOUGH_ITEMS for one priced
// in another item, then both releases; nothing is taken and nobody moves.
// A destination of price 0 still moves the player.
func TestBypassTeleportRefusesWhatCannotBePaid(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, tripPrice-1, 0, false, newTeleportClock())
	w.openAnyNpcPage(t)
	assertFrames(t, "unpaid adena", w.tripFrames(t, npcCommand(gk, "teleport 0")),
		append([][]byte{sysMsg(serverpackets.SystemMessageYouNotEnoughAdena)}, releasedTwice...)...)
	w.openAnyNpcPage(t)
	assertFrames(t, "unpaid token", w.tripFrames(t, npcCommand(gk, "teleport 1")),
		append([][]byte{sysMsg(serverpackets.SystemMessageNotEnoughItems)}, releasedTwice...)...)
	if got := w.held(t, item.AdenaID); got != tripPrice-1 {
		t.Fatalf("adena held = %d, want %d", got, tripPrice-1)
	}

	w.openAnyNpcPage(t)
	frames := w.tripFrames(t, npcCommand(gk, "teleport 2"))
	assertFrames(t, "free destination", frames, trip()...)
	assertLandedNear(t, landing(t, frames), freeSpot)
	if got := w.savedCount(t, item.AdenaID); got != tripPrice-1 {
		t.Fatalf("adena saved = %d, want %d", got, tripPrice-1)
	}
}

// TestBypassTeleportTakesPriceItems pins a destination priced in another
// item: the item is taken (S1_DISAPPEARED naming it, for a single one)
// before the jump, and the stack's removal is persisted.
func TestBypassTeleportTakesPriceItems(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, 0, 1, false, newTeleportClock())
	w.openAnyNpcPage(t)
	frames := w.tripFrames(t, npcCommand(gk, "teleport 1"))
	assertFrames(t, "token trip", frames, trip(sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(travelTokenID)))...)
	assertLandedNear(t, landing(t, frames), tokenSpot)
	if got := w.savedCount(t, travelTokenID); got != 0 {
		t.Fatalf("tokens saved = %d, want 0", got)
	}
}

// sealedDestinations is the fixture gatekeeper offering a standard
// destination priced in ancient adena, whose price needs the Seven Signs
// seal owners, then one priced at two travel tokens.
func sealedDestinations() (travel.TeleportTable, travel.InstantTable) {
	return travel.TeleportTable{
		gatekeeperID: {
			{Location: paidSpot, Description: "Necropolis", Kind: travel.KindStandard, PriceID: int(item.AncientAdenaID), PriceCount: 100},
			{Location: tokenSpot, Description: "Dark Elf Village", Kind: travel.KindStandard, PriceID: travelTokenID, PriceCount: 2},
		},
	}, travel.InstantTable{}
}

// TestBypassTeleportAncientAdenaPrice pins a destination priced in ancient
// adena (TeleportLocation.getCalculatedPriceCount): the list shows, and the
// trip takes, 1.6 times its count, truncated, from anyone but a Gnosis
// follower, who during seal validation pays the count itself: signed up
// for the cabal owning the Seal of Gnosis, having chosen that seal. The
// ancient adena taken is named with its count. A destination priced at
// more than one item says S2_S1_DISAPPEARED with the item and the count
// taken.
func TestBypassTeleportAncientAdenaPrice(t *testing.T) {
	t.Parallel()
	teleports, instants := sealedDestinations()
	w, gk := bootTravelWorld(t, teleports, instants, false, newTeleportClock(),
		map[int32]int32{item.AncientAdenaID: 500, item.AdenaID: 1000, travelTokenID: 3})
	// relocate lands the player where a trip took it and places a
	// gatekeeper beside it.
	relocate := func() {
		w.c.Send(encodeAppearing())
		drainFrames(t, w.c)
		x, y, z := w.srv.PlayerPosition(t, w.player)
		w.at = location.Location{X: x, Y: y, Z: z}
		gk = w.spawnFolk(t, folkTemplate("Gatekeeper", gatekeeperID), 50)
	}
	window := func(price int) string {
		id := strconv.Itoa(int(gk.ObjectID()))
		return `<html><body>&$556;<br><br>` +
			`<a action="bypass -h npc_` + id + `_teleport 0" msg="811;Necropolis">Necropolis - ` + strconv.Itoa(price) + ` &#5575;</a><br1>` +
			`<a action="bypass -h npc_` + id + `_teleport 1" msg="811;Dark Elf Village">Dark Elf Village - 2 &#8542;</a><br1>` +
			`</body></html>`
	}

	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, window(160))
	frames := w.tripFrames(t, npcCommand(gk, "teleport 0"))
	assertFrames(t, "surcharged trip", frames, trip(sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(item.AncientAdenaID), itemNumberParam(160)))...)
	assertLandedNear(t, landing(t, frames), paidSpot)
	relocate()

	// A Dawn member who chose the Seal of Gnosis, which the Dawn owns.
	setSevenSigns(t, w, sevensigns.SealValidation, sevensigns.Dawn, sevensigns.Dawn, sevensigns.NoCabal, sevensigns.Dawn)
	if _, err := w.srv.DB.ExecContext(context.Background(), `UPDATE seven_signs SET seal = 'GNOSIS' WHERE char_obj_id = ?`, w.player); err != nil {
		t.Fatal(err)
	}
	if err := w.srv.SevenSigns.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, window(100))
	frames = w.tripFrames(t, npcCommand(gk, "teleport 0"))
	assertFrames(t, "follower trip", frames, trip(sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(item.AncientAdenaID), itemNumberParam(100)))...)
	relocate()
	if got := w.savedCount(t, item.AncientAdenaID); got != 240 {
		t.Fatalf("ancient adena saved = %d, want 240", got)
	}

	// Outside seal validation the follower pays the surcharge too; 240
	// ancient adena cannot pay 160 twice.
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.Dawn, sevensigns.Dawn, sevensigns.NoCabal, sevensigns.Dawn)
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, window(160))
	w.tripFrames(t, npcCommand(gk, "teleport 0"))
	relocate()
	w.openAnyNpcPage(t)
	assertFrames(t, "short of ancient adena", w.tripFrames(t, npcCommand(gk, "teleport 0")),
		sysMsg(serverpackets.SystemMessageNotEnoughItems), []byte{serverpackets.OpcodeActionFailed}, []byte{serverpackets.OpcodeActionFailed})
	if got := w.savedCount(t, item.AncientAdenaID); got != 80 {
		t.Fatalf("ancient adena saved = %d, want 80", got)
	}

	w.openAnyNpcPage(t)
	frames = w.tripFrames(t, npcCommand(gk, "teleport 1"))
	assertFrames(t, "two-token trip", frames, trip(sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(travelTokenID), itemNumberParam(2)))...)
	assertLandedNear(t, landing(t, frames), tokenSpot)
	if got := w.savedCount(t, travelTokenID); got != 1 {
		t.Fatalf("tokens saved = %d, want 1", got)
	}
}

// TestBypassTeleportFreeListsAncientAdenaUnpriced pins FreeTeleport over a
// destination priced in ancient adena: no price is read, so the list shows
// it without one and taking it moves the player, nothing taken.
func TestBypassTeleportFreeListsAncientAdenaUnpriced(t *testing.T) {
	t.Parallel()
	teleports, instants := sealedDestinations()
	w, gk := bootTravelWorld(t, teleports, instants, true, newTeleportClock(), map[int32]int32{item.AncientAdenaID: 500})

	id := strconv.Itoa(int(gk.ObjectID()))
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, `<html><body>&$556;<br><br>`+
		`<a action="bypass -h npc_`+id+`_teleport 0" msg="811;Necropolis">Necropolis</a><br1>`+
		`<a action="bypass -h npc_`+id+`_teleport 1" msg="811;Dark Elf Village">Dark Elf Village</a><br1>`+
		`</body></html>`)
	frames := w.tripFrames(t, npcCommand(gk, "teleport 0"))
	assertFrames(t, "free sealed trip", frames, trip()...)
	assertLandedNear(t, landing(t, frames), paidSpot)
	if got := w.savedCount(t, item.AncientAdenaID); got != 500 {
		t.Fatalf("ancient adena saved = %d, want 500", got)
	}
}

// TestBypassTeleportIndexEdges pins the destination index: one past the
// last destination, or an NPC without a list, answers only the
// dispatcher's release; a negative index, the list's own length, or an
// index missing or not a number add the command's own release. The same
// holds for instant_teleport, whose valid index jumps with the dispatcher's
// release alone; teleport_request at an NPC without a list opens nothing.
func TestBypassTeleportIndexEdges(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, 0, 0, false, newTeleportClock())
	plain := w.spawnFolk(t, folkTemplate("Gatekeeper", 30081), 60)
	twice := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}

	for _, tc := range []struct {
		f       *npc.Folk
		command string
		want    []byte
	}{
		{gk, "teleport 4", releaseOnly},
		{gk, "teleport 3", twice},
		{gk, "teleport -1", twice},
		{gk, "teleport", twice},
		{gk, "teleport x", twice},
		{plain, "teleport 0", releaseOnly},
		{plain, "teleport x", twice},
		{plain, "teleport_request", releaseOnly},
		{gk, "instant_teleport 2", releaseOnly},
		{gk, "instant_teleport 1", twice},
		{gk, "instant_teleport -1", twice},
		{gk, "instant_teleport", twice},
		{plain, "instant_teleport 0", releaseOnly},
	} {
		w.openAnyNpcPage(t)
		assertAnswer(t, w.tripFrames(t, npcCommand(tc.f, tc.command)), tc.want, tc.f, "")
	}

	w.openAnyNpcPage(t)
	frames := w.tripFrames(t, npcCommand(gk, "instant_teleport 0"))
	assertFrames(t, "instant_teleport 0", frames, append(append([][]byte{}, jumped...), []byte{serverpackets.OpcodeActionFailed})...)
	assertLandedNear(t, landing(t, frames), instantSpot)
}

// TestBypassAdventurerCommands pins an adventurer guildsman's own
// commands: raidInfo 0 opens the raid overview and raidInfo <n> the level
// n page, each as a chat window; a missing page reads as the html-missing
// notice; a raidInfo without a level, or with one that does not parse,
// aborts with nothing sent. questlist, in any case, opens the quest
// information window, then the dispatcher releases.
func TestBypassAdventurerCommands(t *testing.T) {
	t.Parallel()
	pages := dialogPages()
	pages["adventurer_guildsman/raid_info/info.htm"] = "<html><body>Raid bosses %objectId%</body></html>"
	pages["adventurer_guildsman/raid_info/level40.htm"] = "<html><body>Level 40 bosses</body></html>"
	w := bootFolkWorld(t, pages, noBypassReuse)
	guide := w.spawnFolk(t, folkTemplate("Adventurer", 31729), 50)

	for _, tc := range []struct {
		command string
		want    []byte
		html    string
	}{
		{"raidInfo 0", chatWindowAnswer, wantChatPage(pages["adventurer_guildsman/raid_info/info.htm"], guide)},
		{"raidInfo 40", chatWindowAnswer, wantChatPage(pages["adventurer_guildsman/raid_info/level40.htm"], guide)},
		{"raidInfo 41", chatWindowAnswer, "<html><body>My html is missing:<br>data/html/adventurer_guildsman/raid_info/level41.htm</body></html>"},
		{"raidInfo", nil, ""},
		{"raidInfo x", nil, ""},
	} {
		w.openAnyNpcPage(t)
		assertAnswer(t, w.bypass(t, npcCommand(guide, tc.command)), tc.want, guide, tc.html)
	}
	for _, command := range []string{"questlist", "QuestList"} {
		w.openAnyNpcPage(t)
		frames := w.bypass(t, npcCommand(guide, command))
		assertFrames(t, command, frames, []byte{serverpackets.OpcodeExtended, byte(serverpackets.OpcodeExShowQuestInfo), 0}, []byte{serverpackets.OpcodeActionFailed})
	}
}

// TestBypassTeleportListShippedGatekeeper pins the list of a shipped
// gatekeeper, Dion's 30080, against its teleports.xml rows read by hand:
// its fourteen standard destinations at weekday prices, then none of the
// noble hunting zone ones that follow them.
func TestBypassTeleportListShippedGatekeeper(t *testing.T) {
	t.Parallel()
	teleports, err := gamexml.LoadTeleports(datapack.Path(t, "data", "xml", "teleports.xml"))
	if err != nil {
		t.Fatalf("load teleports.xml: %v", err)
	}
	clock := newTeleportClock()
	pages := dialogPages()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithTeleports(teleports, nil, false, clock.now),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	gk := w.spawnFolk(t, folkTemplate("Gatekeeper", gatekeeperID), 50)

	id := strconv.Itoa(int(gk.ObjectID()))
	want := `<html><body>&$556;<br><br>`
	for i, row := range []struct {
		desc  string
		price int
	}{
		{"Town of Oren", 9400},
		{"Heine", 7600},
		{"The Town of Dion", 6800},
		{"Town of Goddard", 63000},
		{"Rune Township", 59000},
		{"Town of Schuttgart", 87000},
		{"The Town of Gludio", 29000},
		{"Town of Aden", 13000},
		{"Giran Harbor", 5200},
		{"Hardin's Private Academy", 4400},
		{"Dragon Valley", 1800},
		{"Antharas' Lair", 7000},
		{"Devil's Isle", 5700},
		{"Breka's Stronghold", 1000},
	} {
		want += `<a action="bypass -h npc_` + id + `_teleport ` + strconv.Itoa(i) + `" msg="811;` + row.desc + `">` +
			row.desc + ` - ` + strconv.Itoa(row.price) + ` &#57;</a><br1>`
	}
	want += `</body></html>`
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, want)
}
