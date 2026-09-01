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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
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
	restarts               *restart.Table
	zones                  *zone.Index
	attackStance           *task.AttackStance
	attackStanceNow        func() time.Time
	spawnProtection        time.Duration
	allowDelevel           bool
	rateKarmaExpLost       float64
	characterSelectDelay   time.Duration
	serverBypassDelay      time.Duration
	maxBuffsAmount         int
	seed                   func(*gamesql.CharacterStore, *gamesql.ItemStore)
	seedShortcuts          func(*gamesql.ShortcutStore)
	seedHennas             func(db *sql.DB, hennas *gamesql.HennaStore)
	seedSevenSigns         func(*gamesql.SevenSignsStore)
	npcs                   *npc.Table
	summonItems            *item.SummonItemTable
	wantChars              int
	enchantRoll            func() float64
	skillEnchantRoll       func() int
	levels                 *player.LevelTable
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

// WithRestartPoints supplies the restart-point table wired into the link
// (default: none, so restart requests answer ActionFailed).
func WithRestartPoints(table *restart.Table) Option {
	return func(o *options) { o.restarts = table }
}

// WithZones supplies the zone index wired into the link (default: none, so
// no zone flags are raised on enter world or movement).
func WithZones(index *zone.Index) Option {
	return func(o *options) { o.zones = index }
}

// WithAttackStance supplies the combat-stance tracker wired into the link
// (default: nil, so stance is neither tracked nor consulted).
func WithAttackStance(tracker *task.AttackStance) Option {
	return func(o *options) { o.attackStance = tracker }
}

// WithAttackStanceClock builds the production combat-stance timeout
// adapter over Boot's world state, driven by now so tests can expire the
// 15-second inactivity window without waiting.
func WithAttackStanceClock(now func() time.Time) Option {
	return func(o *options) { o.attackStanceNow = now }
}

// WithSpawnProtection sets the players.properties SpawnProtection window
// activated on teleport completion (default: disabled).
func WithSpawnProtection(window time.Duration) Option {
	return func(o *options) { o.spawnProtection = window }
}

// WithReuseDelays overrides the server.properties CharacterSelectTime and
// ServerBypassTime reuse delays (defaults 3s and 100ms). The reference
// treats 0 as "never rate-limited", so flows that legitimately repeat a
// gated action within the shipped window can boot with zero delays.
func WithReuseDelays(characterSelect, serverBypass time.Duration) Option {
	return func(o *options) { o.characterSelectDelay, o.serverBypassDelay = characterSelect, serverBypass }
}

// WithAllowDelevel sets the players.properties AllowDelevel gate: whether a
// death may cost experience/karma (default false).
func WithAllowDelevel(allow bool) Option {
	return func(o *options) { o.allowDelevel = allow }
}

// WithRateKarmaExpLost sets the server.properties RateKarmaExpLost
// multiplier applied to the death exp-loss percentage while karma is
// positive (default 1).
func WithRateKarmaExpLost(rate float64) Option {
	return func(o *options) { o.rateKarmaExpLost = rate }
}

// WithMaxBuffsAmount sets the players.properties MaxBuffsAmount base
// buff-slot count (default 20). Known Divine Inspiration levels add on top.
func WithMaxBuffsAmount(amount int) Option {
	return func(o *options) { o.maxBuffsAmount = amount }
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

// WithHennaSeed inserts character_hennas rows (and optional class updates)
// after selectable characters are created, before the client dials.
func WithHennaSeed(seed func(db *sql.DB, hennas *gamesql.HennaStore)) Option {
	return func(o *options) { o.seedHennas = seed }
}

// WithSevenSignsSeed adjusts the seven_signs_status row before the Seven
// Signs calendar restores it, so boot-time period catch-up can be exercised.
func WithSevenSignsSeed(seed func(*gamesql.SevenSignsStore)) Option {
	return func(o *options) { o.seedSevenSigns = seed }
}

// WithNPCs supplies the NPC template table wired into the link (and the
// roster), so flows that resolve NPC templates — pet collars, decorative
// summons — have data to resolve.
func WithNPCs(table *npc.Table) Option { return func(o *options) { o.npcs = table } }

// WithSummonItems supplies the summon-item table wired into the link, so
// collar-shaped items dispatch through the summon-item use path.
func WithSummonItems(items *item.SummonItemTable) Option {
	return func(o *options) { o.summonItems = items }
}

// WithWantChars asserts how many characters CharSelectInfo reports after the
// handshake.
func WithWantChars(n int) Option { return func(o *options) { o.wantChars = n } }

// WithEnchantRoll supplies the enchant dice roll source wired into the link
// (default: the random source), so enchant outcomes are deterministic.
func WithEnchantRoll(roll func() float64) Option {
	return func(o *options) { o.enchantRoll = roll }
}

// WithSkillEnchantRoll supplies the skill-enchant dice roll source wired into
// the link (default: the random source), so enchant outcomes are
// deterministic.
func WithSkillEnchantRoll(roll func() int) Option {
	return func(o *options) { o.skillEnchantRoll = roll }
}

// WithLevels supplies the player level table wired into the link (default: a
// flat synthetic table covering levels 1-85), so level-gated flows such as
// skill enchant have real thresholds to check.
func WithLevels(levels *player.LevelTable) Option {
	return func(o *options) { o.levels = levels }
}

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
	Hennas           *gamesql.HennaStore
	KnownSkills      *gamesql.CharacterSkillStore
	Pets             *gamesql.PetStore
	InventoryUpdates *task.InventoryUpdates
	ItemInstances    *task.ItemInstances
	GroundItems      *task.GroundItems
	ShadowItems      *task.ShadowItems
	AttackStance     *task.AttackStance
	account          string
	templates        *player.TemplateTable
	itemTable        *item.Table
	levelTable       *player.LevelTable
	ids              *sequentialIDs
	addr             net.Addr
	sessions         *manager.SessionStore
	groundStore      *gamesql.GroundItemStore
	cursedWeapons    *entity.CursedWeaponTable
	autosave         *task.Autosave
	autosaveClock    *autosaveClock

	closeOnce sync.Once
	cancel    context.CancelFunc
}

