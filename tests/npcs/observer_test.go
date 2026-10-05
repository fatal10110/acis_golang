package npcs

import (
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/observer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// broadcastingTowerID is the shipped broadcasting tower, a plain civilian.
const broadcastingTowerID = 31031

// Shipped viewpoints these scenarios use (observerGroups.xml): group 619
// is the colosseum, 80 adena a seat; 612 holds Gludio castle's two
// viewpoints, 500 adena each; locId 819 is listed under both 929 and 930.
var (
	colosseumSeat = location.Location{X: 148416, Y: 46724, Z: -3000} // locId 634, yaw 0
	group929Seat  = location.Location{X: -80206, Y: 87891, Z: -4852} // locId 819 in group 929, yaw 16384
)

const colosseumFee = 80

func shippedObserverGroups(t *testing.T) *observer.Table {
	t.Helper()
	table, err := gamexml.LoadObserverGroups(datapack.Path(t, "data", "xml", "observerGroups.xml"))
	if err != nil {
		t.Fatalf("load observer groups: %v", err)
	}
	return table
}

// bootTowerWorld enters the world holding adena next to a broadcasting
// tower offering the shipped groups 612 and 619.
func bootTowerWorld(t *testing.T, adena int32, extra ...gameservertest.Option) (*folkWorld, *npc.Folk) {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithObserverGroups(shippedObserverGroups(t)),
		gameservertest.WithItemTemplates(travelTemplates()),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(dialogPages()),
	}, opts...)...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	tower := w.spawnFolk(t, folkTemplate("Folk", broadcastingTowerID), 50)
	tower.SetObserverGroups([]int{612, 619})
	return w, tower
}

func (w *folkWorld) character(t *testing.T) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatalf("player %d not in the world", w.player)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is %T, not an online character", w.player, obj)
	}
	return c
}

// observeFrames sends command and keeps the frames observer entry and
// rejection are judged by.
func (w *folkWorld) observeFrames(t *testing.T, command string) [][]byte {
	t.Helper()
	return w.observerKept(w.bypass(t, command))
}

func (w *folkWorld) observerKept(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed,
			serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeTargetUnselected,
			serverpackets.OpcodeObserverStart, serverpackets.OpcodeObserverEnd:
			out = append(out, f)
		case serverpackets.OpcodeTeleportToLocation:
			if int32(binary.LittleEndian.Uint32(f[1:5])) == w.player {
				out = append(out, f)
			}
		}
	}
	return out
}

func encodeObserverReturn() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeObserverReturn).Bytes()
}

func encodeAppearing() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes()
}

// TestObserverTowerTalkListsGroups pins a broadcasting tower's first page:
// its groups as links in place of its chat window, with no release after
// it, where a chat window is followed by ActionFailed.
func TestObserverTowerTalkListsGroups(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 0)
	w.selectFolk(t, tower)
	frames := w.talk(t, tower, false)
	page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("talk = %x, want a page", opcodes(frames))
	}
	if order := interactOrder(frames); order[len(order)-1] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("talk order = %x, want the page last", order)
	}
	oid := itoa(tower.ObjectID())
	want := `<html><body>&$650;<br><br>` +
		`<a action="bypass -h npc_` + oid + `_observe_group 612">&$612;</a><br1>` +
		`<a action="bypass -h npc_` + oid + `_observe_group 619">&$619;</a><br1>` +
		`</body></html>`
	if objectID, html, itemID := htmlMessage(t, page); objectID != tower.ObjectID() || itemID != 0 || html != want {
		t.Fatalf("page = object %d item %d %q, want %d/0 %q", objectID, itemID, html, tower.ObjectID(), want)
	}
}

// TestObserverGroupListsViewpoints pins observe_group: the group's
// viewpoints with their fee, then the dispatcher's release. An unknown
// group is only released; a command missing its id, or whose id does not
// parse, is dropped with nothing sent.
func TestObserverGroupListsViewpoints(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 0)
	w.talkTo(t, tower)
	oid := itoa(tower.ObjectID())
	want := `<html><body>&$650;<br><br>` +
		`<a action="bypass -h npc_` + oid + `_observe 634">&$634; - 80 &#57;</a><br1>` +
		`<a action="bypass -h npc_` + oid + `_observe 635">&$635; - 80 &#57;</a><br1>` +
		`<a action="bypass -h npc_` + oid + `_observe 636">&$636; - 80 &#57;</a><br1>` +
		`</body></html>`
	assertAnswer(t, w.observeFrames(t, npcCommand(tower, "observe_group 619")), pageAnswer, tower, want)

	w.openAnyNpcPage(t)
	assertAnswer(t, w.observeFrames(t, npcCommand(tower, "observe_group 999")), releaseOnly, tower, "")
	w.openAnyNpcPage(t)
	assertAnswer(t, w.observeFrames(t, npcCommand(tower, "observe_group")), nil, tower, "")
	w.openAnyNpcPage(t)
	assertAnswer(t, w.observeFrames(t, npcCommand(tower, "observe_group x")), nil, tower, "")
}

