// Package gameservertest boots a real gameserver stack against a real
// MariaDB container for behavior tests: one call gives suites a live TCP
// listener wired through the production GameClientLink, a scripted client
// positioned right after the initial (empty) CharSelectInfo, and handles for
// world state, persistence stores, and the batching inventory task.
package gameservertest

import (
	"cmp"
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// HexID is the fixed server hex id every booted server registers under.
var HexID = []byte{0x01, 0x02, 0x03, 0x04}

// Option customizes Boot.
type Option func(*options)

type options struct {
	// slowStores delays every handler-issued persistence write (WithSlowStores).
	slowStores             time.Duration
	itemFlushFault         *ItemFlushFault
	subclassFault          SubclassFault
	petNameLookupErr       error
	captureLog             bool
	account                string
	characters             []characterSpec
	skills                 *skillstate.Persistence
	trees                  *modelskill.Trees
	healSps                *modelskill.HealSpsTable
	spellbooks             modelskill.BookPolicy
	crests                 *datacache.Crests
	cursedWeapons          []*entity.CursedWeaponTable
	karmaPlayerCanTeleport bool
	karmaServiceGates      [3]bool
	htmlPages              map[string]string
	karmaPlayerCanTrade    bool
	admin                  *admin.Data
	gmStartupUnlisted      bool
	gmStartupModes         [3]bool
	gmAudit                zerolog.Logger
	chat                   network.ChatConfig
	restarts               *restart.Table
	teleports              travel.TeleportTable
	instantTeleports       travel.InstantTable
	freeTeleport           bool
	teleportClock          func() time.Time
	tradeClock             func() time.Time
	zones                  *zone.Index
	water                  bool
	waterNow               func() time.Time
	disallowWater          bool
	disableFallingDamage   bool
	attackStance           *task.AttackStance
	pvpFlags               *task.PvPFlags
	decay                  *task.Decay
	npcSpawns              *spawn.Table
	attackStanceTracker    network.AttackStanceTracker
	attackStanceNow        func() time.Time
	spawnProtection        time.Duration
	allowDelevel           bool
	autoLearnSkills        bool
	deepBlueDropRules      bool
	autoLoot               bool
	rateKarmaExpLost       float64
	characterSelectDelay   time.Duration
	persistWait            time.Duration
	serverBypassDelay      time.Duration
	maxBuffsAmount         int
	weightLimitMultiplier  float64
	inventorySlots         player.InventorySlots
	storageSlots           player.StorageSlots
	freight                *network.FreightConfig
	storeSkillCooltime     bool
	cancelLesserEffect     bool
	magicFailures          bool
	night                  conditions.NightSource
	maxGeoPathFailCount    int
	seed                   func(*gamesql.CharacterStore, *gamesql.ItemStore)
	seedShortcuts          func(*gamesql.ShortcutStore)
	seedHennas             func(db *sql.DB, hennas *gamesql.HennaStore)
	seedSevenSigns         func(*gamesql.SevenSignsStore)
	clanConfig             *clan.Config
	seedClans              func(db *sql.DB)
	board                  bbs.Config
	seedBoard              func(db *sql.DB)
	seedOlympiad           func(db *sql.DB)
	seedBoss               func(db *sql.DB)
	serverNews             bool
	announcements          string
	clanClock              func() time.Time
	npcs                   *npc.Table
	summonItems            *item.SummonItemTable
	wantChars              int
	enchantRoll            func() float64
	recipes                *recipe.Table
	hennas                 *henna.Table
	craftingDisabled       bool
	discardItemDisabled    bool
	manufactureDelay       time.Duration
	craftRoll              func(n int) int
	fish                   *fish.Table
	fishingRoll            func(n int) int
	multisells             *multisell.Table
	multisellDelay         time.Duration
	rollDiceDelay          time.Duration
	keepMaintained         bool
	augmentations          *augmentation.Table
	armorSets              *armorset.Table
	manor                  network.ManorConfig
	augmentationChances    *augmentation.Chances
	augmentRoll            augmentation.Rand
	enchantConfig          *enchant.Config
	skillEnchantRoll       func() int
	levels                 *player.LevelTable
	classTemplate          *player.Template
	extraClassTemplates    []*player.Template
	subclassWithoutQuests  bool
	subclassDelay          time.Duration
	log                    zerolog.Logger
	geo                    move.Geo
	itemTemplates          *item.Table
	productionTickers      bool
	handAI                 bool
	realPool               bool
	merchant               merchantOptions
	doors                  []*door.Template
	boats                  []route.BoatItinerary
	petitionConfig         *petition.Config
	schemeBuffer           *schemebuffer.Manager
	weddingConfig          *wedding.Config
	derby                  *derbyOptions
	// rewardPartiesWrap wraps the link's kill-party resolver
	// (WithRewardParties).
	rewardPartiesWrap func(gamemanager.RewardParties) gamemanager.RewardParties
}

type characterSpec struct {
	name  string
	level int
	sp    int
	sex   player.Sex
}

// WithAccount sets the login account the scripted client authenticates as
// (default "player1").
func WithAccount(account string) Option { return func(o *options) { o.account = account } }

// WithSkills supplies the skill persistence layer wired into the link.
func WithSkills(skills *skillstate.Persistence) Option { return func(o *options) { o.skills = skills } }

// WithStoreSkillCooltime controls whether effect and reuse state survives relog.
func WithStoreSkillCooltime(enabled bool) Option {
	return func(o *options) { o.storeSkillCooltime = enabled }
}

// WithSkillTrees supplies the skill trees available at learn time.
func WithSkillTrees(trees *modelskill.Trees) Option { return func(o *options) { o.trees = trees } }

// WithHealSps supplies the spiritshot heal corrections a charged heal reads
// (none by default).
func WithHealSps(table *modelskill.HealSpsTable) Option {
	return func(o *options) { o.healSps = table }
}

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

// WithKarmaServiceGates sets the players.properties KarmaPlayerCanShop,
// KarmaPlayerCanUseGK and KarmaPlayerCanUseWareHouse gates (default false,
// false, true).
func WithKarmaServiceGates(shop, gatekeeper, warehouse bool) Option {
	return func(o *options) { o.karmaServiceGates = [3]bool{shop, gatekeeper, warehouse} }
}

// WithHTMLPages adds datapack HTML pages, keyed by their path under
// data/html, to the link's page cache.
func WithHTMLPages(pages map[string]string) Option {
	return func(o *options) {
		if o.htmlPages == nil {
			o.htmlPages = map[string]string{}
		}
		for name, content := range pages {
			o.htmlPages[name] = content
		}
	}
}

// WithKarmaTrade sets the players.properties KarmaPlayerCanTrade gate
// (default true).
func WithKarmaTrade(allowed bool) Option {
	return func(o *options) { o.karmaPlayerCanTrade = allowed }
}

// WithAdmin supplies the access-level table characters resolve their
// persisted access level against at login (default: none, so every
// character plays under the attribute defaults).
func WithAdmin(data *admin.Data) Option { return func(o *options) { o.admin = data } }

// WithGMStartupUnlisted sets players.properties GMStartupAutoList = False:
// a game master logs in hidden from /gmlist.
func WithGMStartupUnlisted() Option { return func(o *options) { o.gmStartupUnlisted = true } }

// WithGMStartupModes sets players.properties GMStartupInvulnerable,
// GMStartupInvisible and GMStartupBlockAll: the modes a game master logs in
// with (default: none, as shipped).
func WithGMStartupModes(invulnerable, invisible, blockAll bool) Option {
	return func(o *options) { o.gmStartupModes = [3]bool{invulnerable, invisible, blockAll} }
}

// WithGMAudit records every admin command run to log (server.properties
// GMAudit = True); by default nothing is recorded.
func WithGMAudit(log zerolog.Logger) Option { return func(o *options) { o.gmAudit = log } }

// WithChat sets the server.properties chat settings: the chat log, the bot
// whisper filter and the chat reuse delays (default: nothing logged,
// nothing filtered, no delay).
func WithChat(cfg network.ChatConfig) Option { return func(o *options) { o.chat = cfg } }

// WithRestartPoints supplies the restart-point table wired into the link
// (default: none, so restart requests answer ActionFailed).
func WithRestartPoints(table *restart.Table) Option {
	return func(o *options) { o.restarts = table }
}

// WithTeleports supplies the destinations civilian NPCs offer (default:
// none), charged for unless free, with the weekend half-price hours read
// from now (nil means time.Now).
func WithTeleports(teleports travel.TeleportTable, instants travel.InstantTable, free bool, now func() time.Time) Option {
	return func(o *options) {
		o.teleports, o.instantTeleports, o.freeTeleport, o.teleportClock = teleports, instants, free, now
	}
}

// WithTradeClock times every pending request out against now (nil means
// time.Now): direct trade, party, command channel, party room and friend
// invitations alike, so a scenario can let a request expire without
// waiting.
func WithTradeClock(now func() time.Time) Option {
	return func(o *options) { o.tradeClock = now }
}

// WithZones supplies the zone index wired into the link (default: none, so
// no zone flags are raised on enter world or movement).
func WithZones(index *zone.Index) Option {
	return func(o *options) { o.zones = index }
}

// WithWater wires the drowning tracker into the link, reading breath
// deadlines from now (nil means time.Now), so water-zone entry and exit
// start and stop the breath countdown. Its one-second tick is not started;
// a test drives drowning through Server.Water.Tick.
func WithWater(now func() time.Time) Option {
	return func(o *options) {
		o.water = true
		o.waterNow = now
	}
}

// WithAllowWater sets the AllowWater server option (default true); false
// keeps water zones swimming but never starts a breath countdown.
func WithAllowWater(allowed bool) Option {
	return func(o *options) { o.disallowWater = !allowed }
}

// WithFallingDamage sets the EnableFallingDamage server option (default true).
func WithFallingDamage(enabled bool) Option {
	return func(o *options) { o.disableFallingDamage = !enabled }
}

// WithAttackStance supplies the combat-stance tracker wired into the link
// (default: nil, so stance is neither tracked nor consulted).
func WithAttackStance(tracker *task.AttackStance) Option {
	return func(o *options) { o.attackStance = tracker }
}

// WithPvPFlags supplies the PvP-flag tracker wired into the link (default:
// nil, so a resolved attack flags nobody).
func WithPvPFlags(flags *task.PvPFlags) Option {
	return func(o *options) { o.pvpFlags = flags }
}

// WithDecay supplies the corpse-decay task wired into the link and the
// civilian NPC spawner (default: nil, so a dead summon's or civilian NPC's
// corpse never decays).
func WithDecay(decay *task.Decay) Option {
	return func(o *options) { o.decay = decay }
}

// WithAttackStanceTracker substitutes the combat-stance tracker the link
// consults, including from the exit guard that runs inside the task a
// restart or logout request posts to the player's queue, so a suite can
// drive a failure from inside a queued handler. It replaces only the link's
// collaborator; Server.AttackStance stays the production one. It cannot be
// combined with WithAttackStance, which sets the same collaborator.
func WithAttackStanceTracker(tracker network.AttackStanceTracker) Option {
	return func(o *options) { o.attackStanceTracker = tracker }
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

// WithPersistWait bounds how long a connection waits for queued saves before
// reading rows back, so a suite can drive the wait's timeout quickly.
func WithPersistWait(d time.Duration) Option {
	return func(o *options) { o.persistWait = d }
}

// WithReuseDelays overrides the server.properties CharacterSelectTime and
// ServerBypassTime reuse delays (defaults 3s and 100ms). The reference
// treats 0 as "never rate-limited", so flows that legitimately repeat a
// gated action within the shipped window can boot with zero delays.
func WithReuseDelays(characterSelect, serverBypass time.Duration) Option {
	return func(o *options) { o.characterSelectDelay, o.serverBypassDelay = characterSelect, serverBypass }
}

// WithAutoLearnSkills sets the players.properties AutoLearnSkills gate: a
// level change grants every available class skill (default false).
func WithAutoLearnSkills() Option {
	return func(o *options) { o.autoLearnSkills = true }
}

// WithAllowDelevel sets the players.properties AllowDelevel gate: whether a
// death may cost experience/karma (default false).
func WithAllowDelevel(allow bool) Option {
	return func(o *options) { o.allowDelevel = allow }
}

// WithDeepBlueDropRules sets the DeepBlueDropRules gate: whether an
// out-leveled kill cuts the drop chance (default false).
func WithDeepBlueDropRules(enabled bool) Option {
	return func(o *options) { o.deepBlueDropRules = enabled }
}

// WithAutoLoot sets the server.properties AutoLoot gate: whether a
// non-raid kill's drops go straight into the killer's inventory instead of
// onto the ground (default false).
func WithAutoLoot(enabled bool) Option {
	return func(o *options) { o.autoLoot = enabled }
}

// WithRateKarmaExpLost sets the server.properties RateKarmaExpLost
// multiplier applied to the death exp-loss percentage while karma is
// positive (default 1).
func WithRateKarmaExpLost(rate float64) Option {
	return func(o *options) { o.rateKarmaExpLost = rate }
}

// WithWeightLimitMultiplier sets the players.properties WeightLimit
// multiplier (default 1, the shipped value). 0 leaves every player with a
// weight limit of 0: no weight penalty band is ever computed, and no
// weighted item can be received through a weight-checked path.
func WithWeightLimitMultiplier(m float64) Option {
	return func(o *options) { o.weightLimitMultiplier = m }
}

// WithInventorySlots sets the players.properties MaximumSlotsForNoDwarf /
// MaximumSlotsForDwarf base inventory slot counts (default 80/100).
func WithInventorySlots(noDwarf, dwarf int) Option {
	return func(o *options) {
		o.inventorySlots = player.InventorySlots{NoDwarf: noDwarf, Dwarf: dwarf, Configured: true}
	}
}

// WithStorageSlots sets the players.properties warehouse, freight, private
// store and recipe book base sizes (default player.DefaultStorageSlots).
func WithStorageSlots(slots player.StorageSlots) Option {
	return func(o *options) {
		slots.Configured = true
		o.storageSlots = slots
	}
}

// WithFreight sets the freight service settings (default
// network.DefaultFreightConfig).
func WithFreight(cfg network.FreightConfig) Option {
	return func(o *options) { o.freight = &cfg }
}

// WithMaxBuffsAmount sets the players.properties MaxBuffsAmount base
// buff-slot count (default 20). Known Divine Inspiration levels add on top.
func WithMaxBuffsAmount(amount int) Option {
	return func(o *options) { o.maxBuffsAmount = amount }
}

// WithCancelLesserEffect sets the players.properties CancelLesserEffect
// switch: whether a newly stacked non-herb effect removes the lower-priority
// effect it displaces (default true).
func WithCancelLesserEffect(enabled bool) Option {
	return func(o *options) { o.cancelLesserEffect = enabled }
}

// WithMagicFailures sets the players.properties MagicFailures switch:
// whether magic-damage casts roll for a half or full resist (default true).
func WithMagicFailures(enabled bool) Option {
	return func(o *options) { o.magicFailures = enabled }
}

// WithNightSource sets the in-game clock <game night=.../> stat conditions
// and melee hit chance read on this server (default nil: always day).
func WithNightSource(src conditions.NightSource) Option {
	return func(o *options) { o.night = src }
}

// WithMaxGeoPathFailCount sets the geoengine.properties MaxGeopathFailCount
// overflow threshold of every hostile the fixtures spawn (default 0: the
// shipped 50).
func WithMaxGeoPathFailCount(n int) Option {
	return func(o *options) { o.maxGeoPathFailCount = n }
}

// WithSeed inserts rows through the real SQL stores before the client dials.
func WithSeed(seed func(*gamesql.CharacterStore, *gamesql.ItemStore)) Option {
	return func(o *options) { o.seed = seed }
}

// WithCharacter seeds a selectable character (account-bound, human fighter
// template) through the real SQL character store before the client dials, so
// the initial CharSelectInfo already reports it.
func WithCharacter(name string, level, sp int) Option {
	return WithCharacterSex(name, level, sp, player.SexMale)
}

// WithCharacterSex seeds a selectable character with the given sex.
func WithCharacterSex(name string, level, sp int, sex player.Sex) Option {
	return func(o *options) {
		o.characters = append(o.characters, characterSpec{name: name, level: level, sp: sp, sex: sex})
	}
}

// WithShortcutSeed inserts shortcut rows before the client dials.
func WithShortcutSeed(seed func(*gamesql.ShortcutStore)) Option {
	return func(o *options) { o.seedShortcuts = seed }
}

// WithHennaTable boots with hennas as the loaded dye symbol table instead
// of HennaTemplates.
func WithHennaTable(hennas *henna.Table) Option {
	return func(o *options) { o.hennas = hennas }
}

// WithHennaSeed inserts character_hennas rows (and optional class updates)
// after selectable characters are created, before the client dials.
func WithHennaSeed(seed func(db *sql.DB, hennas *gamesql.HennaStore)) Option {
	return func(o *options) { o.seedHennas = seed }
}

// WithClanConfig sets the clans.properties join and creation penalties.
func WithClanConfig(cfg clan.Config) Option {
	return func(o *options) { o.clanConfig = &cfg }
}

// WithClanSeed writes clan rows, and the clanhall rows naming their owned
// halls, once the seeded characters are stored and before the clans are
// restored from them.
func WithClanSeed(seed func(db *sql.DB)) Option {
	return func(o *options) { o.seedClans = seed }
}

// WithCommunityBoard sets the community board's server.properties
// settings (default off, opening on _bbshome). With the board on, the mail
// stored in bbs_mail is restored once the characters and clans are seeded.
func WithCommunityBoard(cfg bbs.Config) Option {
	return func(o *options) { o.board = cfg }
}

// WithBoardSeed runs seed against the database after the characters and
// clans are seeded and before the board's mail is restored.
func WithBoardSeed(seed func(db *sql.DB)) Option {
	return func(o *options) { o.seedBoard = seed }
}

// WithServerNews sets server.properties ShowServerNews (default false).
func WithServerNews(shown bool) Option {
	return func(o *options) { o.serverNews = shown }
}

// WithAnnouncements boots with file as the announcements.xml content
// (default: a file holding none). The server rewrites the copy at
// Server.AnnounceFile, never the datapack's.
func WithAnnouncements(file string) Option {
	return func(o *options) { o.announcements = file }
}

// WithClanClock times clan invitations out on now instead of the wall
// clock.
func WithClanClock(now func() time.Time) Option {
	return func(o *options) { o.clanClock = now }
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

// WithEnchantConfig sets the players.properties scroll-of-enchant rates and
// limits; without it the shipped defaults apply.
func WithEnchantConfig(cfg enchant.Config) Option {
	return func(o *options) { o.enchantConfig = &cfg }
}

// WithRecipes replaces the recipe table the link loads (default:
// RecipeTemplates).
func WithRecipes(table *recipe.Table) Option {
	return func(o *options) { o.recipes = table }
}

// WithCraftingDisabled boots with players.properties CraftingEnabled off.
func WithCraftingDisabled() Option {
	return func(o *options) { o.craftingDisabled = true }
}

// WithDiscardItemDisabled sets server.properties AllowDiscardItem = False:
// only a GM may drop items (default: anyone may).
func WithDiscardItemDisabled() Option {
	return func(o *options) { o.discardItemDisabled = true }
}

// WithManufactureDelay sets the reuse delay between two crafts on one
// client (default 0: every craft request is taken).
func WithManufactureDelay(d time.Duration) Option {
	return func(o *options) { o.manufactureDelay = d }
}

// WithCraftRoll supplies the craft success roll in [0,n) (default: the
// random source), so craft outcomes are deterministic.
func WithCraftRoll(roll func(n int) int) Option {
	return func(o *options) { o.craftRoll = roll }
}

// WithFish loads table as the fish a cast fishing line draws from
// (default: none, so every line comes back empty).
func WithFish(table *fish.Table) Option {
	return func(o *options) { o.fish = table }
}

// WithFishingRoll supplies the fishing dice in [0,n) (default: the random
// source), so a fishing run is deterministic.
func WithFishingRoll(roll func(n int) int) Option {
	return func(o *options) { o.fishingRoll = roll }
}

// WithMultisells loads table as the multisell lists (default: none).
func WithMultisells(table *multisell.Table) Option {
	return func(o *options) { o.multisells = table }
}

// WithRollDiceDelay sets the reuse delay between two dice throws on one
// client session. The default 0 leaves throws ungated.
func WithRollDiceDelay(d time.Duration) Option {
	return func(o *options) { o.rollDiceDelay = d }
}

// WithMultisellDelay sets the reuse delay between two multisell exchanges
// on one client (default 0: every exchange request is taken).
func WithMultisellDelay(d time.Duration) Option {
	return func(o *options) { o.multisellDelay = d }
}

// WithKeepMaintainedIngredients boots with players.properties
// BlacksmithUseRecipes off: a maintainIngredient ingredient is kept.
func WithKeepMaintainedIngredients() Option {
	return func(o *options) { o.keepMaintained = true }
}

// WithAugmentations loads table as the augmentation data refines roll
// from and worn augmented weapons draw their bonuses from (default: none,
// every refine refused), with chances (nil: the shipped defaults) and roll
// drawing the refine's random ints (nil: the random source).
func WithAugmentations(table *augmentation.Table, chances *augmentation.Chances, roll augmentation.Rand) Option {
	return func(o *options) {
		o.augmentations, o.augmentationChances, o.augmentRoll = table, chances, roll
	}
}

// WithArmorSets loads table as the armor set data a worn set grants its
// skills from (default: none).
func WithArmorSets(table *armorset.Table) Option {
	return func(o *options) { o.armorSets = table }
}

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

// WithClassTemplate replaces the human-fighter class template (id 0) every
// seeded character selects (default: ClassTemplate), so a suite can give its
// characters, say, a real body size.
func WithClassTemplate(tmpl *player.Template) Option {
	return func(o *options) { o.classTemplate = tmpl }
}

// WithClassTemplates adds class templates beside the default ones, each
// replacing a default of the same id, so a suite can play further classes.
func WithClassTemplates(tmpls ...*player.Template) Option {
	return func(o *options) { o.extraClassTemplates = append(o.extraClassTemplates, tmpls...) }
}

// WithSubclassRules sets players.properties SubClassWithoutQuests and the
// server.properties SubclassTime reuse delay (default false and none).
func WithSubclassRules(withoutQuests bool, delay time.Duration) Option {
	return func(o *options) { o.subclassWithoutQuests, o.subclassDelay = withoutQuests, delay }
}

// SubclassFault decides the outcome of one class change's
// character_subclasses write: op is "insert" or "delete", index the slot.
// A non-nil error fails that write before it reaches the database; the
// function may also panic, as a store or driver bug would.
type SubclassFault func(op string, index int) error

// WithSubclassFault runs fault before every character_subclasses Insert and
// Delete a class change issues.
func WithSubclassFault(fault SubclassFault) Option {
	return func(o *options) { o.subclassFault = fault }
}

// WithLog sets the link logger (default zero-logger).
func WithLog(log zerolog.Logger) Option { return func(o *options) { o.log = log } }

// WithSlowStores makes every persistence write an in-world handler issues —
// item rows, shortcut rows, character_skills rows, pets rows — and the
// pets-row restore read take d, the way a degraded database would. A suite
// pairs it with WithLog to prove no actor-queue task waits on persistence:
// the sim pool's watchdog logs any task that runs longer than 50 ms.
func WithSlowStores(d time.Duration) Option { return func(o *options) { o.slowStores = d } }

// WithCapturedLog sends every component's log to an in-memory buffer the
// suite reads back with Server.LogText or Server.SlowTaskLogs.
func WithCapturedLog() Option { return func(o *options) { o.captureLog = true } }

// WithRealPool runs this Boot's actor queues on the production sim.Pool even
// in a suite that drives its clock, for a test that needs the pool itself:
// its slow-task watchdog (Server.SlowTaskLogs) exists only there.
func WithRealPool() Option { return func(o *options) { o.realPool = true } }

// WithGeo supplies the movement geodata collaborator wired into live
// players. The default is the always-passable Geo double.
func WithGeo(geo move.Geo) Option { return func(o *options) { o.geo = geo } }

// WithProductionTickers starts the recurring tasks the production boot runs
// on the move/attack path — position updates, effects, attack stance,
// inventory updates, item persistence, NPC AI and NPC regen — on their real
// wall-clock tickers, stopped at cleanup, and exposes the AI registry as
// Server.AI. Without it every one of them stays idle and suites drive them by
// hand. Effects is the process-wide shared registry, so a suite using this
// must not tick it by hand in parallel.
func WithProductionTickers() Option { return func(o *options) { o.productionTickers = true } }

// WithAITask wires the AI registry the production boot runs summons and NPCs
// on, without starting its ticker: the suite drives each one-second AI cycle
// itself through Server.AI.Tick.
func WithAITask() Option { return func(o *options) { o.handAI = true } }

// WithItemTemplates boots against tbl instead of the shared catalog, so a
// suite can hand the server a catalog it also holds a reference to and edit
// a template mid-test — what a datapack edit looks like from the inside.
func WithItemTemplates(tbl *item.Table) Option {
	return func(o *options) { o.itemTemplates = tbl }
}

func bootGeo(geo move.Geo) move.Geo {
	if geo != nil {
		return geo
	}
	return Geo{}
}

// Server is a booted gameserver stack plus its first connected client.
type Server struct {
	Client *testsupport.ScriptedClient
	State  *world.State
	DB     *sql.DB
	// RaidPoints is the players' raid points, restored at boot.
	RaidPoints       *raidpoint.Points
	Chars            *gamesql.CharacterStore
	Items            *gamesql.ItemStore
	Shortcuts        *gamesql.ShortcutStore
	Hennas           *gamesql.HennaStore
	RecipeBooks      *gamesql.RecipeBookStore
	KnownSkills      *gamesql.CharacterSkillStore
	Pets             *gamesql.PetStore
	InventoryUpdates *task.InventoryUpdates
	ItemInstances    *task.ItemInstances
	GroundItems      *task.GroundItems
	ShadowItems      *task.ShadowItems
	AttackStance     *task.AttackStance
	Effects          *task.Effects
	AI               *task.AI
	Water            *task.Water // set by WithWater; nil otherwise
	BuyListStock     *merchant.Stock
	BuyListRows      *gamesql.BuyListStore
	WorldObjects     *gamemanager.WorldObjects // doors spawned by WithDoors; nil otherwise
	doors            *doorHarness              // geodata and regeneration of WithDoors' doors
	Boats            *boat.Fleet               // boats sailing WithBoats' itineraries; nil otherwise
	Derby            *derby.Track              // race track of WithDerbyTrack; nil otherwise
	NpcSpawns        *gamemanager.Npcs         // live NPC population of WithNpcSpawns; nil otherwise
	Relations        *relation.Manager         // friend and block lists the link was wired with
	relationRows     *gamesql.RelationStore
	Petitions        *petition.Manager // petitions the link was wired with
	petitionRows     *gamesql.PetitionStore
	Couples          *wedding.Manager // couples the link was wired with
	coupleRows       *gamesql.CoupleStore
	Clans            *clan.Service
	SevenSigns       *sevensigns.State
	AnnounceFile     string // the announcements.xml the server reads and rewrites
	account          string
	templates        *player.TemplateTable
	itemTable        *item.Table
	levelTable       *player.LevelTable
	deepBlueDrops    bool
	autoLoot         bool
	ids              *sequentialIDs
	positions        *task.PositionUpdates
	addr             net.Addr
	sessions         *manager.SessionStore
	groundStore      *gamesql.GroundItemStore
	cursedWeapons    *entity.CursedWeaponTable
	autosave         *task.Autosave
	gameClock        *task.GameClock
	autosaveClock    *autosaveClock
	persist          *persist.Worker
	logs             *lockedBuffer
	// positionTicks is when TickPositions last posted a tick on the real
	// pool, guarded by its mutex.
	positionTicks struct {
		sync.Mutex
		last time.Time
	}
	// effectEnv is the Env every effect list this server builds shares.
	effectEnv effect.Env
	// castEffects is the link's hostile-NPC cast seam, as boot wires it.
	castEffects actorcast.EffectHandlers
	// rewardParties resolves a kill's party for the hostiles the suite spawns.
	rewardParties gamemanager.RewardParties
	// raidKills credits the raid boss kills of the hostiles the suite
	// spawns.
	raidKills gamemanager.RaidKillRecorder
	// stance is the stance tracker the link was wired with, nil when none
	// was; fixture NPCs report their attack stances to it.
	stance network.AttackStanceTracker
	// maxGeoPathFail is each fixture hostile's MaxGeopathFailCount.
	maxGeoPathFail int
	// decay is the corpse-decay task WithDecay supplied; nil when none was.
	decay *task.Decay
	// zones is the zone index WithZones supplied; nil when none was.
	zones        *zone.Index
	queues       *queues
	traffic      *traffic
	heldLanes    [persist.Lanes]atomic.Int32 // HoldPersistenceLane holds per lane
	log          zerolog.Logger
	sendObserver *atomic.Pointer[func(payload []byte)]
	// refreshRecommendations runs the link's daily recommendation refresh.
	refreshRecommendations func(context.Context) error

	closeOnce    sync.Once
	cancel       context.CancelFunc
	waitHandlers func()
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

	readCharSelectInfo(t, c, account, wantChars)
	// Registered once logged in: the handshake reads wait on the wall
	// clock, as Boot's do, never by moving a driven clock.
	s.addClient(c)
	return c
}

// handshakeReadTimeout bounds the wait for the CharSelectInfo that answers a
// harness client's AuthLogin. That reply follows the session check over the
// login link and the account's character rows from the shared database, which
// a machine running every suite at once can hold past a test body's 5s read; a
// server that never answers still fails the test.
const handshakeReadTimeout = 30 * time.Second

// readCharSelectInfo reads the CharSelectInfo that answers c's AuthLogin and
// checks it lists wantChars characters for account.
func readCharSelectInfo(tb testing.TB, c *testsupport.ScriptedClient, account string, wantChars int) {
	tb.Helper()
	reply := c.ReadWithTimeout(handshakeReadTimeout)
	if reply == nil {
		tb.Fatalf("CharSelectInfo for %s not received within %v", account, handshakeReadTimeout)
	}
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		tb.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != int32(wantChars) {
		tb.Fatalf("char count for %s = %d, want %d", account, count, wantChars)
	}
}

// onlineCharacter resolves the online player objID to its character,
// failing the test when no such player is in the world.
func (s *Server) onlineCharacter(tb testing.TB, objID int32) *player.Character {
	tb.Helper()
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("world.Player(%d) missing", objID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		tb.Fatalf("world.Player(%d) = %T is not an online character", objID, obj)
	}
	return c
}

// MarkPlayerDead transitions the live player's dead state without routing a
// kill through the combat stack, which its own suites drive end to end; item
// suites use it only to set up gate preconditions no single packet reaches.
func (s *Server) MarkPlayerDead(tb testing.TB, objID int32) {
	tb.Helper()
	marker := s.onlineCharacter(tb, objID)
	marker.MarkDead()
}

// PlayerDead reports the live player's dead state, the reader half of
// MarkPlayerDead.
func (s *Server) PlayerDead(tb testing.TB, objID int32) bool {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return reader.Dead()
}

// PlayerCharges reports the live player's force/soul charge count.
func (s *Server) PlayerCharges(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return reader.Charges()
}

// PlayerCastingNow reports whether the live player has a cast in flight.
func (s *Server) PlayerCastingNow(tb testing.TB, objID int32) bool {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return reader.CastingNow()
}

// SetPlayerOperating opens (a sell store, with nothing listed) or closes
// the live player's private store, the precondition of the store-mode
// gates.
func (s *Server) SetPlayerOperating(tb testing.TB, objID int32, operating bool) {
	tb.Helper()
	operate := privatestore.OperateNone
	if operating {
		operate = privatestore.OperateSell
	}
	s.SetPlayerOperateType(tb, objID, operate)
}

// PlayerOperateType returns what the live player's private store is doing.
func (s *Server) PlayerOperateType(tb testing.TB, objID int32) privatestore.OperateType {
	tb.Helper()
	return s.onlineCharacter(tb, objID).OperateType()
}

// SetPlayerOperateType sets what the live player's private store is doing.
func (s *Server) SetPlayerOperateType(tb testing.TB, objID int32, operate privatestore.OperateType) {
	tb.Helper()
	setter := s.onlineCharacter(tb, objID)
	setter.SetOperateType(operate)
}

// SetPlayerInCombat toggles the live player's combat flag, the precondition
// the auto-attack stop path checks before it touches the stance tracker.
func (s *Server) SetPlayerInCombat(tb testing.TB, objID int32, inCombat bool) {
	tb.Helper()
	setter := s.onlineCharacter(tb, objID)
	setter.SetInCombat(inCombat)
}

// SetPlayerFishing toggles the live player's fishing flag, the precondition
// of the while-fishing gates.
func (s *Server) SetPlayerFishing(tb testing.TB, objID int32, fishing bool) {
	tb.Helper()
	s.onlineCharacter(tb, objID).SetFishing(fishing)
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

// DespawnGroundItem takes the ground item objectID out of the world the way
// the cleanup tick expires one: claimed, untracked, then despawned, so
// every player that saw it is sent DeleteObject. Suites use it for an item
// that vanishes while a pickup of it waits.
func (s *Server) DespawnGroundItem(tb testing.TB, objectID int32) {
	tb.Helper()
	obj, _ := s.State.Object(objectID)
	ground, ok := obj.(*grounditem.Item)
	if !ok {
		tb.Fatalf("world.Object(%d) = %T, want a ground item", objectID, obj)
	}
	if !ground.Claim() {
		tb.Fatalf("ground item %d is held by a pickup in flight", objectID)
	}
	s.GroundItems.Remove(ground)
	s.State.Despawn(ground)
}

// SetPlayerFlying toggles the live player's transport mode, the precondition
// of the datapack's flying use conditions no single packet reaches.
func (s *Server) SetPlayerFlying(tb testing.TB, objID int32, flying bool) {
	tb.Helper()
	setter := s.onlineCharacter(tb, objID)
	setter.SetFlying(flying)
}

// DisablePlayerItem installs a per-item reuse disable on the live player,
// mirroring what a timed-task disable produces; item suites use it only to
// set up gate preconditions no single packet reaches.
func (s *Server) DisablePlayerItem(tb testing.TB, objID, objectID int32, delay time.Duration) {
	tb.Helper()
	disabler := s.onlineCharacter(tb, objID)
	disabler.DisableItem(objectID, delay)
}

// SetInventorySlotLimit pins the live player's inventory slot limit to limit,
// replacing the character's own, so a full-inventory rejection is reachable
// without seeding dozens of rows.
func (s *Server) SetInventorySlotLimit(tb testing.TB, objID int32, limit int) {
	tb.Helper()
	holder := s.onlineCharacter(tb, objID)
	holder.Inventory().SetLimiter(fixedSlotLimit{limit: limit, owner: holder})
}

// fixedSlotLimit is a slot limit that never changes; the weight limit stays
// the owner's own.
type fixedSlotLimit struct {
	limit int
	owner itemcontainer.Limiter
}

func (l fixedSlotLimit) InventoryLimit() int { return l.limit }
func (l fixedSlotLimit) WeightLimit() int    { return l.owner.WeightLimit() }

// PlayerInventory returns the live player's inventory so suites can stage a
// mutation from a queue task, where no client packet can reach: the
// connection goroutine blocks on each request it posts, so work it sends
// can never land behind a request task that is still running.
func (s *Server) PlayerInventory(tb testing.TB, objID int32) *itemcontainer.Inventory {
	tb.Helper()
	return s.onlineCharacter(tb, objID).Inventory()
}

// PlayerQueue returns the live player's actor queue so suites can park it on
// a gate task and pin the order of work queued behind that gate.
//
// Parking is not per-player on the inline executor: every queue there shares
// one FIFO drained by a single runner, so a gate blocks every queue in the
// process and freezes the virtual clock until it is released, which also
// stops every Queue.After and Queue.Every. Only a suite that drives one actor
// and posts its own ticks may park a queue; one that waits on another actor
// or on a sim timer hangs under inline while passing under pool.
func (s *Server) PlayerQueue(tb testing.TB, objID int32) *sim.Queue {
	tb.Helper()
	return s.onlineCharacter(tb, objID).Queue()
}

// PlayerMove returns the live player's movement state so suites can drive
// interpolation ticks and fire the blocked-arrival path.
func (s *Server) PlayerMove(tb testing.TB, objID int32) *move.CreatureMove {
	tb.Helper()
	mover := s.onlineCharacter(tb, objID)
	return mover.Move()
}

// TickPlayerBlocked advances two interpolation ticks then closes geo so the
// next tick fires the blocked-arrival hook. Returns the cell the walk had
// reached before the path closed.
func (s *Server) TickPlayerBlocked(tb testing.TB, objID int32, geo *GateGeo) location.Location {
	tb.Helper()
	mover := s.PlayerMove(tb, objID)
	for i := 0; i < 2; i++ {
		if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); !moving {
			tb.Fatalf("UpdatePosition() tick %d moving = false, want origin to leave start", i+1)
		}
	}
	advanced := mover.Position()
	geo.Block()
	if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); moving {
		tb.Fatal("UpdatePosition() moving = true after path closed, want blocked stop")
	}
	return advanced
}

// ReadMoveToLocationCoords decodes object id, destination, and origin from a
// MoveToLocation frame (opcode byte included).
func ReadMoveToLocationCoords(tb testing.TB, frame []byte) (objectID int32, dest, origin location.Location) {
	tb.Helper()
	if len(frame) < 1 {
		tb.Fatal("MoveToLocation frame empty")
	}
	r := wire.NewReader(frame[1:])
	objectID = r.ReadInt32()
	dest.X = int(r.ReadInt32())
	dest.Y = int(r.ReadInt32())
	dest.Z = int(r.ReadInt32())
	origin.X = int(r.ReadInt32())
	origin.Y = int(r.ReadInt32())
	origin.Z = int(r.ReadInt32())
	if err := r.Err(); err != nil {
		tb.Fatalf("read MoveToLocation: %v", err)
	}
	return objectID, dest, origin
}

// PlayerPosition reports the live player's current world position.
func (s *Server) PlayerPosition(tb testing.TB, objID int32) (int, int, int) {
	tb.Helper()
	located := s.onlineCharacter(tb, objID)
	return located.Position()
}

// PlayerTotalWeight reports the live inventory's last-computed total weight.
func (s *Server) PlayerTotalWeight(tb testing.TB, objID int32) int {
	tb.Helper()
	holder := s.onlineCharacter(tb, objID)
	return holder.Inventory().TotalWeight()
}

// DrainPlayerMP reduces the live player's current MP by amount, so restore
// flows have observable headroom no single packet creates.
func (s *Server) DrainPlayerMP(tb testing.TB, objID int32, amount int) {
	tb.Helper()
	reducer := s.onlineCharacter(tb, objID)
	reducer.ReduceCurrentMP(amount)
}

// PlayerCurrentMP reports the live player's current MP.
func (s *Server) PlayerCurrentMP(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return reader.CurrentMP()
}

// PlayerCurrentCP reports the live player's current CP.
func (s *Server) PlayerCurrentCP(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return reader.CurrentCP()
}

// DamagePlayerHP reduces the live player's current HP by amount, so heal
// flows have observable headroom no single packet creates.
func (s *Server) DamagePlayerHP(tb testing.TB, objID int32, amount int) {
	tb.Helper()
	reducer := s.onlineCharacter(tb, objID)
	reducer.ReduceCurrentHP(amount)
}

// AddPlayerHP restores HP to the live player and reports the amount that
// actually landed, so a suite can place HP at a fractional value the
// integer-reporting packet surface cannot express and still prove the
// placement was not clamped away.
func (s *Server) AddPlayerHP(tb testing.TB, objID int32, amount float64) float64 {
	tb.Helper()
	healer := s.onlineCharacter(tb, objID)
	return healer.AddHP(amount)
}

// AddPlayerMP restores MP to the live player and reports the amount that
// actually landed, so a suite can place MP at an exact value after draining
// the pool.
func (s *Server) AddPlayerMP(tb testing.TB, objID int32, amount float64) float64 {
	tb.Helper()
	restorer := s.onlineCharacter(tb, objID)
	return restorer.AddMP(amount)
}

// PlayerCurrentHP reports the live player's current HP.
func (s *Server) PlayerCurrentHP(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return reader.CurrentHP()
}

// PlayerMaxHP reports the live player's stat-computed maximum HP.
func (s *Server) PlayerMaxHP(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return int(reader.MaxHPValue())
}

// PlayerMaxCP reports the live player's stat-computed maximum CP.
func (s *Server) PlayerMaxCP(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return int(reader.MaxCPValue())
}

// PlayerMaxMP reports the live player's calculated max MP.
func (s *Server) PlayerMaxMP(tb testing.TB, objID int32) int {
	tb.Helper()
	reader := s.onlineCharacter(tb, objID)
	return int(reader.MaxMPValue())
}

// FlushItems persists every pending item mutation the way the production
// lazy-persistence tick does, so suites can assert the items rows mid-test.
// It then waits for the persistence worker, which carries both the tick's own
// writes and the per-action item writes a handler queued (network's
// applyPersistActions).
func (s *Server) FlushItems(tb testing.TB) {
	tb.Helper()
	if err := s.ItemInstances.Save(context.Background()); err != nil {
		tb.Fatalf("flush items: %v", err)
	}
	if err := s.flushPersistence(); err != nil {
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
//
// It first settles, as the production stop order closes the listener and
// stops the actor pool before those saves: a request still being handled —
// a drop whose DropItem frame is already out but whose ground item is not
// yet tracked — finishes before anything is snapshotted.
func (s *Server) Shutdown(tb testing.TB) {
	tb.Helper()
	s.Settle(tb)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
	defer cancel()
	if err := s.ItemInstances.Save(ctx); err != nil {
		tb.Fatalf("shutdown item flush: %v", err)
	}
	if err := s.persist.Flush(ctx); err != nil {
		tb.Fatalf("shutdown persistence flush: %v", err)
	}
	if err := s.groundStore.Save(ctx, s.GroundItems.Snapshots(s.skipCursedGroundItem)); err != nil {
		tb.Fatalf("shutdown ground-item save: %v", err)
	}
	s.Close()
	// Boot's own t.Cleanup(taskEffects.Reset) only fires at the end of the
	// whole test function, not between two Boot calls a restart test makes
	// within one function — reset explicitly here too, so the second
	// Boot's Effects.Tick doesn't also carry this server's leftovers.
	s.Effects.Reset()
}

// CrossDayNight ticks the in-game clock the player clock runs on, one
// in-game minute at a time, until it crosses the day/night boundary, so the
// day/night listeners run once; the per-minute listeners run on every tick.
// Boot does not start the game-minute ticker.
// It reports whether night has just fallen.
func (s *Server) CrossDayNight(tb testing.TB) bool {
	tb.Helper()
	night := s.gameClock.IsNight()
	for range 24 * 60 {
		s.gameClock.Tick()
		if s.gameClock.IsNight() != night {
			return !night
		}
	}
	tb.Fatal("game clock never crossed the day/night boundary")
	return false
}

// TickAutosave advances the harness clock past the next autosave deadline
// and runs one production sweep. Boot does not start the autosave ticker,
// so tests that need the periodic save path call this instead of waiting
// AutosaveInitialDelay.
func (s *Server) TickAutosave(tb testing.TB) {
	tb.Helper()
	s.QueueAutosave()
	s.FlushPersistence(tb)
}

// QueueAutosave is TickAutosave without waiting for the sweep's queued
// writes, so a suite can hold a lane and act while they are pending.
func (s *Server) QueueAutosave() {
	if s.autosave == nil || s.autosaveClock == nil {
		return
	}
	s.autosaveClock.Advance(task.AutosaveInitialDelay)
	s.autosave.Tick()
	// The sweep runs each save on its player's queue; once those tasks
	// have run, every write is on its persistence lane.
	if err := s.queues.settle(); err != nil {
		panic(err)
	}
}

// HoldPersistenceLane blocks ownerID's persistence lane until the returned
// release is called (also on cleanup), so a suite can prove a reader waits
// for writes queued behind it.
func (s *Server) HoldPersistenceLane(tb testing.TB, ownerID int32) (release func()) {
	tb.Helper()
	held := make(chan struct{})
	started := make(chan struct{})
	lane := &s.heldLanes[persist.LaneIndex(ownerID)]
	lane.Add(1)
	s.persist.Enqueue(ownerID, func() {
		close(started)
		<-held
	})
	<-started
	var once sync.Once
	release = func() {
		once.Do(func() {
			lane.Add(-1)
			close(held)
		})
	}
	tb.Cleanup(release)
	return release
}

// FlushPersistence waits until every save already handed to the persistence
// worker (autosave, detach, pet and container writes) has run, so a suite
// can assert the rows those paths wrote.
func (s *Server) FlushPersistence(tb testing.TB) {
	tb.Helper()
	if err := s.flushPersistence(); err != nil {
		tb.Fatalf("flush persistence: %v", err)
	}
}

func (s *Server) flushPersistence() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
	defer cancel()
	return s.persist.Flush(ctx)
}

// Settle waits until the server has handled every frame a client already
// wrote, then until every task already posted to an actor queue has run: a
// packet handler's follow-up, a timer that fired, or the per-actor work a
// tick fanned out. Work those tasks post in turn may still be pending.
//
// Without the first wait a frame still crossing the socket (or waiting for
// its connection goroutine) posts its work after the queues were checked, so
// a Send followed by Settle would not see the frame's effect on the real
// pool, where settling idle queues is quicker than a socket round trip.
func (s *Server) Settle(tb testing.TB) {
	tb.Helper()
	if err := s.awaitHandled(); err != nil {
		tb.Fatal(err)
	}
	if err := s.queues.settle(); err != nil {
		tb.Fatal(err)
	}
}

// AwaitHandled is Settle's first wait alone: it returns once the server has
// handled every frame a client already wrote, without waiting on the actor
// queues. A test holding an actor's queue uses it to know a request was
// answered, or refused, before it checks what the client received.
func (s *Server) AwaitHandled(tb testing.TB) {
	tb.Helper()
	if err := s.awaitHandled(); err != nil {
		tb.Fatal(err)
	}
}

// NewObjectID allocates the next object id from the server's id sequence.
func (s *Server) NewObjectID() int32 {
	id, err := s.ids.NextID()
	if err != nil {
		panic(err)
	}
	return id
}

// Stop stops serving the way the production listener stop hook does and
// waits for every connection handler to return, so each connected player has
// been detached and its saves have run.
func (s *Server) Stop() {
	s.Close()
	s.waitHandlers()
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

// pages is the link's HTML page cache content: the help tutorial page plus
// every WithHTMLPages page.
func (o *options) pages() map[string]string {
	pages := map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"}
	for name, content := range o.htmlPages {
		pages[name] = content
	}
	return pages
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
		karmaServiceGates:      [3]bool{false, false, true},
		karmaPlayerCanTrade:    true,
		characterSelectDelay:   3 * time.Second,
		serverBypassDelay:      100 * time.Millisecond,
		maxBuffsAmount:         20,
		cancelLesserEffect:     true,
		magicFailures:          true,
		storeSkillCooltime:     true,
		weightLimitMultiplier:  1,
	}
	for _, opt := range opts {
		opt(o)
	}

	var logs *lockedBuffer
	if o.captureLog {
		logs = &lockedBuffer{}
		o.log = zerolog.New(logs)
	}

	db := sqltest.SharedDB(t)
	chars := gamesql.NewCharacterStore(db)
	items := gamesql.NewItemStore(db)
	shortcuts := gamesql.NewShortcutStore(db)
	hennas := gamesql.NewHennaStore(db)
	recipeBooks := gamesql.NewRecipeBookStore(db)
	subclasses := gamesql.NewSubclassStore(db)
	knownSkills := gamesql.NewCharacterSkillStore(db)
	if o.skills == nil {
		skillTable := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}})
		if o.slowStores > 0 {
			o.skills = skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skillTable, slowCharacterSkillStore{CharacterSkillStore: knownSkills, delay: o.slowStores})
		} else {
			o.skills = skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skillTable, knownSkills)
		}
	}
	o.skills.SetStoreSkillCooltime(o.storeSkillCooltime)
	if err := o.skills.SetAugmentations(o.augmentations); err != nil {
		t.Fatalf("augmentation bonuses: %v", err)
	}
	o.skills.SetArmorSets(o.armorSets)
	if o.seed != nil {
		o.seed(chars, items)
	}
	relationRows := gamesql.NewRelationStore(db)
	loadedRelations, err := relationRows.Load(context.Background())
	if err != nil {
		t.Fatalf("load character relations: %v", err)
	}
	relations := relation.NewManager(loadedRelations)
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

	loginAddr, servers, sessions := startLoginServerAcceptor(t, loginsql.NewAccountStore(db))
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
	taskEffects := task.NewEffects()
	// Registered early so it runs last (t.Cleanup is LIFO): everything
	// else this Boot registers for cleanup — including the connection
	// teardown that Untracks a logged-out player's effect list — gets to
	// run first, and Reset only needs to mop up whatever a test left
	// registered without a clean teardown (an NPC or EffectPoint the test
	// never decayed or despawned), so the next Boot in this process starts
	// from an empty registry instead of also ticking this test's leftovers.
	t.Cleanup(taskEffects.Reset)
	effectEnv := effect.Env{Activity: taskEffects, KeepLesser: !o.cancelLesserEffect, Night: o.night}
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
	class0 := ClassTemplate()
	if o.classTemplate != nil {
		class0 = o.classTemplate
	}
	templates := templatesWith(t, class0, o.extraClassTemplates...)
	itemTemplates := o.itemTemplates
	if itemTemplates == nil {
		itemTemplates = ItemTemplates()
	}
	ids := &sequentialIDs{next: 100}
	var worldObjects *gamemanager.WorldObjects
	var doors *doorHarness
	if len(o.doors) > 0 {
		worldObjects, doors = bootDoors(t, o.doors, ids, state)
	}
	boats := bootBoats(t, o.boats, ids, state)
	derbyTrack := bootDerbyTrack(t, o.derby, db, ids, state, o.zones)
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
	// Registered before the listener's cleanup, so it drains only after
	// every connection's detach has enqueued its saves.
	persistWorker := persist.New(o.log)
	// Known-skill writes run on the worker in production too, so suites see
	// the same ordering (FlushPersistence waits for them).
	o.skills.SetPersistWorker(persistWorker, o.log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
		defer cancel()
		if err := persistWorker.Close(ctx); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	// Registered after the persistence worker and before the listener, so
	// it stops once every connection has detached on its queue and before
	// the worker drains.
	queues := startQueues(t, o.log, o.realPool)
	// Both writers of the items table share one ordering, as production does.
	itemWrites := persist.NewOrder()
	var itemFlusher task.ItemFlusher = gamesql.NewItemFlushStore(db)
	if o.itemFlushFault != nil {
		itemFlusher = faultyItemFlusher{inner: itemFlusher, fault: o.itemFlushFault}
	}
	itemInstances := task.NewItemInstances(itemFlusher, itemTemplates, persistWorker, itemWrites, zerolog.Nop())
	petStore := gamesql.NewPetStore(db)

	// Mirror the production boot for the Seven Signs calendar: optional
	// test seed first, then restore the persisted status and arm the
	// transition timer.
	sevenSignsStore := gamesql.NewSevenSignsStore(db)
	if o.seedSevenSigns != nil {
		o.seedSevenSigns(sevenSignsStore)
	}
	sevenSigns := sevensigns.NewState(sevenSignsStore, network.NewSevenSignsBroadcaster(state), o.log, time.Now, nil)
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
	roster.SetSubclasses(subclasses)
	effects.SetAutosave(roster, o.skills, petStore, persistWorker, zerolog.Nop())
	autosaveClock := &autosaveClock{now: time.Now()}
	autosave, err := task.NewAutosave(effects, autosaveClock.Now)
	if err != nil {
		t.Fatalf("new autosave: %v", err)
	}
	positions := task.NewPositionUpdates(state)
	var ai *task.AI
	if o.handAI {
		ai = task.NewAI(state, o.log)
	}
	if o.productionTickers {
		ai = task.NewAI(state, o.log)
		if o.attackStance == nil && o.attackStanceNow == nil {
			o.attackStanceNow = time.Now
		}
	}
	buyListStore := gamesql.NewBuyListStore(db)
	shops, stock := bootMerchant(t, o.merchant, buyListStore, persistWorker, ids, o.log)
	gclConfig := network.GameClientLinkConfig{
		Merchant:         shops,
		Validator:        validator,
		Effects:          effectEnv,
		LoginLink:        func() *network.LoginLink { return loginLink },
		Roster:           roster,
		Items:            items,
		Shortcuts:        shortcuts,
		Hennas:           hennas,
		HennaTable:       cmp.Or(o.hennas, HennaTemplates(t)),
		RecipeBooks:      recipeBooks,
		Subclasses:       subclasses,
		Recipes:          cmp.Or(o.recipes, RecipeTemplates()),
		Multisells:       o.multisells,
		CraftRoll:        o.craftRoll,
		Fish:             o.fish,
		FishingRoll:      o.fishingRoll,
		Templates:        templates,
		ItemTemplates:    itemTemplates,
		HTML:             HTMLCache(t, o.pages()),
		Crests:           crests,
		Skills:           o.skills,
		Spellbooks:       o.spellbooks,
		SkillTrees:       o.trees,
		HealSps:          o.healSps,
		CursedWeapons:    cursed,
		World:            state,
		NPCs:             o.npcs,
		SummonItems:      o.summonItems,
		PetStore:         petStore,
		Geo:              bootGeo(o.geo),
		IDs:              ids,
		GroundItems:      groundItems,
		Positions:        positions,
		PlayerClock:      playerClock,
		GameClock:        task.NewGameClock(time.Now),
		PvPFlags:         task.NewPvPFlags(task.DefaultPvPFlagOptions(), time.Now),
		SevenSigns:       sevenSigns,
		InventoryUpdates: inventoryUpdates,
		ItemInstances:    itemInstances,
		Persist:          persistWorker,
		ItemWrites:       itemWrites,
		PersistWait:      o.persistWait,
		Queues:           queues,
		ShadowItems:      shadowItems,
		Autosave:         autosave,
		PlayerConfig:     network.PlayerConfig{Enchant: o.enchantConfig, RespawnRestoreHP: 0.7, SkillEnchantSPBookNeeded: true, KarmaPlayerCanTeleport: o.karmaPlayerCanTeleport, KarmaPlayerCanShop: o.karmaServiceGates[0], KarmaPlayerCanUseGK: o.karmaServiceGates[1], KarmaPlayerCanUseWareHouse: o.karmaServiceGates[2], KarmaPlayerCanTrade: o.karmaPlayerCanTrade, AllowWater: !o.disallowWater, EnableFallingDamage: !o.disableFallingDamage, PerfectShieldBlockRate: 5, SpawnProtection: o.spawnProtection, AllowDelevel: o.allowDelevel, RateKarmaExpLost: o.rateKarmaExpLost, CharacterSelectDelay: o.characterSelectDelay, ServerBypassDelay: o.serverBypassDelay, CraftingDisabled: o.craftingDisabled, DiscardItemDisabled: o.discardItemDisabled, GMStartupUnlisted: o.gmStartupUnlisted, ManufactureDelay: o.manufactureDelay, MultisellDelay: o.multisellDelay, RollDiceDelay: o.rollDiceDelay, SubclassDelay: o.subclassDelay, SubclassWithoutQuests: o.subclassWithoutQuests, KeepMaintainedIngredients: o.keepMaintained, MaxBuffsAmount: o.maxBuffsAmount, MagicFailures: o.magicFailures, WeightLimitMultiplier: o.weightLimitMultiplier, InventorySlots: o.inventorySlots, StorageSlots: o.storageSlots, Freight: o.freight, PartyRange: fixturePartyRange},
		Restarts:         o.restarts,
		Teleports:        o.teleports,
		InstantTeleports: o.instantTeleports,
		FreeTeleport:     o.freeTeleport,
		TeleportClock:    o.teleportClock,
		TradeClock:       o.tradeClock,
		Zones:            o.zones,
		PetConfig:        petmodel.DefaultConfig(),
		EnchantRoll:      o.enchantRoll,
		SkillEnchantRoll: o.skillEnchantRoll,
		Levels:           levels,
		Admin:            o.admin,
		GMAudit:          o.gmAudit,
		Chat:             o.chat,
		Log:              o.log,
	}
	// The clans are restored once the characters are seeded, below.
	clanStore := gamesql.NewClanStore(db)
	clanConfig := clan.DefaultConfig()
	if o.clanConfig != nil {
		clanConfig = *o.clanConfig
	}
	gclConfig.Clans = clan.NewService(clan.NewTable(), clanStore, persistWorker, ids, clanConfig, o.clanClock, o.log)
	// The mail is restored once the characters are seeded, below.
	mailStore := gamesql.NewMailStore(db)
	gclConfig.Board, gclConfig.ShowServerNews = o.board, o.serverNews
	announcementsPath := filepath.Join(t.TempDir(), "announcements.xml")
	gclConfig.Announcements = bootAnnouncements(t, announcementsPath, o.announcements, queues.NewQueue("announcements"), state, o.log)
	forumStore, favoriteStore := gamesql.NewForumStore(db), gamesql.NewFavoriteStore(db)
	if o.board.Enabled {
		gclConfig.Mailbox = bbs.NewMailbox(mailStore, persistWorker, o.log)
		gclConfig.Forums = bbs.NewForums(forumStore, persistWorker, o.log)
		gclConfig.Favorites = bbs.NewFavorites(favoriteStore, persistWorker, o.log)
	}
	gclConfig.PlayerConfig.AutoLearnSkills = o.autoLearnSkills
	gclConfig.PlayerConfig.GMStartupInvulnerable, gclConfig.PlayerConfig.GMStartupInvisible, gclConfig.PlayerConfig.GMStartupBlockAll = o.gmStartupModes[0], o.gmStartupModes[1], o.gmStartupModes[2]
	gclConfig.Augmentations, gclConfig.AugmentRoll = o.augmentations, o.augmentRoll
	gclConfig.ArmorSets = o.armorSets
	gclConfig.Manor = o.manor
	gclConfig.SchemeBuffer = o.schemeBuffer
	gclConfig.Derby = derbyTrack
	gclConfig.Relations, gclConfig.Characters = relations, chars
	gclConfig.AccessLevels = chars
	gclConfig.Punishments = chars
	petitions, petitionRows := bootPetitions(t, db, chars, ids, o.petitionConfig)
	gclConfig.Petitions = petitions
	couples, coupleRows := bootWedding(t, db, ids, o.weddingConfig)
	gclConfig.Wedding = couples
	gclConfig.Macros = gamesql.NewMacroStore(db)
	gclConfig.Recommendations = gamesql.NewRecommendationStore(db)
	gclConfig.AugmentationChances = augmentation.DefaultChances()
	if o.augmentationChances != nil {
		gclConfig.AugmentationChances = *o.augmentationChances
	}
	var water *task.Water
	if o.water {
		var err error
		water, err = task.NewWater(effects, o.waterNow)
		if err != nil {
			t.Fatalf("new water: %v", err)
		}
		gclConfig.Water = water
	}
	if o.subclassFault != nil {
		gclConfig.Subclasses = faultySubclassStore{SubclassStore: subclasses, fault: o.subclassFault}
	}
	if o.slowStores > 0 {
		gclConfig.Items = slowItemStore{ItemStore: items, delay: o.slowStores}
		gclConfig.Shortcuts = slowShortcutStore{ShortcutStore: shortcuts, delay: o.slowStores}
		gclConfig.PetStore = slowPetStore{PetStore: petStore, delay: o.slowStores}
	}
	if o.petNameLookupErr != nil {
		if o.slowStores > 0 {
			t.Fatal("gameservertest: WithPetNameLookupError cannot be combined with WithSlowStores")
		}
		gclConfig.PetStore = failingPetNameStore{PetStore: petStore, err: o.petNameLookupErr}
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
	// Checked here rather than with the other option validation: the wiring
	// below is unconditional, so a guard inside any narrower block would be
	// dead for the suites that can actually trip it.
	if o.attackStance != nil && o.attackStanceTracker != nil {
		t.Fatalf("WithAttackStance and WithAttackStanceTracker both set: they wire the same link collaborator")
	}
	if o.attackStanceTracker != nil {
		gclConfig.AttackStance = o.attackStanceTracker
	} else if attackStance != nil {
		gclConfig.AttackStance = attackStance
	}
	if o.pvpFlags != nil {
		gclConfig.PvPFlags = o.pvpFlags
	}
	gclConfig.Decay = o.decay
	if ai != nil {
		gclConfig.AI = ai
	}
	if worldObjects != nil {
		gclConfig.Doors = worldObjects
	}
	// The Olympiad's records are restored once the characters are seeded,
	// below; its calendar is not started (see WithOlympiadSeed).
	olympiadState := olympiad.New(olympiad.DefaultConfig(), gamesql.NewOlympiadStore(db), persistWorker, network.NewOlympiadAnnouncer(state), queues.NewQueue("olympiad"), o.log)
	gclConfig.Olympiad = olympiadState
	raidPoints := raidpoint.New(gamesql.NewRaidPointStore(db), persistWorker, o.log)
	gclConfig.RaidPoints = raidPoints
	gcl, err := network.NewGameClientLink(gclConfig)
	if err != nil {
		t.Fatalf("gameservertest: build game client link: %v", err)
	}
	effects.SetShadowItemExpiry(gcl.ExpireShadowItem)
	var npcSpawns *gamemanager.Npcs
	if o.npcSpawns != nil {
		npcSpawns = bootNpcSpawns(t, gcl, npcSpawnDeps{
			state: state, templates: o.npcs, geo: bootGeo(o.geo), ids: ids, decay: o.decay, ai: ai, positions: positions,
			items: itemTemplates, ground: groundItems, effects: effectEnv, queues: queues, stance: gclConfig.AttackStance, log: o.log, makers: o.npcSpawns,
		})
	}
	if o.productionTickers {
		for _, start := range []func(zerolog.Logger) *scheduler.Ticker{
			positions.Start,
			taskEffects.Start,
			attackStance.Start,
			inventoryUpdates.Start,
			itemInstances.Start,
			ai.Start,
			task.NewNPCRegen(state).Start,
		} {
			t.Cleanup(start(o.log).StopAndWait)
		}
	}

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
	waitHandlers := func() {
		handlers.Lock()
		defer handlers.Unlock()
		for handlers.count > 0 {
			handlersDone.Wait()
		}
	}
	t.Cleanup(func() {
		cancel()
		ln.Close()
		waitHandlers()
	})
	sendObserver := new(atomic.Pointer[func(payload []byte)])
	frames := new(traffic)
	serve := func(ctx context.Context, conn *network.Conn) {
		sent, closed := frames.track(conn)
		defer closed()
		conn.ObserveSends(func(payload []byte) {
			sent()
			if observe := sendObserver.Load(); observe != nil {
				(*observe)(payload)
			}
		})
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
	}
	// Serve returns once cleanup cancels ctx; nothing waits on its error.
	go func() { _ = network.Serve(ctx, ln, serve, zerolog.Nop()) }()

	for _, spec := range o.characters {
		tmpl, ok := templates.Get(0)
		if !ok {
			t.Fatal("missing test class template")
		}
		ch, err := player.NewCharacter(ids.nextID(), tmpl, o.account, spec.name, 1, 0, 0, spec.sex)
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
	if o.seedClans != nil {
		o.seedClans(db)
	}
	clanNow := time.Now()
	if err := clanStore.DeleteExpiredWars(context.Background(), clanNow.UnixMilli()); err != nil {
		t.Fatalf("delete expired clan wars: %v", err)
	}
	clanRows, err := clanStore.Load(context.Background())
	if err != nil {
		t.Fatalf("load clans: %v", err)
	}
	if o.skills != nil {
		clanRows.KeepSkills(func(sk clan.Skill) bool {
			return o.skills.HasDefinition(modelskill.Ref{ID: modelskill.ID(sk.ID), Level: sk.Level})
		})
	}
	gclConfig.Clans.Table().Restore(clanRows, clanNow, clanConfig.JoinDays)
	hallOwners, err := clanStore.LoadHallOwners(context.Background())
	if err != nil {
		t.Fatalf("load clan hall owners: %v", err)
	}
	// The fixture loads no clan hall data, so every seeded hall counts.
	gclConfig.Clans.Table().RestoreHalls(hallOwners, nil)
	gclConfig.Clans.DropMissingCrests(crests)
	gclConfig.Clans.DropDanglingAlliances()
	if o.seedBoard != nil {
		o.seedBoard(db)
	}
	if o.seedOlympiad != nil {
		o.seedOlympiad(db)
	}
	if err := olympiadState.Restore(context.Background()); err != nil {
		t.Fatalf("restore olympiad: %v", err)
	}
	t.Cleanup(func() { olympiadState.Stop(context.Background()) })
	if o.seedBoss != nil {
		o.seedBoss(db)
	}
	if err := raidPoints.Restore(context.Background()); err != nil {
		t.Fatalf("restore raid points: %v", err)
	}
	if o.zones != nil {
		if err := gamesql.NewBossZoneStore(db).Restore(context.Background(), zone.OfKind[*zone.Boss](o.zones)); err != nil {
			t.Fatalf("restore boss zones: %v", err)
		}
	}
	if gclConfig.Mailbox != nil {
		mails, _, err := mailStore.Load(context.Background())
		if err != nil {
			t.Fatalf("load mail: %v", err)
		}
		gclConfig.Mailbox.Restore(mails)
		restoreBoardForums(t, forumStore, favoriteStore, gclConfig.Forums, gclConfig.Favorites, gclConfig.Clans)
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
	readCharSelectInfo(t, c, o.account, o.wantChars)

	srv := &Server{
		Client:           c,
		State:            state,
		WorldObjects:     worldObjects,
		doors:            doors,
		Boats:            boats,
		Derby:            derbyTrack,
		Clans:            gclConfig.Clans,
		SevenSigns:       sevenSigns,
		itemTable:        itemTemplates,
		levelTable:       levels,
		deepBlueDrops:    o.deepBlueDropRules,
		autoLoot:         o.autoLoot,
		DB:               db,
		RaidPoints:       raidPoints,
		Chars:            chars,
		Relations:        relations,
		relationRows:     relationRows,
		Petitions:        petitions,
		petitionRows:     petitionRows,
		Couples:          couples,
		coupleRows:       coupleRows,
		Items:            items,
		Shortcuts:        shortcuts,
		Hennas:           hennas,
		RecipeBooks:      recipeBooks,
		KnownSkills:      knownSkills,
		Pets:             petStore,
		InventoryUpdates: inventoryUpdates,
		ItemInstances:    itemInstances,
		GroundItems:      groundItems,
		ShadowItems:      shadowItems,
		AttackStance:     attackStance,
		Effects:          taskEffects,
		effectEnv:        effectEnv,
		castEffects:      gcl.HostileCastEffects(),
		rewardParties:    o.rewardParties(gcl),
		raidKills:        gcl,
		stance:           gclConfig.AttackStance,
		maxGeoPathFail:   o.maxGeoPathFailCount,
		zones:            o.zones,
		decay:            o.decay,
		AI:               ai,
		NpcSpawns:        npcSpawns,
		Water:            water,
		BuyListStock:     stock,
		BuyListRows:      buyListStore,
		account:          o.account,
		templates:        templates,
		ids:              ids,
		positions:        positions,
		addr:             ln.Addr(),
		sessions:         sessions,
		groundStore:      gamesql.NewGroundItemStore(db),
		cursedWeapons:    cursed,
		autosave:         autosave,
		gameClock:        clock,
		autosaveClock:    autosaveClock,
		persist:          persistWorker,
		queues:           queues,
		traffic:          frames,
		log:              o.log,
		logs:             logs,
		cancel:           cancel,
		waitHandlers:     waitHandlers,
		sendObserver:     sendObserver,
	}
	srv.AnnounceFile = announcementsPath
	srv.refreshRecommendations = gcl.RefreshDailyRecommendations
	srv.addClient(c)
	return srv
}

// ObserveSends has fn see the cleartext payload of every frame any of this
// server's connections queues, on the goroutine that queues it, so a suite
// can pin which task sent a reply; see network.Conn.ObserveSends. fn must
// not retain payload. It stays installed until the test ends.
func (s *Server) ObserveSends(tb testing.TB, fn func(payload []byte)) {
	tb.Helper()
	s.sendObserver.Store(&fn)
	tb.Cleanup(func() { s.sendObserver.Store(nil) })
}

// sharedRSAKeys is the GS-LS link key pool every Boot in the process shares.
// Generating a pool costs ten RSA key generations, which dominated suite CPU
// when each Boot made its own; the pool is read-only once built.
var sharedRSAKeys = sync.OnceValues(manager.NewRSAKeyPool)

// startLoginServerAcceptor mirrors the login-side GS-LS acceptor the network
// package's own tests use, so Boot completes a real login handshake.
//
// accounts takes the account access levels the game server sends.
func startLoginServerAcceptor(t *testing.T, accounts *loginsql.AccountStore) (addr string, servers *manager.ServerRegistry, sessions *manager.SessionStore) {
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

	keys, err := sharedRSAKeys()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}

	servers = manager.NewServerRegistry()
	sessions = manager.NewSessionStore()
	bans := manager.NewIPBanList(zerolog.Nop())

	gsLink := loginserver.NewGameServerLink(servers, names, keys, sessions, bans, accounts, nil, false, nil, loginserver.NewLinkRoster(), zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// Serve returns once cleanup cancels ctx; nothing waits on its error.
	go func() { _ = gsLink.Serve(ctx, ln) }()

	return ln.Addr().String(), servers, sessions
}

// SaveRelations writes the friend and block lists to character_relations,
// as the shutdown save does.
func (s *Server) SaveRelations(tb testing.TB) {
	tb.Helper()
	if err := s.relationRows.Save(context.Background(), s.Relations.Changes()); err != nil {
		tb.Fatalf("save character relations: %v", err)
	}
}