// autosaveClock is the harness clock task.Autosave reads. EnterWorld's
// Add and TickAutosave share it, so the time value is mutex-guarded.
type autosaveClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *autosaveClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *autosaveClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
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

// Addr is the gameserver TCP listener address additional clients dial.
func (s *Server) Addr() string { return s.addr.String() }

// SeedCharacterFor inserts a selectable character for the given account
// through the real SQL character store.
func (s *Server) SeedCharacterFor(tb testing.TB, account, name string, level, sp int) *player.Character {
	return s.seedCharacter(tb, account, name, level, sp)
}

// DialClient connects a second scripted client as account: handshake,
// AuthLogin, and the initial CharSelectInfo (whose character count must be
// wantChars). The primary client keeps using Server.Client.
func (s *Server) DialClient(t *testing.T, account string, wantChars int) *testsupport.ScriptedClient {
	t.Helper()
	c := testsupport.Dial(t, s.addr.String())
	c.SendProtocolVersion(746)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	s.sessions.Put(account, key)
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(account)
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	c.Send(w.Bytes())

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != int32(wantChars) {
		t.Fatalf("char count for %s = %d, want %d", account, count, wantChars)
	}
	return c
}

// MarkPlayerDead transitions the live player's dead state without routing a
// kill through the combat stack, which its own suites drive end to end; item
// suites use it only to set up gate preconditions no single packet reaches.
func (s *Server) MarkPlayerDead(tb testing.TB, objID int32) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	marker, ok := obj.(interface{ MarkDead() bool })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose MarkDead", objID, obj)
	}
	marker.MarkDead()
}

// SetPlayerOperating toggles the live player's store/workshop operation
// state, the precondition of the use-item storing gate.
func (s *Server) SetPlayerOperating(tb testing.TB, objID int32, operating bool) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	setter, ok := obj.(interface{ SetOperating(bool) bool })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose SetOperating", objID, obj)
	}
	setter.SetOperating(operating)
}

// SeedGroundItem places an item instance directly on the ground at the given
// location, owned by ownerID, without routing a drop request through the
// client protocol. Item suites use it to stage loot — herbs, other players'
// protected drops — that no single packet produces.
func (s *Server) SeedGroundItem(tb testing.TB, ownerID, templateID, count int32, x, y, z int) {
	tb.Helper()
	tmpl, ok := s.itemTable.Get(templateID)
	if !ok {
		tb.Fatalf("no item template %d for ground seed", templateID)
	}
	ground, err := grounditem.New(item.Instance{
		ObjectID:   s.NewObjectID(),
		TemplateID: templateID,
		OwnerID:    ownerID,
		Count:      int(count),
		ManaLeft:   -1,
	}, tmpl)
	if err != nil {
		tb.Fatalf("seed ground item: %v", err)
	}
	s.GroundItems.Drop(ground, task.DropOptions{X: x, Y: y, Z: z})
}

