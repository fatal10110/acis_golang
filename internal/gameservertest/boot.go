// Package gameservertest boots a real gameserver stack against a real
// MariaDB container for behavior tests: one call gives suites a live TCP
// listener wired through the production GameClientLink, a scripted client
// positioned right after the initial (empty) CharSelectInfo, and handles for
// world state, persistence stores, and the batching inventory task.
package gameservertest

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// HexID is the fixed server hex id every booted server registers under.
var HexID = []byte{0x01, 0x02, 0x03, 0x04}

// Option customizes Boot.
type Option func(*options)

type options struct {
	account                string
	characters             []characterSpec
	skills                 *skillstate.Persistence
	trees                  *modelskill.Trees
	spellbooks             modelskill.BookPolicy
	crests                 *datacache.Crests
	cursedWeapons          []*entity.CursedWeaponTable
	karmaPlayerCanTeleport bool
	seed                   func(*gamesql.CharacterStore, *gamesql.ItemStore)
	seedShortcuts          func(*gamesql.ShortcutStore)
	wantChars              int
	log                    zerolog.Logger
}

type characterSpec struct {
	name  string
	level int
	sp    int
}

// WithAccount sets the login account the scripted client authenticates as
// (default "player1").
func WithAccount(account string) Option { return func(o *options) { o.account = account } }

// WithSkills supplies the skill persistence layer wired into the link.
func WithSkills(skills *skillstate.Persistence) Option { return func(o *options) { o.skills = skills } }

// WithSkillTrees supplies the skill trees available at learn time.
func WithSkillTrees(trees *modelskill.Trees) Option { return func(o *options) { o.trees = trees } }

// WithSpellbooks supplies the spellbook policy applied to skill learning.
func WithSpellbooks(policy modelskill.BookPolicy) Option {
	return func(o *options) { o.spellbooks = policy }
}

// WithCrests supplies a pre-populated crest cache.
func WithCrests(crests *datacache.Crests) Option { return func(o *options) { o.crests = crests } }

// WithCursedWeapons supplies cursed weapon tables.
func WithCursedWeapons(tables ...*entity.CursedWeaponTable) Option {
	return func(o *options) { o.cursedWeapons = tables }
}

// WithKarmaTeleport sets the players.properties KarmaPlayerCanTeleport gate
// (default true).
func WithKarmaTeleport(allowed bool) Option {
	return func(o *options) { o.karmaPlayerCanTeleport = allowed }
}

// WithSeed inserts rows through the real SQL stores before the client dials.
func WithSeed(seed func(*gamesql.CharacterStore, *gamesql.ItemStore)) Option {
	return func(o *options) { o.seed = seed }
}

// WithCharacter seeds a selectable character (account-bound, human fighter
// template) through the real SQL character store before the client dials, so
// the initial CharSelectInfo already reports it.
func WithCharacter(name string, level, sp int) Option {
	return func(o *options) { o.characters = append(o.characters, characterSpec{name: name, level: level, sp: sp}) }
}

// WithShortcutSeed inserts shortcut rows before the client dials.
func WithShortcutSeed(seed func(*gamesql.ShortcutStore)) Option {
	return func(o *options) { o.seedShortcuts = seed }
}

// WithWantChars asserts how many characters CharSelectInfo reports after the
// handshake.
func WithWantChars(n int) Option { return func(o *options) { o.wantChars = n } }

// WithLog sets the link logger (default zero-logger).
func WithLog(log zerolog.Logger) Option { return func(o *options) { o.log = log } }

// Server is a booted gameserver stack plus its first connected client.
type Server struct {
	Client           *testsupport.ScriptedClient
	State            *world.State
	DB               *sql.DB
	Chars            *gamesql.CharacterStore
	Items            *gamesql.ItemStore
	Shortcuts        *gamesql.ShortcutStore
	KnownSkills      *gamesql.CharacterSkillStore
	InventoryUpdates *task.InventoryUpdates
	account          string
	templates        *player.TemplateTable
	ids              *sequentialIDs

	closeOnce sync.Once
	cancel    context.CancelFunc
}

// Account is the login account the scripted client authenticated as.
func (s *Server) Account() string { return s.account }

// SeedCharacter inserts a selectable character for Account through the real
// SQL character store.
func (s *Server) SeedCharacter(tb testing.TB, name string, level, sp int) *player.Character {
	return s.seedCharacter(tb, s.account, name, level, sp)
}

// SoleObjectID returns the single character id persisted for Account.
func (s *Server) SoleObjectID(tb testing.TB) int32 {
	tb.Helper()
	characters, err := s.Chars.ListByAccount(context.Background(), s.account)
	if err != nil {
		tb.Fatalf("list characters: %v", err)
	}
	if len(characters) != 1 {
		tb.Fatalf("character count = %d, want 1", len(characters))
	}
	return characters[0].ID
}

// GiveItem persists an inventory item for ownerID through the real SQL item
// store and returns its object id.
func (s *Server) GiveItem(tb testing.TB, ownerID, templateID, count int32) int32 {
	return s.giveItem(tb, ownerID, templateID, count)
}

// NewObjectID allocates the next object id from the server's id sequence.
func (s *Server) NewObjectID() int32 {
	id, err := s.ids.NextID()
	if err != nil {
		panic(err)
	}
	return id
}

// Close tears the stack down (also invoked via testing cleanup).
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
	})
}

// sequentialIDs is the deterministic id source wired into the roster.
type sequentialIDs struct {
	mu   sync.Mutex
	next int32
}

func (s *sequentialIDs) NextID() (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	return s.next, nil
}

