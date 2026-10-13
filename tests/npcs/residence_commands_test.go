package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: the onBypassFeedback and showChatWindow overrides of the
// castle NPCs.
//   - CastleWarehouseKeeper (CastleWarehouseKeeper.java:23-48): Chat <n>
//     sends ActionFailed, then castlewarehouse-<n>.htm to a member of the
//     owning clan, -no.htm to anyone else, with %npcname%.
//   - CastleBlacksmith (CastleBlacksmith.java:17-49): every command answers
//     npcdefault.htm while AllowManor is off, nothing to a talker who may not
//     run the manor or during the siege; Chat <n> opens
//     castleblacksmith-<n>.htm alone.
//   - CastleMagician (CastleMagician.java:48-132): Chat <n> as the warehouse
//     keeper; gotoleader sends a clan member to its online leader's clan
//     gate, magician-nogate.htm when it has none, refusing with a plain
//     message when the leader cannot be reached or, from a Seven Signs
//     dungeon, is of the wrong cabal.
//   - Doorman / CastleDoorman (Doorman.java:27-115, CastleDoorman.java:20-61):
//     open_doors / close_doors work the doors named; a plain doorman skips
//     the first id of its list and stops at an unknown door; a castle
//     doorman passes over a door that is no gate of its castle, refuses
//     non-owners silently, and during the siege sends busy.htm, ActionFailed
//     and GATES_NOT_OPENED_CLOSED_DURING_SIEGE.
//   - MercenaryManagerNpc (MercenaryManagerNpc.java:26-75): every command
//     answers nothing to a talker who may not hire mercenaries; back,
//     how_to, merc_limit and hire <n> (seal validation only; buylist
//     <npcId><n>, untaxed, then mseller004.htm).
//   - CastleGatekeeper (CastleGatekeeper.java:27-87): tele arms one
//     teleport of the defenders, MassGK-1.htm naming the delay (30 s, 480 s
//     during the siege with no life tower standing); when it fires, the
//     region hears a shout during the siege, and the castle's mass teleport
//     room is emptied to its exit point.
//   - WyvernManagerNpc (WyvernManagerNpc.java:28-138): page 0 (4 while
//     flying) for the residence owner's leader, 2 for another member, 0a
//     for anyone else, each then ActionFailed; Chat <val> opens page val;
//     only the talker who last selected it is heard.
//   - ManorManagerNpc (ManorManagerNpc.java:84-88): every chat page is
//     manager.htm.

const (
	massGatekeeperID = 35095
	wyvernManagerID  = 35140
	castleGateA      = 19_210_001
	castleGateB      = 19_210_002
	frontierGate     = 24_170_001
	clanGateSkillID  = 3632
)

// commandSetup is residenceSetup with the Seven Signs period, the castle's
// mass teleport room around the players, and its gates.
type commandSetup struct {
	residenceSetup
	period sevensigns.Period
	room   bool
	gates  bool
	extra  []gameservertest.Option
}

// commandCastles is Gludio listing the castle NPCs and the mass gatekeeper
// and wyvern manager, with two gates.
func commandCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
		Tax:   residence.Tax{Rate: taxCastleTax},
		Gates: []string{"gludio_castle_gate_a", "gludio_castle_gate_b"},
		NPCs: []int{
			castleChamberlainID, castleBlacksmithID, castleMagicianID, castleWarehouseID,
			castleMercenaryID, castleManorID, castleDoormanID, massGatekeeperID, wyvernManagerID,
		},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// commandPage is the page of file, naming itself and every placeholder a
// castle NPC fills.
func commandPage(file string) string {
	return "<html><body>" + file + " %objectId% %npcname% %castleid% %delay% %castleName% %wyvern_level% %needed_crystals%</body></html>"
}

