package network

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// --- test server setup ---

// testInventoryUpdates maps each test's *world.State to the
// *task.InventoryUpdates wired into its GameClientLink, so a test that
// otherwise only gets back state (not gcl) can still drive the batching
// task's tick deterministically instead of waiting on its real cadence.
var (
	testInventoryUpdatesMu sync.Mutex
	testInventoryUpdates   = map[*world.State]*task.InventoryUpdates{}
)

func registerTestInventoryUpdates(t *testing.T, state *world.State, updates *task.InventoryUpdates) {
	t.Helper()
	testInventoryUpdatesMu.Lock()
	testInventoryUpdates[state] = updates
	testInventoryUpdatesMu.Unlock()
	t.Cleanup(func() {
		testInventoryUpdatesMu.Lock()
		delete(testInventoryUpdates, state)
		testInventoryUpdatesMu.Unlock()
	})
}

// inventoryUpdatesFor returns the batching task registered for state by
// registerTestInventoryUpdates.
func inventoryUpdatesFor(t *testing.T, state *world.State) *task.InventoryUpdates {
	t.Helper()
	u, ok := lookupTestInventoryUpdates(state)
	if !ok {
		t.Fatal("no inventory update task registered for this test link")
	}
	return u
}

// lookupTestInventoryUpdates is inventoryUpdatesFor without the test
// failure, for callers like attachTestPet that run before some test setups
// have registered a task yet and need to treat that as "nothing to wire
// here" rather than a failure.
func lookupTestInventoryUpdates(state *world.State) (*task.InventoryUpdates, bool) {
	testInventoryUpdatesMu.Lock()
	defer testInventoryUpdatesMu.Unlock()
	u, ok := testInventoryUpdates[state]
	return u, ok
}

// assertSystemMessageStringFrame checks a single-param text SystemMessage.
func assertSystemMessageStringFrame(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage opcode = %#x, want %#x", frame[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("SystemMessage param type = %d, want text", typ)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("SystemMessage text = %q, want %q", got, text)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func newTestGameClientLink(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithLog(t, loginLink, validator, zerolog.Nop())
}

func newTestGameClientLinkWithLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, log zerolog.Logger) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithSkillsAndLog(t, loginLink, validator, nil, log)
}

func newTestGameClientLinkWithSkillsAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, log zerolog.Logger) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	addr, chars, items, _, state = newTestGameClientLinkWithSkillsShortcutsAndLog(t, loginLink, validator, skills, log)
	return addr, chars, items, state
}

func newTestGameClientLinkWithSkillsShortcutsAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, log zerolog.Logger) (addr string, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithSkillsShortcutsCrestsAndLog(t, loginLink, validator, skills, nil, modelskill.BookPolicy{}, nil, log)
}

func newTestGameClientLinkWithSkillsShortcutsCrestsAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, crests *datacache.Crests, spellbooks modelskill.BookPolicy, trees *modelskill.Trees, log zerolog.Logger, cursedWeapons ...*entity.CursedWeaponTable) (addr string, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithSkillsShortcutsCrestsKarmaAndLog(t, loginLink, validator, skills, crests, spellbooks, trees, true, log, cursedWeapons...)
}