// SetPlayerFlying toggles the live player's transport mode, the precondition
// of the datapack's flying use conditions no single packet reaches.
func (s *Server) SetPlayerFlying(tb testing.TB, objID int32, flying bool) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	setter, ok := obj.(interface{ SetFlying(bool) bool })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose SetFlying", objID, obj)
	}
	setter.SetFlying(flying)
}

// DisablePlayerItem installs a per-item reuse disable on the live player,
// mirroring what a timed-task disable produces; item suites use it only to
// set up gate preconditions no single packet reaches.
func (s *Server) DisablePlayerItem(tb testing.TB, objID, objectID int32, delay time.Duration) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	disabler, ok := obj.(interface{ DisableItem(int32, time.Duration) })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose DisableItem", objID, obj)
	}
	disabler.DisableItem(objectID, delay)
}

// SetInventorySlotLimit shrinks the live player's inventory slot limit so a
// full-inventory rejection is reachable without seeding dozens of rows.
func (s *Server) SetInventorySlotLimit(tb testing.TB, objID int32, limit int) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(interface {
		Inventory() *itemcontainer.Inventory
	})
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose Inventory", objID, obj)
	}
	holder.Inventory().SlotLimit = limit
}

// PlayerPosition reports the live player's current world position.
func (s *Server) PlayerPosition(tb testing.TB, objID int32) (int, int, int) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	located, ok := obj.(interface{ Position() (int, int, int) })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose Position", objID, obj)
	}
	return located.Position()
}

// PlayerTotalWeight reports the live inventory's last-computed total weight.
func (s *Server) PlayerTotalWeight(tb testing.TB, objID int32) int {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(interface {
		Inventory() *itemcontainer.Inventory
	})
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose Inventory", objID, obj)
	}
	return holder.Inventory().TotalWeight()
}

// DrainPlayerMP reduces the live player's current MP by amount, so restore
// flows have observable headroom no single packet creates.
func (s *Server) DrainPlayerMP(tb testing.TB, objID int32, amount int) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	reducer, ok := obj.(interface{ ReduceCurrentMP(int) })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose ReduceCurrentMP", objID, obj)
	}
	reducer.ReduceCurrentMP(amount)
}

// PlayerCurrentMP reports the live player's current MP.
func (s *Server) PlayerCurrentMP(tb testing.TB, objID int32) int {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	reader, ok := obj.(interface{ CurrentMP() int })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose CurrentMP", objID, obj)
	}
	return reader.CurrentMP()
}

// DamagePlayerHP reduces the live player's current HP by amount, so heal
// flows have observable headroom no single packet creates.
func (s *Server) DamagePlayerHP(tb testing.TB, objID int32, amount int) {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	reducer, ok := obj.(interface{ ReduceCurrentHP(int) bool })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose ReduceCurrentHP", objID, obj)
	}
	reducer.ReduceCurrentHP(amount)
}

// PlayerCurrentHP reports the live player's current HP.
func (s *Server) PlayerCurrentHP(tb testing.TB, objID int32) int {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	reader, ok := obj.(interface{ CurrentHP() int })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose CurrentHP", objID, obj)
	}
	return reader.CurrentHP()
}

// PlayerMaxHP reports the live player's stat-computed maximum HP.
func (s *Server) PlayerMaxHP(tb testing.TB, objID int32) int {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	reader, ok := obj.(interface{ MaxHPValue() float64 })
	if !ok {
		tb.Fatalf("world.Player(%d) = %T does not expose MaxHPValue", objID, obj)
	}
	return int(reader.MaxHPValue())
}

// FlushItems persists every pending item mutation the way the production
// lazy-persistence tick does, so suites can assert the items rows mid-test.
func (s *Server) FlushItems(tb testing.TB) {
	tb.Helper()
	if err := s.ItemInstances.Save(context.Background()); err != nil {
		tb.Fatalf("flush items: %v", err)
	}
}

// FlushGroundItems persists every tracked ground item into items_on_ground
// the way the production shutdown hook does, so suites can assert the rows
// mid-test instead of tearing the server down.
func (s *Server) FlushGroundItems(tb testing.TB) {
	tb.Helper()
	if err := s.groundStore.Save(context.Background(), s.GroundItems.Snapshots(s.skipCursedGroundItem)); err != nil {
		tb.Fatalf("save ground items: %v", err)
	}
}

// skipCursedGroundItem excludes cursed weapon item ids from ground-item
// persistence, matching Java's ItemsOnGroundTaskManager.save() skip.
func (s *Server) skipCursedGroundItem(itemID int32) bool {
	if s.cursedWeapons == nil {
		return false
	}
	_, ok := s.cursedWeapons.Weapon(itemID)
	return ok
}