func commandPages() map[string]string {
	pages := map[string]string{"test/any.htm": anyNpcPage}
	for _, file := range []string{
		"npcdefault.htm", "chamberlain/noprivs.htm", "manormanager/manager.htm",
		"castlewarehouse/castlewarehouse-1.htm", "castlewarehouse/castlewarehouse-no.htm", "castlewarehouse/castlewarehouse.htm",
		"castleblacksmith/castleblacksmith-1.htm",
		"castlemagician/magician.htm", "castlemagician/magician-1.htm", "castlemagician/magician-nogate.htm",
		"doormen/35096.htm", "doormen/30596.htm", "doormen/busy.htm",
		"mercmanager/mseller001.htm", "mercmanager/mseller004.htm", "mercmanager/mseller005.htm",
		"mercmanager/msellerLimit.htm", "mercmanager/msellerdenial.htm",
		"castleteleporter/MassGK.htm", "castleteleporter/MassGK-1.htm", "castleteleporter/MassGK-2.htm",
		"wyvernmanager/wyvernmanager-0.htm", "wyvernmanager/wyvernmanager-0a.htm", "wyvernmanager/wyvernmanager-1.htm",
		"wyvernmanager/wyvernmanager-2.htm", "wyvernmanager/wyvernmanager-3.htm",
	} {
		pages[file] = commandPage(file)
	}
	return pages
}

// gludioGates are the two gates of commandCastles and a frontier gate, all
// closed, beside the players.
func gludioGates() gameservertest.Option {
	gate := func(id int, name string, x int) *door.Template {
		return &door.Template{
			ID: id, Name: name, Kind: door.KindDoor, Level: 1,
			Position: location.Location{X: x, Y: 100, Z: gameservertest.SpawnZ},
			Coordinates: []location.Point{
				{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
			},
			HP: 100, PDef: 10, MDef: 10, Height: 32, OpenKind: door.OpenNPC,
		}
	}
	return gameservertest.WithDoors(
		gate(castleGateA, "gludio_castle_gate_a", -60),
		gate(castleGateB, "gludio_castle_gate_b", 80),
		gate(frontierGate, "frontier_gate", 160),
	)
}

// massRoomExit is where the mass teleport room sends its occupants.
var massRoomExit = location.Location{X: 5000, Y: 5000, Z: gameservertest.SpawnZ}

func commandZones(t *testing.T, s commandSetup) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(-2000, 2000, -2000, 2000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	if s.siege {
		field := zone.NewSiege(1, form)
		field.ResidenceID = taxCastleID
		zones.Add(field)
	}
	if s.room {
		room := zone.NewCastleTeleport(2, form)
		room.CastleID = taxCastleID
		room.ExitMinX, room.ExitMaxX = massRoomExit.X, massRoomExit.X
		room.ExitMinY, room.ExitMaxY = massRoomExit.Y, massRoomExit.Y
		room.ExitZ = massRoomExit.Z
		zones.Add(room)
	}
	return zones
}

// bootResidenceCommands boots the world s describes, the player in it.
func bootResidenceCommands(t *testing.T, s commandSetup) *folkWorld {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithCastles(commandCastles(t)),
		gameservertest.WithManor(network.ManorConfig{Allowed: !s.manorOff}),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedResidenceChat(t, db, s.residenceSetup) }),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load seven signs status: found=%v err=%v", found, err)
			}
			row.SealOwners[2] = s.strife
			row.Period = s.period
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed seven signs status: %v", err)
			}
		}),
		gameservertest.WithZones(commandZones(t, s)),
		noBypassReuse,
	}
	if s.siege {
		opts = append(opts, gameservertest.WithSieges(siege.DefaultConfig()))
	}
	if s.gates {
		opts = append(opts, gludioGates())
	}
	w := bootFolkWorld(t, commandPages(), append(opts, s.extra...)...)
	if s.siege {
		gludio, ok := w.srv.Sieges.Get(taxCastleID)
		if !ok {
			t.Fatal("no Gludio siege")
		}
		gludio.Start()
		if !gludio.InProgress() {
			t.Fatal("Gludio siege not under way")
		}
		drainUntilQuiet(t, w.c)
	}
	return w
}

// spawnKeeper spawns an NPC of kind named Keeper dx east of the player and
// talks to it, so that it is the NPC the player last selected.
func (w *folkWorld) spawnKeeper(t *testing.T, kind string, id, dx int) *npc.Folk {
	t.Helper()
	tmpl := folkTemplate(kind, id)
	tmpl.Name = "Keeper"
	f := w.spawnFolk(t, tmpl, dx)
	w.selectFolk(t, f)
	w.talk(t, f, false)
	return f
}