// Boot starts the shared MariaDB container, wires the full gameserver stack,
// serves it on an ephemeral port behind a real GS-LS login link, dials a
// scripted client through ProtocolVersion/AuthLogin, and returns the server
// positioned right after the initial (empty or seeded) CharSelectInfo.
func Boot(t *testing.T, opts ...Option) *Server {
	t.Helper()
	o := &options{
		account:                "player1",
		karmaPlayerCanTeleport: true,
	}
	for _, opt := range opts {
		opt(o)
	}

	db := sqltest.SharedDB(t)
	chars := gamesql.NewCharacterStore(db)
	items := gamesql.NewItemStore(db)
	shortcuts := gamesql.NewShortcutStore(db)
	knownSkills := gamesql.NewCharacterSkillStore(db)
	if o.skills == nil {
		o.skills = skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}), knownSkills)
	}
	if o.seed != nil {
		o.seed(chars, items)
	}
	if o.seedShortcuts != nil {
		o.seedShortcuts(shortcuts)
	}
	crests := o.crests
	if crests == nil {
		crests = datacache.NewCrests()
	}
	var cursed *entity.CursedWeaponTable
	if len(o.cursedWeapons) > 0 {
		cursed = o.cursedWeapons[0]
	}

	loginAddr, servers, sessions := startLoginServerAcceptor(t)
	servers.Register(1, HexID)

	validator := network.NewSessionValidator()
	loginLink, err := network.DialLoginLink(context.Background(), loginAddr,
		network.LoginServerAuth{ServerID: 1, HexID: HexID, HostName: "*", Port: 7777, MaxPlayers: 300},
		network.LoginLinkHandlers{PlayerAuthResponse: validator.Resolve}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	t.Cleanup(func() { loginLink.Close() })

	state := world.New()
	clock := task.NewGameClock(time.Now)
	playerClock, err := task.NewPlayerClock(clock, state, network.NewPlayerClockEffects(state))
	if err != nil {
		t.Fatalf("new player clock: %v", err)
	}
	templates := Templates(t)
	itemTemplates := ItemTemplates()
	ids := &sequentialIDs{next: 100}
	inventoryUpdates := task.NewInventoryUpdates()
	roster := gamemanager.NewRoster(chars, items, shortcuts, templates, itemTemplates, npc.NewTable(nil), ids, gamemanager.DefaultDeleteAfter, time.Now)
	gcl := network.NewGameClientLink(network.GameClientLinkConfig{
		Validator:        validator,
		LoginLink:        func() *network.LoginLink { return loginLink },
		Roster:           roster,
		Items:            items,
		Shortcuts:        shortcuts,
		Templates:        templates,
		ItemTemplates:    itemTemplates,
		HTML:             HTMLCache(t, map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"}),
		Crests:           crests,
		Skills:           o.skills,
		Spellbooks:       o.spellbooks,
		SkillTrees:       o.trees,
		CursedWeapons:    cursed,
		World:            state,
		Geo:              Geo{},
		IDs:              ids,
		GroundItems:      task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour, PlayerDroppedMultiplier: 1}, time.Now),
		Positions:        task.NewPositionUpdates(state),
		PlayerClock:      playerClock,
		InventoryUpdates: inventoryUpdates,
		PlayerConfig:     network.PlayerConfig{RespawnRestoreHP: 0.7, SkillEnchantSPBookNeeded: true, KarmaPlayerCanTeleport: o.karmaPlayerCanTeleport, AllowWater: true, PerfectShieldBlockRate: 5},
		PetConfig:        petmodel.DefaultConfig(),
		Log:              o.log,
	})

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
	go network.Serve(ctx, ln, func(ctx context.Context, conn *network.Conn) {
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

	for _, spec := range o.characters {
		tmpl, ok := templates.Get(0)
		if !ok {
			t.Fatal("missing test class template")
		}
		ch, err := player.NewCharacter(100, tmpl, o.account, spec.name, 1, 0, 0, player.SexMale)
		if err != nil {
			t.Fatalf("seed character: %v", err)
		}
		ch.CharLevel = spec.level
		ch.SP = spec.sp
		if err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed character store: %v", err)
		}
	}

	c := testsupport.Dial(t, ln.Addr().String())
	c.SendProtocolVersion(746)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	sessions.Put(o.account, key)
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(o.account)
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	c.Send(w.Bytes())

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != int32(o.wantChars) {
		t.Fatalf("initial char count = %d, want %d", count, o.wantChars)
	}

	return &Server{
		Client:           c,
		State:            state,
		DB:               db,
		Chars:            chars,
		Items:            items,
		Shortcuts:        shortcuts,
		KnownSkills:      knownSkills,
		InventoryUpdates: inventoryUpdates,
		account:          o.account,
		templates:        templates,
		ids:              ids,
		cancel:           cancel,
	}
}

// startLoginServerAcceptor mirrors the login-side GS-LS acceptor the network
// package's own tests use, so Boot completes a real login handshake.
func startLoginServerAcceptor(t *testing.T) (addr string, servers *manager.ServerRegistry, sessions *manager.SessionStore) {
	t.Helper()

	dir := t.TempDir()
	namesPath := filepath.Join(dir, "serverNames.xml")
	if err := os.WriteFile(namesPath, []byte(`<?xml version='1.0'?><list>
		<server id="1" name="Bartz" />
	</list>`), 0o644); err != nil {
		t.Fatalf("write serverNames.xml: %v", err)
	}
	names, err := manager.LoadServerNames(namesPath)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}

	keys, err := manager.NewRSAKeyPool()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}

	servers = manager.NewServerRegistry()
	sessions = manager.NewSessionStore()
	bans := manager.NewIPBanList(zerolog.Nop())

	gsLink := loginserver.NewGameServerLink(servers, names, keys, sessions, bans, nil, nil, false, zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go gsLink.Serve(ctx, ln)

	return ln.Addr().String(), servers, sessions
}