// shutdownDrainTimeout bounds each final flush Shutdown runs, mirroring the
// production stop hooks giving their last-chance writes their own budget.
const shutdownDrainTimeout = 10 * time.Second

// Shutdown drains every pending persistence batch exactly the way the
// production stop hooks do — pending item mutations through one final
// ItemInstances save, tracked ground items back into items_on_ground — and
// then tears the stack down. Restart tests call it on the first Boot cycle
// so the second Boot restores what the first died holding.
func (s *Server) Shutdown(tb testing.TB) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
	defer cancel()
	if err := s.ItemInstances.Save(ctx); err != nil {
		tb.Fatalf("shutdown item flush: %v", err)
	}
	if err := s.groundStore.Save(ctx, s.GroundItems.Snapshots(s.skipCursedGroundItem)); err != nil {
		tb.Fatalf("shutdown ground-item save: %v", err)
	}
	s.Close()
}

// TickAutosave advances the harness clock past the next autosave deadline
// and runs one production sweep. Boot does not start the autosave ticker,
// so tests that need the periodic save path call this instead of waiting
// AutosaveInitialDelay.
func (s *Server) TickAutosave() {
	if s.autosave == nil || s.autosaveClock == nil {
		return
	}
	s.autosaveClock.Advance(task.AutosaveInitialDelay)
	s.autosave.Tick()
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

// nextID allocates the next object id, panicking on allocation failure (the
// deterministic sequence never fails).
func (s *sequentialIDs) nextID() int32 {
	id, err := s.NextID()
	if err != nil {
		panic(err)
	}
	return id
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
		characterSelectDelay:   3 * time.Second,
		serverBypassDelay:      100 * time.Millisecond,
		maxBuffsAmount:         20,
	}
	for _, opt := range opts {
		opt(o)
	}

	db := sqltest.SharedDB(t)
	chars := gamesql.NewCharacterStore(db)
	items := gamesql.NewItemStore(db)
	shortcuts := gamesql.NewShortcutStore(db)
	hennas := gamesql.NewHennaStore(db)
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
	groundStore := gamesql.NewGroundItemStore(db)
	groundItems := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour, PlayerDroppedMultiplier: 1}, time.Now)
	clock := task.NewGameClock(time.Now)
	playerClock, err := task.NewPlayerClock(clock, state, network.NewPlayerClockEffects(state))
	if err != nil {
		t.Fatalf("new player clock: %v", err)
	}
	effects := network.NewTaskEffects(state)
	shadowItems, err := task.NewShadowItems(effects)
	if err != nil {
		t.Fatalf("new shadow items: %v", err)
	}
	templates := Templates(t)
	itemTemplates := ItemTemplates()
	ids := &sequentialIDs{next: 100}
	levels := o.levels
	if levels == nil {
		synthetic := make(map[int]player.Level, 85)
		for lvl := 1; lvl <= 85; lvl++ {
			synthetic[lvl] = player.Level{RequiredExpToLevelUp: 1000}
		}
		levels, err = player.NewLevelTable(synthetic)
		if err != nil {
			t.Fatalf("build level table: %v", err)
		}
	}
	inventoryUpdates := task.NewInventoryUpdates()
	itemInstances := task.NewItemInstances(gamesql.NewItemFlushStore(db), itemTemplates)
	petStore := gamesql.NewPetStore(db)

	// Mirror the production boot for the Seven Signs calendar: optional
	// test seed first, then restore the persisted status and arm the
	// transition timer.
	sevenSignsStore := gamesql.NewSevenSignsStore(db)
	if o.seedSevenSigns != nil {
		o.seedSevenSigns(sevenSignsStore)
	}
	sevenSigns := sevensigns.NewState(sevenSignsStore, o.log, time.Now, nil)
	if err := sevenSigns.Restore(context.Background()); err != nil {
		t.Fatalf("restore seven signs status: %v", err)
	}
	sevenSigns.Start()
	t.Cleanup(sevenSigns.Stop)

	// Restore the ground items the previous session saved at shutdown,
	// mirroring the production boot: hydrate into world state, then clear
	// the rows so a crash cannot double-restore them.
	rows, err := groundStore.Load(context.Background())
	if err != nil {
		t.Fatalf("load ground items: %v", err)
	}
	if err := groundItems.Load(rows, itemTemplates); err != nil {
		t.Fatalf("restore ground items: %v", err)
	}
	if err := groundStore.Clear(context.Background()); err != nil {
		t.Fatalf("clear ground items: %v", err)
	}

	rosterNPCs := o.npcs
	if rosterNPCs == nil {
		rosterNPCs = npc.NewTable(nil)
	}
	roster := gamemanager.NewRoster(chars, items, shortcuts, templates, itemTemplates, rosterNPCs, ids, gamemanager.DefaultDeleteAfter, time.Now)
	effects.SetAutosave(roster, o.skills, zerolog.Nop())
	autosaveClock := &autosaveClock{now: time.Now()}
	autosave, err := task.NewAutosave(effects, autosaveClock.Now)
	if err != nil {
		t.Fatalf("new autosave: %v", err)
	}
	gclConfig := network.GameClientLinkConfig{
		Validator:        validator,
		LoginLink:        func() *network.LoginLink { return loginLink },
		Roster:           roster,
		Items:            items,
		Shortcuts:        shortcuts,
		Hennas:           hennas,
		HennaTable:       HennaTemplates(t),
		Templates:        templates,
		ItemTemplates:    itemTemplates,
		HTML:             HTMLCache(t, map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"}),
		Crests:           crests,
		Skills:           o.skills,
		Spellbooks:       o.spellbooks,
		SkillTrees:       o.trees,
		CursedWeapons:    cursed,
		World:            state,
		NPCs:             o.npcs,
		SummonItems:      o.summonItems,
		PetStore:         petStore,
		Geo:              Geo{},
		IDs:              ids,
		GroundItems:      groundItems,
		Positions:        task.NewPositionUpdates(state),
		PlayerClock:      playerClock,
		GameClock:        task.NewGameClock(time.Now),
		SevenSigns:       sevenSigns,
		InventoryUpdates: inventoryUpdates,
		ItemInstances:    itemInstances,
		ShadowItems:      shadowItems,
		Autosave:         autosave,
		PlayerConfig:     network.PlayerConfig{RespawnRestoreHP: 0.7, SkillEnchantSPBookNeeded: true, KarmaPlayerCanTeleport: o.karmaPlayerCanTeleport, AllowWater: true, PerfectShieldBlockRate: 5, SpawnProtection: o.spawnProtection, AllowDelevel: o.allowDelevel, RateKarmaExpLost: o.rateKarmaExpLost, CharacterSelectDelay: o.characterSelectDelay, ServerBypassDelay: o.serverBypassDelay, MaxBuffsAmount: o.maxBuffsAmount},
		Restarts:         o.restarts,
		Zones:            o.zones,
		PetConfig:        petmodel.DefaultConfig(),
		EnchantRoll:      o.enchantRoll,
		SkillEnchantRoll: o.skillEnchantRoll,
		Levels:           levels,
		Log:              o.log,
	}
	// Assign through the interface only when set: a typed-nil
	// *task.AttackStance would otherwise become a non-nil interface and defeat
	// the link's nil checks.
	attackStance := o.attackStance
	if attackStance == nil && o.attackStanceNow != nil {
		var err error
		attackStance, err = task.NewAttackStance(network.NewAttackStanceEffects(state), o.attackStanceNow)
		if err != nil {
			t.Fatalf("attack stance: %v", err)
		}
	}
	if attackStance != nil {
		gclConfig.AttackStance = attackStance
	}
	gcl := network.NewGameClientLink(gclConfig)
	effects.SetShadowItemExpiry(gcl.ExpireShadowItem)

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
		ch, err := player.NewCharacter(ids.nextID(), tmpl, o.account, spec.name, 1, 0, 0, player.SexMale)
		if err != nil {
			t.Fatalf("seed character: %v", err)
		}
		ch.CharLevel = spec.level
		ch.SP = spec.sp
		if err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed character store: %v", err)
		}
	}
	if o.seedHennas != nil {
		o.seedHennas(db, hennas)
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
		itemTable:        itemTemplates,
		levelTable:       levels,
		DB:               db,
		Chars:            chars,
		Items:            items,
		Shortcuts:        shortcuts,
		Hennas:           hennas,
		KnownSkills:      knownSkills,
		Pets:             petStore,
		InventoryUpdates: inventoryUpdates,
		ItemInstances:    itemInstances,
		GroundItems:      groundItems,
		ShadowItems:      shadowItems,
		AttackStance:     attackStance,
		account:          o.account,
		templates:        templates,
		ids:              ids,
		addr:             ln.Addr(),
		sessions:         sessions,
		groundStore:      gamesql.NewGroundItemStore(db),
		cursedWeapons:    cursed,
		autosave:         autosave,
		autosaveClock:    autosaveClock,
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

	gsLink := loginserver.NewGameServerLink(servers, names, keys, sessions, bans, nil, nil, false, nil, loginserver.NewLinkRoster(), zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go gsLink.Serve(ctx, ln)

	return ln.Addr().String(), servers, sessions
}