// command sends command to f from a page admitting it.
func (w *folkWorld) command(t *testing.T, f *npc.Folk, command string) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.bypass(t, npcCommand(f, command))
}

// filled is file's commandPage with pairs filled, then %objectId% naming f,
// as the page cache hands it out.
func filled(file string, f *npc.Folk, pairs ...string) string {
	return wantChatPage(fill(commandPage(file), pairs...), f)
}

// asRead is file's commandPage with pairs filled and nothing else, as the
// page cache hands it out.
func asRead(file string, pairs ...string) string {
	return fill(commandPage(file), pairs...) + "\n"
}

var (
	releasedChat  = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	pageThenFinal = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	chatThenFinal = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
)

// TestCastleWarehouseKeeperChatCommand pins Chat <n> at a castle warehouse
// keeper: its numbered page for a member of the owning clan, its refusal
// page for the leader of another clan, each after ActionFailed. Its storage
// commands are a warehouse keeper's, for anyone.
func TestCastleWarehouseKeeperChatCommand(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup residenceSetup
		file  string
	}{
		{"owner", residenceSetup{rank: 5}, "castlewarehouse/castlewarehouse-1.htm"},
		{"rival", residenceSetup{rival: true}, "castlewarehouse/castlewarehouse-no.htm"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootResidenceCommands(t, commandSetup{residenceSetup: c.setup})
			f := w.spawnKeeper(t, "CastleWarehouseKeeper", castleWarehouseID, 30)
			assertAnswer(t, w.command(t, f, "Chat 1"), releasedChat, f, filled(c.file, f, "%npcname%", "Keeper"))
			if _, ok := firstOpcode(w.command(t, f, "DepositP"), serverpackets.OpcodeWarehouseDepositList); !ok {
				t.Fatal("DepositP opened no deposit window")
			}
		})
	}
}

// TestCastleBlacksmithCommands pins the blacksmith's gates: every command
// opens the generic page while the manor is off; a member who may not run
// the manor, and the lord during the siege, are answered nothing; the lord
// gets its numbered page alone.
func TestCastleBlacksmithCommands(t *testing.T) {
	t.Parallel()
	release := []byte{serverpackets.OpcodeActionFailed}
	for _, c := range []struct {
		name  string
		setup residenceSetup
		want  []byte
		file  string
		pairs []string
	}{
		{"manor off", residenceSetup{manorOff: true}, pageThenFinal, "npcdefault.htm", []string{"%npcname%", "Keeper"}},
		{"member", residenceSetup{rank: 5}, release, "", nil},
		{"under siege", residenceSetup{siege: true}, release, "", nil},
		{"lord", residenceSetup{}, pageThenFinal, "castleblacksmith/castleblacksmith-1.htm", []string{"%npcname%", "Keeper", "%castleid%", strconv.Itoa(taxCastleID)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootResidenceCommands(t, commandSetup{residenceSetup: c.setup})
			f := w.spawnKeeper(t, "CastleBlacksmith", castleBlacksmithID, 30)
			assertAnswer(t, w.command(t, f, "Chat 1"), c.want, f, filled(c.file, f, c.pairs...))
			if c.setup.manorOff {
				assertAnswer(t, w.command(t, f, "multisell 1"), c.want, f, filled(c.file, f, c.pairs...))
			}
		})
	}
}

// TestCastleMagicianCommands pins the magician's Chat <n> for the lord, and
// gotoleader: the no-gate page and ActionFailed when the leader, online,
// opened no clan gate; nothing for a clanless talker.
func TestCastleMagicianCommands(t *testing.T) {
	t.Parallel()
	t.Run("lord", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{})
		f := w.spawnKeeper(t, "CastleMagician", castleMagicianID, 30)
		assertAnswer(t, w.command(t, f, "Chat 1"), releasedChat, f, filled("castlemagician/magician-1.htm", f))
		assertAnswer(t, w.command(t, f, "gotoleader"), chatThenFinal, f, filled("castlemagician/magician-nogate.htm", f))
	})
	t.Run("clanless", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{clanless: true}})
		f := w.spawnKeeper(t, "CastleMagician", castleMagicianID, 30)
		assertAnswer(t, w.command(t, f, "gotoleader"), []byte{serverpackets.OpcodeActionFailed}, f, "")
	})
}