// TestObserverEntryAndReturn walks a paid viewpoint end to end: the fee
// taken, the stand-up, the abort answered as a teleport's twice, the first
// resetting the selected tower (Player.enterObserverMode's abortAll(true),
// Player.java:5260-5263), the jump, ObserverStart, then the dispatcher's
// release. Watching, the player is hidden, invulnerable and
// paralyzed, its position reports are ignored and its clicks refused.
// ObserverReturn brings it back where it left.
func TestObserverEntryAndReturn(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 1000)
	w.talkTo(t, tower)
	w.observeFrames(t, npcCommand(tower, "observe_group 619"))
	left := w.at

	af := []byte{serverpackets.OpcodeActionFailed}
	frames := w.observeFrames(t, npcCommand(tower, "observe 634"))
	assertFrames(t, "observe 634", frames,
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(colosseumFee)),
		[]byte{serverpackets.OpcodeChangeWaitType},
		af, af, af, af, af, []byte{serverpackets.OpcodeTargetUnselected},
		af, af, af, af, af,
		[]byte{serverpackets.OpcodeTeleportToLocation},
		observerStart(colosseumSeat, 0, 0),
		af)
	assertLandedNear(t, landing(t, frames), colosseumSeat)
	c := w.character(t)
	if !c.ObserverMode() || !c.Invisible() || !c.Invul() || !c.Paralyzed() {
		t.Fatalf("observer=%v invisible=%v invul=%v paralyzed=%v, want all on", c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed())
	}
	if saved, ok := c.SavedLocation(); !ok || saved != left {
		t.Fatalf("saved location = %v %v, want %v", saved, ok, left)
	}
	if got := w.held(t, item.AdenaID); got != 1000-colosseumFee {
		t.Fatalf("adena held = %d, want %d", got, 1000-colosseumFee)
	}
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	seat := w.character(t).CurrentLocation()

	// The client reports its camera: no correction, no fall damage, and
	// the character stays at the seat.
	w.c.Send(encodeValidatePosition(location.Location{X: seat.X + 2000, Y: seat.Y, Z: seat.Z + 600}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("ValidatePosition while observing = %x, want silence", opcodes(frames))
	}
	if got := w.character(t).CurrentLocation(); got != seat {
		t.Fatalf("position after ValidatePosition = %v, want %v", got, seat)
	}
	// A click is refused with OBSERVERS_CANNOT_PARTICIPATE; an attack
	// request, from a player unable to act, is only released; so is an
	// action-bar command.
	w.c.Send(encodeAction(w.player, seat, false))
	assertFrames(t, "Action while observing", drainFrames(t, w.c), sysMsg(serverpackets.SystemMessageObserversCannotParticipate), af)
	w.c.Send(encodeAttackRequest(w.player, seat))
	assertFrames(t, "AttackRequest while observing", drainFrames(t, w.c), af)
	w.c.Send(encodeActionUse(0))
	assertFrames(t, "RequestActionUse while observing", drainFrames(t, w.c), af)

	// Back: the refused idle and the cleared selection each release, then
	// ObserverEnd names the position left, and the jump there.
	w.c.Send(encodeObserverReturn())
	frames = w.observerKept(drainFrames(t, w.c))
	assertFrames(t, "ObserverReturn", frames,
		af, af,
		observerEnd(left),
		af, af, af, af, af,
		[]byte{serverpackets.OpcodeTeleportToLocation})
	assertLandedNear(t, landing(t, frames), left)
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	c = w.character(t)
	if c.ObserverMode() || c.Invisible() || c.Invul() || c.Paralyzed() {
		t.Fatalf("observer=%v invisible=%v invul=%v paralyzed=%v, want all off", c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed())
	}
	if _, ok := c.SavedLocation(); ok {
		t.Fatal("saved location kept after the return")
	}
	// Not observing any more: ObserverReturn is ignored.
	w.c.Send(encodeObserverReturn())
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("ObserverReturn outside observer mode = %x, want silence", opcodes(frames))
	}
}

func observerStart(at location.Location, yaw, pitch int32) []byte {
	return append([]byte{serverpackets.OpcodeObserverStart}, le32(int32(at.X), int32(at.Y), int32(at.Z), yaw, pitch)...)
}

func observerEnd(at location.Location) []byte {
	return append([]byte{serverpackets.OpcodeObserverEnd}, le32(int32(at.X), int32(at.Y), int32(at.Z))...)
}