// newTestGameClientLinkWithSkillsShortcutsCrestsKarmaAndLog is the full
// constructor; karmaPlayerCanTeleport plugs the players.properties
// KarmaPlayerCanTeleport gate so karma-teleport-rejection tests can run it
// false without disturbing every other caller's default-true setup.
func newTestGameClientLinkWithSkillsShortcutsCrestsKarmaAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, crests *datacache.Crests, spellbooks modelskill.BookPolicy, trees *modelskill.Trees, karmaPlayerCanTeleport bool, log zerolog.Logger, cursedWeapons ...*entity.CursedWeaponTable) (addr string, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	chars = newFakeCharStore()
	items = newFakeItemStore()
	shortcuts = newFakeShortcutStore()
	state = world.New()
	templates := testTemplates(t)
	itemTemplates := testItemTemplates()
	ids := &sequentialIDs{next: 100}
	groundItems := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour, PlayerDroppedMultiplier: 1}, time.Now)
	roster := gamemanager.NewRoster(chars, items, shortcuts, templates, itemTemplates, npc.NewTable(nil), ids, gamemanager.DefaultDeleteAfter, time.Now)
	html := testHTMLCache(t, map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"})
	if crests == nil {
		crests = datacache.NewCrests()
	}
	var cursed *entity.CursedWeaponTable
	if len(cursedWeapons) > 0 {
		cursed = cursedWeapons[0]
	}
	playerConfig := PlayerConfig{RespawnRestoreHP: 0.7, SkillEnchantSPBookNeeded: true, KarmaPlayerCanTeleport: karmaPlayerCanTeleport, AllowWater: true}
	inventoryUpdates := task.NewInventoryUpdates()
	gcl := NewGameClientLink(GameClientLinkConfig{
		Validator:        validator,
		LoginLink:        loginLink,
		Roster:           roster,
		Items:            items,
		Shortcuts:        shortcuts,
		Templates:        templates,
		ItemTemplates:    itemTemplates,
		HTML:             html,
		Crests:           crests,
		Skills:           skills,
		Spellbooks:       spellbooks,
		SkillTrees:       trees,
		CursedWeapons:    cursed,
		World:            state,
		Geo:              testGeo{},
		IDs:              ids,
		GroundItems:      groundItems,
		Positions:        task.NewPositionUpdates(state),
		InventoryUpdates: inventoryUpdates,
		PlayerConfig:     playerConfig,
		PetConfig:        petmodel.DefaultConfig(),
		Log:              log,
	})
	registerTestInventoryUpdates(t, state, inventoryUpdates)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var handlers struct {
		sync.Mutex
		count int
	}
	handlersDone := sync.NewCond(&handlers.Mutex)
	t.Cleanup(func() {
		cancel()
		ln.Close()
		handlers.Lock()
		defer handlers.Unlock()
		for handlers.count > 0 {
			handlersDone.Wait()
		}
	})
	go Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
		handlers.Lock()
		handlers.count++
		handlers.Unlock()
		defer func() {
			handlers.Lock()
			handlers.count--
			handlersDone.Broadcast()
			handlers.Unlock()
		}()
		gcl.Handle(ctx, conn)
	}, zerolog.Nop())

	return ln.Addr().String(), chars, items, shortcuts, state
}

// newLinkedGameClient wires a GameClientLink to a real login-server-side
// GS-LS link (the same infrastructure loginlink_test.go uses), dials a fake
// game client through VersionCheck and a successful AuthLogin, and returns
// it positioned right after the initial (empty) CharSelectInfo.
func newLinkedGameClient(t *testing.T) (c *testsupport.ScriptedClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkills(t, nil)
}

func newLinkedGameClientWithSkills(t *testing.T, skills *skillstate.Persistence) (c *testsupport.ScriptedClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkillsSeed(t, skills, nil, 0)
}

func newLinkedGameClientWithSkillsSeed(t *testing.T, skills *skillstate.Persistence, seed func(*fakeCharStore, *fakeItemStore), wantChars int) (c *testsupport.ScriptedClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	c, chars, items, _, state = newLinkedGameClientWithSkillsShortcutsSeed(t, skills, nil, seed, wantChars)
	return c, chars, items, state
}

func newLinkedGameClientWithShortcuts(t *testing.T) (c *testsupport.ScriptedClient, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkillsShortcutsSeed(t, nil, nil, nil, 0)
}

func newLinkedGameClientWithSkillsShortcutsSeed(t *testing.T, skills *skillstate.Persistence, shortcutSeed func(*fakeShortcutStore), seed func(*fakeCharStore, *fakeItemStore), wantChars int) (c *testsupport.ScriptedClient, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkillsShortcutsCrestsSeed(t, skills, shortcutSeed, nil, modelskill.BookPolicy{}, nil, seed, wantChars)
}