// onPlayerQueue runs fn with the player's live character on its queue.
func (w *folkWorld) onPlayerQueue(t *testing.T, fn func(pc *player.Character)) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("player not in the world")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("player queue closed")
	}
	<-done
}

// TestCastleMagicianClanGateFromADungeon pins the gate check from a Seven
// Signs dungeon outside seal validation: the leader, here the talker
// itself, having opened a clan gate, is refused with the plain message
// while it belongs to no cabal, and reached once it signs up with one.
func TestCastleMagicianClanGateFromADungeon(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{period: sevensigns.Competition})
	f := w.spawnKeeper(t, "CastleMagician", castleMagicianID, 30)
	w.onPlayerQueue(t, func(pc *player.Character) {
		pc.SetIn7sDungeon(true)
		gate, err := effect.New(effect.Skill{ID: clanGateSkillID, Level: 1}, modelskill.EffectTemplate{Name: "ClanGate", Time: 600})
		if err != nil {
			t.Errorf("clan gate effect: %v", err)
			return
		}
		gate.Effector, gate.Effected = pc, pc
		pc.EffectList().Add(gate)
	})
	drainUntilQuiet(t, w.c)

	frames := w.command(t, f, "gotoleader")
	if got, want := opcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("gotoleader of no cabal = % x, want % x", got, want)
	}
	if got := systemMessageText(t, frames[0]); got != "Couldn't teleport to clan leader. The requirements was not meet." {
		t.Fatalf("refusal = %q", got)
	}

	if err := w.srv.SevenSigns.SetPlayerInfo(context.Background(), w.player, sevensigns.Dawn, sevensigns.Strife); err != nil {
		t.Fatalf("sign up: %v", err)
	}
	frames = w.command(t, f, "gotoleader")
	if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); !ok {
		t.Fatalf("gotoleader of the Dawn = % x, want a teleport", opcodes(frames))
	}
}

// TestCastleDoormanDoors pins a castle doorman's door commands for a member
// holding the castle's entry rights: each gate of the castle named opens or
// closes, an id naming no gate of it is passed over; a door list that does
// not parse stops after the gates before it, with no closing ActionFailed.
// A member without the rights is answered nothing, and the lord during the
// siege is told the gates stay as they are.
func TestCastleDoormanDoors(t *testing.T) {
	t.Parallel()
	t.Run("entry rights", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{rank: 5, privs: 32768}, gates: true})
		f := w.spawnKeeper(t, "CastleDoorman", castleDoormanID, 30)
		gates := strconv.Itoa(castleGateA) + ", " + strconv.Itoa(castleGateB) + ", " + strconv.Itoa(frontierGate)
		assertGates(t, w.command(t, f, "open_doors "+gates), []int32{castleGateA, castleGateB}, true, true)
		assertGates(t, w.command(t, f, "close_doors "+strconv.Itoa(castleGateA)), []int32{castleGateA}, false, true)
		assertGates(t, w.command(t, f, "close_doors "+strconv.Itoa(castleGateB)+", x, "+strconv.Itoa(castleGateA)), []int32{castleGateB}, false, false)
	})
	t.Run("no rights", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{rank: 5}, gates: true})
		f := w.spawnKeeper(t, "CastleDoorman", castleDoormanID, 30)
		assertGates(t, w.command(t, f, "open_doors "+strconv.Itoa(castleGateA)), nil, true, true)
	})
	t.Run("under siege", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{siege: true}, gates: true})
		f := w.spawnKeeper(t, "CastleDoorman", castleDoormanID, 30)
		frames := w.command(t, f, "open_doors "+strconv.Itoa(castleGateA))
		want := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}
		assertAnswer(t, frames, want, f, asRead("doormen/busy.htm"))
		if got := messages(frames); !slices.Equal(got, []int32{serverpackets.SystemMessageGatesNotOpenedClosedDuringSiege}) {
			t.Fatalf("messages = %v", got)
		}
	})
}