func encodeActionUse(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

func itoa(n int32) string {
	return strconv.Itoa(int(n))
}

// TestObserveRefusals pins the viewpoint gates: a castle viewpoint is
// closed while its castle is not under siege (ONLY_VIEW_SIEGE), a player
// in combat is refused (CANNOT_OBSERVE_IN_COMBAT), one short of the fee
// hears YOU_NOT_ENOUGH_ADENA, an unknown viewpoint is only released, and
// a command missing its id or whose id does not parse is dropped. Nobody
// moves and nothing is paid.
func TestObserveRefusals(t *testing.T) {
	t.Parallel()
	af := []byte{serverpackets.OpcodeActionFailed}
	w, tower := bootTowerWorld(t, colosseumFee-1)
	refused := func(command string, want ...[]byte) {
		t.Helper()
		w.openAnyNpcPage(t)
		assertFrames(t, command, w.observeFrames(t, npcCommand(tower, command)), want...)
	}
	refused("observe 620", sysMsg(serverpackets.SystemMessageOnlyViewSiege), af)
	refused("observe 634", sysMsg(serverpackets.SystemMessageYouNotEnoughAdena), af)
	refused("observe 9999", af)
	refused("observe")
	refused("observe x")
	w.srv.SetPlayerInCombat(t, w.player, true)
	refused("observe 634", sysMsg(serverpackets.SystemMessageCannotObserveInCombat), af)

	if c := w.character(t); c.ObserverMode() || c.Invisible() {
		t.Fatalf("observer=%v invisible=%v after refusals, want off", c.ObserverMode(), c.Invisible())
	}
	if got := w.held(t, item.AdenaID); got != colosseumFee-1 {
		t.Fatalf("adena held = %d, want %d", got, colosseumFee-1)
	}
}

// TestObserveSharedViewpointIdTakesLowestGroup pins a viewpoint id two
// groups share (819 under 929 and 930): it resolves to group 929's seat.
func TestObserveSharedViewpointIdTakesLowestGroup(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 1000)
	if loc, ok := shippedObserverGroups(t).Location(819); !ok || loc.Location != group929Seat || loc.Yaw != 16384 {
		t.Fatalf("Location(819) = %+v %v, want group 929's seat", loc, ok)
	}
	if _, ok := shippedObserverGroups(t).Location(9999); ok {
		t.Fatal("Location(9999) found, want missing")
	}
	w.openAnyNpcPage(t)
	frames := w.observeFrames(t, npcCommand(tower, "observe 819"))
	if start, ok := firstOpcode(frames, serverpackets.OpcodeObserverStart); !ok || string(start) != string(observerStart(group929Seat, 16384, 0)) {
		t.Fatalf("observe 819 = %x, want ObserverStart at group 929's seat", opcodes(frames))
	}
	assertLandedNear(t, landing(t, frames), group929Seat)
}

// TestObserverLogsOutWhereItLeft pins the save of an observer leaving the
// world: its stored position is the one it left to watch, not the seat.
func TestObserverLogsOutWhereItLeft(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 1000)
	left := w.at
	w.openAnyNpcPage(t)
	w.observeFrames(t, npcCommand(tower, "observe 634"))
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)

	// Clear the stored row, so only the logout save can write it back.
	if _, err := w.srv.DB.ExecContext(t.Context(), "UPDATE characters SET x = 0, y = 0, z = 0 WHERE obj_Id = ?", w.player); err != nil {
		t.Fatalf("clear position: %v", err)
	}
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	w.srv.AdvanceUntil(t, "logout save", func() bool {
		var at location.Location
		if err := w.srv.DB.QueryRowContext(t.Context(), "SELECT x, y, z FROM characters WHERE obj_Id = ?", w.player).Scan(&at.X, &at.Y, &at.Z); err != nil {
			t.Fatalf("read position: %v", err)
		}
		if at != (location.Location{}) && at != left {
			t.Fatalf("stored position = %v, want %v", at, left)
		}
		return at == left
	})
}

// TestObserverTowersSpawnFromTheTable pins the boot placement of the
// broadcasting towers: each spawn of a known template is placed with its
// groups and talks with them; a spawn naming an unknown template is
// skipped before anything is built.
func TestObserverTowersSpawnFromTheTable(t *testing.T) {
	t.Parallel()
	tower := folkTemplate("Folk", broadcastingTowerID)
	w := bootFolkWorld(t, dialogPages(),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tower})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithObserverGroups(shippedObserverGroups(t)),
	)
	at := location.Location{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}
	table := observer.NewTable(nil, []observer.Spawn{
		{NPCID: 99999, Location: at, Groups: []int{612}},
		{NPCID: broadcastingTowerID, Location: at, Groups: []int{618, 612}},
	})
	if placed := w.srv.NpcSpawns.SpawnObservers(table); placed != 1 {
		t.Fatalf("SpawnObservers placed %d, want 1", placed)
	}
	var towers []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok {
			towers = append(towers, f)
		}
	}
	if len(towers) != 1 || towers[0].NpcID() != broadcastingTowerID {
		t.Fatalf("civilians in the world = %v, want one broadcasting tower", towers)
	}
	if groups := towers[0].ObserverGroups(); len(groups) != 2 || groups[0] != 618 || groups[1] != 612 {
		t.Fatalf("tower groups = %v, want [618 612]", groups)
	}
	drainUntilQuiet(t, w.c)
	w.selectFolk(t, towers[0])
	page, ok := firstOpcode(w.talk(t, towers[0], false), serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatal("talk to the tower opened no page")
	}
	if _, html, _ := htmlMessage(t, page); html != observer.GroupsWindow(towers[0].ObjectID(), []int{618, 612}) {
		t.Fatalf("tower page = %q, want its groups", html)
	}
}