func newLinkedGameClientWithSkillsShortcutsCrestsSeed(t *testing.T, skills *skillstate.Persistence, shortcutSeed func(*fakeShortcutStore), crests *datacache.Crests, spellbooks modelskill.BookPolicy, trees *modelskill.Trees, seed func(*fakeCharStore, *fakeItemStore), wantChars int, cursedWeapons ...*entity.CursedWeaponTable) (c *testsupport.ScriptedClient, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()

	loginAddr, servers, sessions := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	validator := NewSessionValidator()
	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	loginLink, err := DialLoginLink(context.Background(), loginAddr, auth, LoginLinkHandlers{PlayerAuthResponse: validator.Resolve}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	t.Cleanup(func() { loginLink.Close() })

	addr, chars, items, shortcuts, state := newTestGameClientLinkWithSkillsShortcutsCrestsAndLog(t, func() *LoginLink { return loginLink }, validator, skills, crests, spellbooks, trees, zerolog.Nop(), cursedWeapons...)
	if seed != nil {
		seed(chars, items)
	}
	if shortcutSeed != nil {
		shortcutSeed(shortcuts)
	}

	c = testsupport.Dial(t, addr)
	c.SendProtocolVersion(746)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	sessions.Put("player1", key)
	c.Send(encodeAuthLogin("player1", key))

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != int32(wantChars) {
		t.Fatalf("initial char count = %d, want %d", count, wantChars)
	}
	return c, chars, items, shortcuts, state
}

func seedSelectableCharacter(t *testing.T, chars *fakeCharStore, account, name string, level, sp int) int32 {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, account, name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	return ch.ID
}
func newTestLivePlayer(t testing.TB, id int32, capture *testsupport.FrameCapture) *livePlayer {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch := &player.Character{
		ID: id, Name: "Player", ClassID: 0, BaseClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		CharLevel: 1,
		Location:  location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.SetResourceValues(player.Resources{MaxHP: 80, CurrentHP: 80, MaxMP: 30, CurrentMP: 30})
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, testItemTemplates(), nil))
	ch.SetFrameSender(capture.Send)
	ch.SetBroadcastFrameSender(capture.Send)

	x, y, z := ch.Position()
	live, err := creature.NewLive(location.Location{X: x, Y: y, Z: z}, tmpl.RunSpeed, testGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	moveCtl, err := move.NewController(ch.Move(), ch)
	if err != nil {
		t.Fatal(err)
	}
	attackCtl := attack.NewPlayer(ch)
	combat := ai.NewPlayerAttack(ch, moveCtl, attackCtl)
	moveCtl.SetArrived(combat.Think)
	attackCtl.SetFinished(combat.Think)

	return &livePlayer{Character: ch, template: tmpl, attack: attackCtl, move: moveCtl, combat: combat, visibilitySend: capture.Send}
}

func newTestHostileNPC(t *testing.T, id int32) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID:              100,
		TemplateID:      100,
		Type:            "Monster",
		Level:           1,
		HPMax:           100,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
	inst, err := npc.NewInstance(id, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	live, err := creature.NewLive(location.Location{}, 100, testHostileGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hostile, err := npc.NewHostile(inst, live, testHostileMove{}, testHostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	return hostile
}

type testHostileGeo struct{}

func (testHostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (testHostileGeo) Height(_, _, _ int) int16          { return 0 }

func (testHostileGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (testHostileGeo) Walkable(int, int, int) bool                                 { return true }
func (testHostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type testHostileMove struct{}

func (testHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (testHostileMove) MoveHome(location.Location) error { return nil }
func (testHostileMove) Stop() error                      { return nil }

type testHostileAttack struct{}

func (testHostileAttack) BowCoolingDown() bool                { return false }
func (testHostileAttack) AttackingNow() bool                  { return false }
func (testHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (testHostileAttack) DoAttack(attackable.Combatant) error { return nil }