// TestFrontierDoormanDoors pins a plain doorman's door commands for anyone:
// the first id of its list is skipped, the rest open; an id naming no door
// stops the handling with no closing ActionFailed.
func TestFrontierDoormanDoors(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{clanless: true}, gates: true})
	f := w.spawnKeeper(t, "Doorman", frontierDoormanID, 30)
	assertGates(t, w.command(t, f, "open_doors "+strconv.Itoa(frontierGate)), nil, true, true)
	assertGates(t, w.command(t, f, "open_doors 1 "+strconv.Itoa(frontierGate)), []int32{frontierGate}, true, true)
	assertGates(t, w.command(t, f, "close_doors 1 999, "+strconv.Itoa(frontierGate)), nil, false, false)
}

// assertGates checks frames move exactly gates, each to open, followed by
// the dispatcher's ActionFailed when released.
func assertGates(t *testing.T, frames [][]byte, gates []int32, open, released bool) {
	t.Helper()
	ids, states, _ := gateStates(frames)
	if !slices.Equal(ids, gates) {
		t.Fatalf("gate updates %v, want %v (frames % x)", ids, gates, opcodes(frames))
	}
	for i, s := range states {
		if s != open {
			t.Fatalf("gate %d open=%v, want %v", ids[i], s, open)
		}
	}
	last := byte(0)
	if len(frames) > 0 {
		last = frames[len(frames)-1][0]
	}
	if (last == serverpackets.OpcodeActionFailed) != released {
		t.Fatalf("frames % x: released=%v, want %v", opcodes(frames), last == serverpackets.OpcodeActionFailed, released)
	}
}

// mercenaryList is the mercenary manager's buylist 1.
func mercenaryList() buylist.List {
	id := castleMercenaryID*10 + 1
	return buylist.List{ID: id, NPCID: castleMercenaryID, Products: []buylist.Product{
		{BuyListID: id, ItemID: shopPotionID, Price: 50, MaxCount: -1, RestockDelayMillis: -60000},
	}}
}

// TestMercenaryManagerCommands pins the mercenary manager's commands for the
// lord: its pages alone, hire refused outside seal validation, and hire
// <n> opening buylist <npcId><n> untaxed before mseller004.htm as read; a
// hire naming no list, or one that does not parse, stops the handling. A
// member who may not hire mercenaries is answered nothing.
func TestMercenaryManagerCommands(t *testing.T) {
	t.Parallel()
	shop := []gameservertest.Option{gameservertest.WithItemTemplates(merchantItems()), gameservertest.WithBuyLists(mercenaryList())}
	t.Run("lord", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{extra: shop})
		f := w.spawnKeeper(t, "MercenaryManagerNpc", castleMercenaryID, 30)
		assertAnswer(t, w.command(t, f, "back"), pageThenFinal, f, filled("mercmanager/mseller001.htm", f))
		assertAnswer(t, w.command(t, f, "how_to"), pageThenFinal, f, filled("mercmanager/mseller005.htm", f))
		assertAnswer(t, w.command(t, f, "merc_limit"), pageThenFinal, f, filled("mercmanager/msellerLimit.htm", f, "%castleName%", "Gludio Castle"))
		assertAnswer(t, w.command(t, f, "hire 1"), pageThenFinal, f, filled("mercmanager/msellerdenial.htm", f))
	})
	t.Run("lord during seal validation", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{period: sevensigns.SealValidation, extra: shop})
		f := w.spawnKeeper(t, "MercenaryManagerNpc", castleMercenaryID, 30)
		assertAnswer(t, w.command(t, f, "hire 1"), []byte{serverpackets.OpcodeBuyList, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, f, asRead("mercmanager/mseller004.htm"))
		assertAnswer(t, w.command(t, f, "hire 2"), []byte{serverpackets.OpcodeActionFailed}, f, "")
		for _, bad := range []string{"hire", "hire x"} {
			if frames := w.command(t, f, bad); len(frames) != 0 {
				t.Fatalf("%q = % x, want nothing", bad, opcodes(frames))
			}
		}
	})
	t.Run("member", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{rank: 5}, period: sevensigns.SealValidation, extra: shop})
		f := w.spawnKeeper(t, "MercenaryManagerNpc", castleMercenaryID, 30)
		assertAnswer(t, w.command(t, f, "hire 1"), []byte{serverpackets.OpcodeActionFailed}, f, "")
	})
}

// TestMassGatekeeperTeleport pins the mass gatekeeper outside the siege: its
// first page alone; tele arms the teleport and opens MassGK-1.htm naming 30
// seconds, its object id unfilled; while it is armed the chat window says
// so, and a second tele arms nothing more; 30 seconds on, the room's
// occupant lands on its exit point with no shout, and the teleport can be
// armed again.
func TestMassGatekeeperTeleport(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{room: true})
	f := w.spawnFolk(t, folkTemplate("CastleGatekeeper", massGatekeeperID), 30)
	w.assertMassGatekeeperChat(t, f, filled("castleteleporter/MassGK.htm", f))

	assertAnswer(t, w.command(t, f, "tele"), pageThenFinal, f, asRead("castleteleporter/MassGK-1.htm", "%delay%", "30"))
	w.assertMassGatekeeperChat(t, f, filled("castleteleporter/MassGK-1.htm", f, "%delay%", "30"))
	assertAnswer(t, w.command(t, f, "tele"), pageThenFinal, f, asRead("castleteleporter/MassGK-1.htm", "%delay%", "30"))

	// Each quiet read lets a little time pass on the driven clock, so the
	// teleport is checked between 20 and 30 seconds after the reads above,
	// with ReadQueued, which lets none pass.
	w.srv.Advance(t, 20*time.Second)
	if frames := w.srv.ReadQueued(t, w.c); len(frames) != 0 || !f.MassTeleportPending() {
		t.Fatalf("before the delay: % x, pending %v", opcodes(frames), f.MassTeleportPending())
	}
	w.srv.Advance(t, 10*time.Second)
	frames := w.srv.ReadQueued(t, w.c)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcSay); ok {
		t.Fatal("shout outside the siege")
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); !ok {
		t.Fatalf("oust = % x, want a teleport", opcodes(frames))
	}
	w.srv.AdvanceUntil(t, "landing on the exit point", func() bool {
		x, y, _ := w.srv.PlayerPosition(t, w.player)
		return x == massRoomExit.X && y == massRoomExit.Y
	})
	if f.MassTeleportPending() {
		t.Fatal("teleport still armed after it fired")
	}
}

// TestMassGatekeeperTeleportUnderSiege pins the siege delay: with the siege
// on and no life control tower standing, the chat window opens MassGK-2.htm
// and tele waits 480 seconds; then the region hears the shout before the
// room is emptied.
func TestMassGatekeeperTeleportUnderSiege(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{siege: true}, room: true})
	f := w.spawnFolk(t, folkTemplate("CastleGatekeeper", massGatekeeperID), 30)
	w.assertMassGatekeeperChat(t, f, filled("castleteleporter/MassGK-2.htm", f))
	assertAnswer(t, w.command(t, f, "tele"), pageThenFinal, f, asRead("castleteleporter/MassGK-1.htm", "%delay%", "480"))

	w.srv.Advance(t, 450*time.Second)
	if frames := w.srv.ReadQueued(t, w.c); len(frames) != 0 {
		t.Fatalf("before the delay: % x", opcodes(frames))
	}
	w.srv.Advance(t, 30*time.Second)
	frames := w.srv.ReadQueued(t, w.c)
	say := slices.IndexFunc(frames, func(fr []byte) bool { return fr[0] == serverpackets.OpcodeNpcSay })
	tele := slices.IndexFunc(frames, func(fr []byte) bool { return fr[0] == serverpackets.OpcodeTeleportToLocation })
	if say < 0 || tele < 0 || say > tele {
		t.Fatalf("oust under siege = % x, want the shout, then the teleport", opcodes(frames))
	}
	want := serverpackets.FrameNpcSay(f.ObjectID(), massGatekeeperID, serverpackets.SayTypeShout,
		"The defenders of Gludio Castle castle have been teleported to the inner castle.")
	defer want.Release()
	if !bytes.Equal(frames[say], want.Bytes()[2:]) {
		t.Fatalf("shout = % x, want % x", frames[say], want.Bytes()[2:])
	}
}

// assertMassGatekeeperChat talks to f and checks its page comes alone after
// the interact.
func (w *folkWorld) assertMassGatekeeperChat(t *testing.T, f *npc.Folk, page string) {
	t.Helper()
	w.selectFolk(t, f)
	frames := w.talk(t, f, false)
	// The talk animation is not replayed within its hold.
	got := slices.DeleteFunc(interactOrder(frames), func(op byte) bool { return op == serverpackets.OpcodeSocialAction })
	if want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeNpcHtmlMessage}; !bytes.Equal(got, want) {
		t.Fatalf("talk = % x, want % x", got, want)
	}
	if got := pageIn(t, frames); got != page {
		t.Fatalf("page = %q, want %q", got, page)
	}
}

// TestWyvernManagerChatWindow pins the wyvern manager's first page by the
// talker's place in the clan owning its castle, each naming the ride's
// default strider level and crystal count, then ActionFailed.
func TestWyvernManagerChatWindow(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup residenceSetup
		file  string
	}{
		{"lord", residenceSetup{}, "wyvernmanager/wyvernmanager-0.htm"},
		{"member", residenceSetup{rank: 5}, "wyvernmanager/wyvernmanager-2.htm"},
		{"rival", residenceSetup{rival: true}, "wyvernmanager/wyvernmanager-0a.htm"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootResidenceCommands(t, commandSetup{residenceSetup: c.setup})
			tmpl := folkTemplate("WyvernManagerNpc", wyvernManagerID)
			tmpl.Name = "Keeper"
			f := w.spawnFolk(t, tmpl, 30)
			w.selectFolk(t, f)
			frames := w.talk(t, f, false)
			want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
			if got := interactOrder(frames); !bytes.Equal(got, want) {
				t.Fatalf("talk = % x, want % x", got, want)
			}
			if got, want := pageIn(t, frames), filled(c.file, f, wyvernPairs...); got != want {
				t.Fatalf("page = %q, want %q", got, want)
			}
		})
	}
}

var wyvernPairs = []string{"%npcname%", "Keeper", "%wyvern_level%", "55", "%needed_crystals%", "10"}

// TestWyvernManagerCommands pins the wyvern manager's dialog: Chat <val>
// opens page val, page 1 for a bare Chat, each then ActionFailed, and a val
// climbing out of the wyvern pages reads as a missing page; a stranger's
// other command gets the chamberlain's refusal page as read; a
// talker who selected another NPC since is answered nothing.
func TestWyvernManagerCommands(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{rival: true}})
	f := w.spawnKeeper(t, "WyvernManagerNpc", wyvernManagerID, 30)
	assertAnswer(t, w.command(t, f, "Chat 3"), chatThenFinal, f, filled("wyvernmanager/wyvernmanager-3.htm", f, wyvernPairs...))
	assertAnswer(t, w.command(t, f, "Chat"), chatThenFinal, f, filled("wyvernmanager/wyvernmanager-1.htm", f, wyvernPairs...))
	assertAnswer(t, w.command(t, f, "manage_vault"), pageThenFinal, f, asRead("chamberlain/noprivs.htm"))
	climb := "data/html/wyvernmanager/wyvernmanager-../../chamberlain/noprivs.htm"
	assertAnswer(t, w.command(t, f, "Chat ../../chamberlain/noprivs"), chatThenFinal, f, "<html><body>My html is missing:<br>"+climb+"</body></html>")

	w.spawnKeeper(t, "CastleMagician", castleMagicianID, 50)
	assertAnswer(t, w.command(t, f, "Chat 3"), []byte{serverpackets.OpcodeActionFailed}, f, "")
}

// TestManorManagerChatCommand pins that every chat page of a manor manager
// is manager.htm, then ActionFailed.
func TestManorManagerChatCommand(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{rival: true}})
	f := w.spawnKeeper(t, "ManorManagerNpc", castleManorID, 30)
	assertAnswer(t, w.command(t, f, "Chat 2"), chatThenFinal, f, filled("manormanager/manager.htm", f))
}
