package network

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	"github.com/fatal10110/acis_golang/internal/gameserver/augment"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/craft"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/exchange"
	"github.com/fatal10110/acis_golang/internal/gameserver/gatekeeper"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/partymatch"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/fatal10110/acis_golang/internal/gameserver/symbolmaker"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// itemStore writes item rows from states the caller copied while it owned
// the instances: the write runs on a persistence lane, after the queue task
// that produced it has moved on (applyPersistActions). WriteBatch lands all
// of its rows in one transaction or none of them.
type itemStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]*item.Instance, error)
	WriteBatch(ctx context.Context, batch item.FlushBatch) error
	DeleteOwned(ctx context.Context, ownerID, objectID int32) (bool, error)
	SetEnchantOwned(ctx context.Context, ownerID, objectID int32, enchant int) (bool, error)
}

type shortcutStore interface {
	ListByOwner(ctx context.Context, ownerID int32, classIndex int) ([]shortcut.Shortcut, error)
	Save(ctx context.Context, ownerID int32, classIndex int, sc shortcut.Shortcut) error
	Delete(ctx context.Context, ownerID int32, classIndex int, slot, page int32) error
}

type hennaStore interface {
	ListByOwner(ctx context.Context, ownerID int32, classIndex int) ([]henna.Row, error)
	Insert(ctx context.Context, ownerID int32, classIndex, symbolID, slot int) error
	Delete(ctx context.Context, ownerID int32, classIndex, slot int) error
}

// recipeBookStore reads and writes the character_recipebook rows of one
// player's recipe book.
// subclassStore is the character_subclasses persistence a class change
// writes. Satisfied by *sql.SubclassStore.
type subclassStore interface {
	Insert(ctx context.Context, charID int32, sub player.SubClass) error
	Delete(ctx context.Context, charID int32, index int) error
}

type recipeBookStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]int, error)
	Insert(ctx context.Context, ownerID int32, recipeID int) error
	Delete(ctx context.Context, ownerID int32, recipeID int) error
}

// petStore is the narrow persistence surface a pet-collar restore needs:
// the saved row for a collar's pet, if any (data/sql.PetStore.Get).
type petStore interface {
	Get(ctx context.Context, itemObjectID int32) (petmodel.State, bool, error)
	Save(ctx context.Context, itemObjectID int32, state petmodel.State) error
	SaveFed(ctx context.Context, itemObjectID int32, fed int) error
}

// AttackStanceTracker owns combat-stance membership. It is exported so the
// behavior harness can substitute one without redeclaring the contract.
type AttackStanceTracker interface {
	Add(task.AttackStanceActor)
	Remove(task.AttackStanceActor) bool
	InAttackStance(task.AttackStanceActor) bool
}

type idAllocator interface {
	NextID() (int32, error)
}

type groundItemDropper interface {
	Drop(*grounditem.Item, task.DropOptions)
	Remove(*grounditem.Item)
}

const (
	crystallizeSkillID              = 248
	dropInteractionDistance         = 150
	groundPickupInteractionDistance = 36
)

// PlayerConfig bundles the primitive server/players.properties-derived
// gameplay flags GameClientLink needs, so its constructor doesn't grow one
// bool/float parameter per config key.
type PlayerConfig struct {
	// Enchant holds the scroll-of-enchant rates and limits; nil uses the
	// shipped defaults.
	Enchant               *enchantflow.Config
	WeightLimitMultiplier float64
	// InventorySlots is the base player inventory slot count by race.
	InventorySlots player.InventorySlots
	// StorageSlots is the base warehouse, freight, private store and recipe
	// book size.
	StorageSlots player.StorageSlots
	// Freight holds the freight service settings; nil uses the shipped
	// defaults.
	Freight *FreightConfig
	// AllowWater controls whether entering a water zone starts the
	// drowning breath-gauge countdown at all.
	AllowWater          bool
	EnableFallingDamage bool
	// RespawnRestoreHP is the fraction of calculated max HP a non-percent
	// revive restores.
	RespawnRestoreHP float64
	// DeathPenaltyChance is the percentage chance for a non-karma player to
	// receive a death-penalty level.
	DeathPenaltyChance int
	SpawnProtection    time.Duration
	// SkillEnchantSPBookNeeded controls whether enchanting a skill above
	// level 76 also consumes the tree's configured spellbook item.
	SkillEnchantSPBookNeeded bool
	// AutoLearnSkills grants every available class skill on a level refresh.
	AutoLearnSkills bool
	// KarmaPlayerCanTeleport controls whether a karma-carrying player may
	// use a TELEPORT/RECALL-type skill, direct or item-attached.
	KarmaPlayerCanTeleport bool
	// KarmaPlayerCanShop, KarmaPlayerCanUseGK and KarmaPlayerCanUseWareHouse
	// let a karma-carrying player use a shop, gatekeeper or warehouse NPC;
	// when one is off, that NPC answers with its refusal page if it has
	// one.
	KarmaPlayerCanShop         bool
	KarmaPlayerCanUseGK        bool
	KarmaPlayerCanUseWareHouse bool
	// KarmaPlayerCanTrade controls whether a trade may be requested while
	// either side carries karma.
	KarmaPlayerCanTrade bool
	AwardPKKillPVPPoint bool
	// PerfectShieldBlockRate is the roll threshold (out of 100) below which
	// a successful shield block upgrades to a perfect block.
	PerfectShieldBlockRate int
	// AllowDelevel controls whether a player death may cost experience/karma
	// at all.
	AllowDelevel bool
	// RateKarmaExpLost scales the death exp-loss percentage while the dying
	// player carries positive karma.
	RateKarmaExpLost float64
	// CharacterSelectDelay is the reuse delay shared by the character-list
	// actions (delete, restore, select) on one client session.
	CharacterSelectDelay time.Duration
	// ServerBypassDelay is the reuse delay between two bypass commands on
	// one client session.
	ServerBypassDelay time.Duration
	// MaxBuffsAmount is the base non-toggle, non-seven-signs buff-slot
	// count every character starts with. Known Divine Inspiration levels
	// add on top of this at MaxBuffCount time.
	MaxBuffsAmount int
	// MagicFailures makes magic-damage casts roll for a half or full resist.
	MagicFailures bool
	// DiscardItemDisabled is server.properties AllowDiscardItem inverted, so
	// the zero value lets players drop items as the shipped config does.
	DiscardItemDisabled bool
	// GMStartupUnlisted is players.properties GMStartupAutoList inverted, so
	// the zero value lists a game master that logs in, as the shipped config
	// does.
	GMStartupUnlisted bool
	// GMStartupInvulnerable, GMStartupInvisible and GMStartupBlockAll are
	// players.properties's same-named keys: a game master logs in
	// invulnerable, invisible, or blocking everything.
	GMStartupInvulnerable bool
	GMStartupInvisible    bool
	GMStartupBlockAll     bool
	// CraftingDisabled is players.properties CraftingEnabled inverted, so
	// the zero value keeps crafting on as the shipped config does.
	CraftingDisabled bool
	// ManufactureDelay is the reuse delay between two crafts on one client
	// session.
	ManufactureDelay time.Duration
	// MultisellDelay is the reuse delay between two multisell exchanges on
	// one client session.
	MultisellDelay time.Duration
	// RollDiceDelay is the reuse delay between two dice throws on one
	// client session.
	RollDiceDelay time.Duration
	// SubclassDelay is the reuse delay between two subclass add, change or
	// replace actions of one player.
	SubclassDelay time.Duration
	// SubclassWithoutQuests lets a subclass be added without the quests it
	// otherwise needs.
	SubclassWithoutQuests bool
	// KeepMaintainedIngredients is players.properties BlacksmithUseRecipes
	// inverted, so the zero value takes every ingredient as the shipped
	// config does.
	KeepMaintainedIngredients bool
	// PartyRange is how near the loot a party member must be to share it,
	// body to body; -1 is unlimited. It is the kill reward's party range.
	PartyRange int
}

// GameClientLink accepts and drives connections from Interlude game
// clients: the VersionCheck/cipher handshake, session-key validation
// against the login server, character list/create/delete/restore, and
// character select through to world entry.
type GameClientLink struct {
	validator *SessionValidator
	// clients is the process-owned account-to-connection registry: a second
	// AuthLogin for an account already claimed evicts the prior connection
	// instead of being rejected.
	clients       *ClientRegistry
	loginLink     func() *LoginLink
	roster        *manager.Roster
	items         itemStore
	shortcuts     shortcutStore
	hennas        hennaStore
	hennaTable    *henna.Table
	recipeBooks   recipeBookStore
	subclasses    subclassStore
	craft         *craft.Service
	merchant      *merchant.Service
	symbols       *symbolmaker.Service
	gatekeeper    *gatekeeper.Service
	exchange      *exchange.Service
	augment       *augment.Service
	templates     *player.TemplateTable
	itemTemplates *item.Table
	html          *datacache.HTML
	crests        *datacache.Crests
	skills        *skillstate.Persistence
	spellbooks    modelskill.BookPolicy
	skillTrees    *modelskill.Trees
	cursedWeapons *entity.CursedWeaponTable
	world         *world.State
	npcs          *npc.Table
	summonItems   *item.SummonItemTable
	doors         door.StateOwner
	petStore      petStore
	geo           move.Geo
	zones         *zone.Index
	ids           idAllocator
	groundItems   groundItemDropper
	attackStance  AttackStanceTracker
	ai            AIRegistry
	pvpFlags      *task.PvPFlags
	// decay removes a dead summon's corpse at its deadline; nil leaves
	// summon corpses in the world.
	decay       *task.Decay
	effects     effect.Env
	positions   *task.PositionUpdates
	playerClock *task.PlayerClock
	gameClock   *task.GameClock
	sevenSigns  *sevensigns.State
	olympiad    *olympiad.Olympiad
	raidPoints  *raidpoint.Points
	water       *task.Water
	shadowItems *task.ShadowItems
	autosave    *task.Autosave
	// inventoryUpdates batches InventoryUpdate packets for inventory
	// changes the server makes on its own, outside a client request.
	inventoryUpdates *task.InventoryUpdates
	// itemInstances lazily persists item rows whose live state changed,
	// so a mutation made outside a client request still reaches the
	// items table.
	itemInstances *task.ItemInstances
	// persist runs this link's database writes for detached players, pets
	// and containers on per-owner lanes.
	persist *persist.Worker
	// itemWrites orders the writes of one items row against each other, which
	// the lanes alone cannot: a row outlives its owner's lane (see
	// applyPersistActions).
	itemWrites       *persist.Order
	persistWait      time.Duration
	queues           Queues
	queuedPets       queuedPets
	restarts         *restart.Table
	levels           *player.LevelTable
	admin            *admin.Data
	playerConfig     PlayerConfig
	petConfig        petmodel.Config // passed into summon.PetConfig by newPet.
	disableRaidCurse bool
	inventory        *invops.Service
	petItems         *petitem.Service
	trades           *tradebook.Book
	parties          *partyRegistry
	rooms            *roomRegistry
	duels            *duelRegistry
	partyPositions   partyPositions
	clans            *clan.Service
	clanWarehouses   clanWarehouseBook
	enchantState     *enchantflow.State
	enchant          *enchantflow.Service
	targets          *skilltarget.Registry
	skillHandlers    *handlerskill.Registry
	chance           *actorcast.ChanceProcs
	log              zerolog.Logger

	// newCipherKey supplies each connection's XOR cipher key; overridden in
	// tests for a deterministic handshake.
	newCipherKey func() ([]byte, error)
	noCipher     bool

	// now supplies wall time for packet accounting; nil falls back to
	// time.Now at the call site. Overridden in tests for deterministic
	// flood windows.
	now func() time.Time

	// enchantRoll supplies enchant dice rolls; overridden in tests.
	enchantRoll func() float64

	// skillEnchantRoll supplies skill-enchant dice rolls in [0,99];
	// overridden in tests for a deterministic outcome.
	skillEnchantRoll func() int

	// fishing is the fish table and dice a cast line draws from; see
	// fishing.go.
	fishing fishingDeps

	// board is the community board; see community_board.go.
	board communityBoard

	// relations and characters back the friend and block lists; see
	// friends.go.
	relations  *relation.Manager
	characters characterDirectory
	// accessLevels stores the access levels admin commands change; nil
	// keeps them in memory only.
	accessLevels accessLevelStore
	// punishments stores the chat bans and jail terms; nil keeps them in
	// memory only. See punishment.go.
	punishments punishmentStore
	// macros persists each player's macro list; see macro.go.
	macros macroStore
	// recommendations persists who recommended whom and the counters; see
	// recommendation.go.
	recommendations recommendationStore
	// recommendGate orders recommendations against the daily refresh; see
	// RefreshDailyRecommendations.
	recommendGate sync.RWMutex
	// gms is the online game-master roster /gmlist reads and petitions
	// notify.
	gms admin.GMList[*livePlayer]
	// petitions holds the petitions players send the game masters; see
	// petition.go.
	petitions *petition.Manager
	// gmAudit records every admin command run; the zero logger records
	// nothing.
	gmAudit zerolog.Logger
	// chat is the chat settings: log, bot filter and reuse delays.
	chat ChatConfig
	// announcements are the server announcements: read at login, repeated
	// on their schedules and managed by //announce.
	announcements *announcement.Registry
	schemeBuffer  *schemebuffer.Manager
	lottery       *lottery.Lottery
	// npcSpawns is the live NPC population the admin spawn commands use;
	// see SetNpcSpawns.
	npcSpawns atomic.Pointer[manager.Npcs]
	// manor is what the seed and harvester items read.
	manor ManorConfig
}

// AIRegistry owns recurring actor-AI registrations.
type AIRegistry interface {
	Add(task.AIActor)
	Remove(task.AIActor)
}

// Queues creates the queue one live actor's work runs on; id names it in
// logs.
type Queues interface {
	NewQueue(id string) *sim.Queue
}

// GameClientLinkConfig contains the collaborators required by GameClientLink.
type GameClientLinkConfig struct {
	Validator *SessionValidator
	// NoCipher keeps game packets cleartext after VersionCheck.
	NoCipher    bool
	LoginLink   func() *LoginLink
	Roster      *manager.Roster
	Items       itemStore
	Shortcuts   shortcutStore
	Hennas      hennaStore
	HennaTable  *henna.Table
	RecipeBooks recipeBookStore
	// Subclasses writes the subclass rows a village master's subclass
	// commands add and replace.
	Subclasses subclassStore
	// Recipes is the loaded recipe table; nil loads none, so every recipe
	// request is dropped.
	Recipes  *recipe.Table
	Merchant *merchant.Service // nil: every buylist is unknown
	// Multisells is the loaded multisell table; nil loads none, so no list
	// opens.
	Multisells    *multisell.Table
	Templates     *player.TemplateTable
	ItemTemplates *item.Table
	HTML          *datacache.HTML
	Crests        *datacache.Crests
	Skills        *skillstate.Persistence
	Spellbooks    modelskill.BookPolicy
	SkillTrees    *modelskill.Trees
	// HealSps holds the spiritshot heal corrections; nil gives a charged
	// heal no correction term.
	HealSps       *modelskill.HealSpsTable
	CursedWeapons *entity.CursedWeaponTable
	World         *world.State
	NPCs          *npc.Table
	SummonItems   *item.SummonItemTable
	Doors         door.StateOwner
	PetStore      petStore
	Geo           move.Geo
	Zones         *zone.Index
	IDs           idAllocator
	GroundItems   groundItemDropper
	AttackStance  AttackStanceTracker
	AI            AIRegistry
	PvPFlags      *task.PvPFlags
	// Decay removes a dead summon's corpse once its decay delay has passed.
	Decay       *task.Decay
	Effects     effect.Env // Activity required: without it no effect expires
	Positions   *task.PositionUpdates
	PlayerClock *task.PlayerClock
	// GameClock is the server's in-game clock; CharSelected reports its
	// current minute of day. Nil is tolerated (tests) and reports 0.
	GameClock *task.GameClock
	// SevenSigns owns the event calendar; EnterWorld reports the active
	// period's system message. Nil is tolerated (tests) and sends nothing.
	SevenSigns  *sevensigns.State
	Water       *task.Water
	ShadowItems *task.ShadowItems
	Autosave    *task.Autosave
	// InventoryUpdates batches InventoryUpdate packets for inventory
	// changes the server makes on its own, outside a client request.
	InventoryUpdates *task.InventoryUpdates
	// ItemInstances lazily persists item rows whose live state changed.
	ItemInstances *task.ItemInstances
	// Persist runs detach, pet and container saves; nil writes inline.
	Persist *persist.Worker
	// ItemWrites orders one items row's writes; the item persistence task
	// shares it, since it writes the same rows.
	ItemWrites *persist.Order
	// PersistWait bounds how long a connection waits for queued saves before
	// reading rows back; zero means LivePlayerPersistWait.
	PersistWait time.Duration
	// Olympiad holds the nobles' Olympiad records. Nil is tolerated
	// (tests) and holds none.
	Olympiad *olympiad.Olympiad
	// RaidPoints holds the players' raid points. Nil is tolerated (tests):
	// kills credit none and every record reads empty.
	RaidPoints *raidpoint.Points
	// Queues creates each live player's queue, which its in-world packet
	// handlers, timers and periodic ticks run on. Required.
	Queues       Queues
	Restarts     *restart.Table
	Levels       *player.LevelTable
	Admin        *admin.Data
	PlayerConfig PlayerConfig
	PetConfig    petmodel.Config
	// DisableRaidCurse is npcs.properties DisableRaidCurse: when true, raid
	// petrification and anti-strider curses never apply.
	DisableRaidCurse bool
	// Teleports and InstantTeleports are the destinations civilian NPCs
	// offer; nil offers none. FreeTeleport is npcs.properties FreeTeleport:
	// when true, no destination is charged for. TeleportClock is the local
	// wall clock the weekend half-price hours are read from; nil means
	// time.Now.
	Teleports        travel.TeleportTable
	InstantTeleports travel.InstantTable
	FreeTeleport     bool
	TeleportClock    func() time.Time
	Log              zerolog.Logger
	// Now supplies the clock packet accounting uses to bucket received
	// frames into flood windows; nil means time.Now.
	Now func() time.Time
	// TradeClock times every pending request out (direct trade, party,
	// command channel, party room and friend invitations); nil means
	// time.Now.
	TradeClock func() time.Time
	// Relations holds the friend and block lists; nil starts with none and
	// keeps what changes in memory only.
	Relations *relation.Manager
	// Characters finds characters, online or not, by name and id for the
	// friend and block commands; nil finds none.
	Characters characterDirectory
	// AccessLevels stores the access levels admin commands change; nil
	// keeps them in memory only and finds no offline character.
	AccessLevels accessLevelStore
	// Punishments stores the chat bans and jail terms; nil keeps them in
	// memory only and finds no offline character.
	Punishments punishmentStore
	// Petitions holds the petitions players send the game masters; nil
	// starts with none and refuses every new one.
	Petitions *petition.Manager
	// EnchantRoll supplies enchant dice rolls in [0,1); nil falls back to
	// the random source. Behavior harnesses inject a deterministic roll.
	EnchantRoll func() float64
	// SkillEnchantRoll supplies skill-enchant dice rolls in [0,99]; nil
	// falls back to the random source. Behavior harnesses inject a
	// deterministic roll.
	SkillEnchantRoll func() int
	// CraftRoll supplies craft success rolls in [0,n); nil falls back to
	// the random source.
	CraftRoll func(n int) int
	// Fish is the loaded fish table a cast line draws its fish from; nil
	// draws none, so every line comes back empty.
	Fish *fish.Table
	// FishingRoll supplies the fishing dice in [0,n); nil falls back to the
	// random source.
	FishingRoll func(n int) int
	// Augmentations is the loaded augmentation data a refine rolls from;
	// nil refuses every refine as unsuitable.
	Augmentations *augmentation.Table
	// AugmentationChances are the players.properties augmentation skill,
	// glow and base stat chances.
	AugmentationChances augmentation.Chances
	// AugmentRoll draws the uniform ints a refine rolls, both ends
	// inclusive; nil falls back to the random source.
	AugmentRoll augmentation.Rand
	// ArmorSets is the loaded armor set data whose +6 skill an armor
	// enchant grants and revokes; nil grants none. The equip-time set
	// skills come from the skill persistence's own table.
	ArmorSets *armorset.Table
	// Manor is what the seed and harvester items and the harvest read; the
	// zero value turns the manor off.
	Manor ManorConfig
	// Macros persists each player's macro list; nil keeps macros in memory
	// only.
	Macros macroStore
	// Recommendations persists who recommended whom and the counters; nil
	// keeps them in memory only.
	Recommendations recommendationStore
	// GMAudit records every admin command run (server.properties GMAudit);
	// the zero logger records nothing.
	GMAudit zerolog.Logger
	// Chat is the chat settings of server.properties; the zero value logs
	// nothing and rate-limits nothing.
	Chat ChatConfig
	// Clans is the clan registry and its rules; nil runs with no clan at
	// all and nothing written.
	Clans *clan.Service
	// Board is the community board's settings; the zero value keeps the
	// board off.
	Board bbs.Config
	// Mailbox holds the board's mail; nil, as while the board is off,
	// holds none.
	Mailbox *bbs.Mailbox
	// Forums holds the board's forums, Favorites its favorites; nil holds
	// them in memory only, starting with none.
	Forums    *bbs.Forums
	Favorites *bbs.Favorites
	// ShowServerNews shows the server news page at login when no clan
	// notice is shown.
	ShowServerNews bool
	// Announcements are the server announcements; nil holds them in
	// memory only, starting with none.
	Announcements *announcement.Registry
	// SchemeBuffer is the scheme buffer's buffs and every player's
	// schemes; nil offers no buff and starts with no scheme.
	SchemeBuffer *schemebuffer.Manager
	// Lottery is the Lucky Lottery; nil runs no round.
	Lottery *lottery.Lottery
}

// NewGameClientLink builds a GameClientLink from its collaborators.
// loginLink returns the game server's current link to the login server, or
// nil while disconnected/reconnecting: session validation fails clients
// gracefully (AuthLoginFail) rather than panicking while the link is down.
func NewGameClientLink(cfg GameClientLinkConfig) (*GameClientLink, error) {
	if cfg.Queues == nil {
		return nil, errors.New("network: GameClientLinkConfig.Queues is required")
	}
	if cfg.Effects.Activity == nil {
		return nil, errors.New("network: GameClientLinkConfig.Effects.Activity is required")
	}
	link := &GameClientLink{
		validator:     cfg.Validator,
		clients:       NewClientRegistry(),
		loginLink:     cfg.LoginLink,
		roster:        cfg.Roster,
		items:         cfg.Items,
		shortcuts:     cfg.Shortcuts,
		hennas:        cfg.Hennas,
		hennaTable:    cfg.HennaTable,
		recipeBooks:   cfg.RecipeBooks,
		subclasses:    cfg.Subclasses,
		merchant:      cfg.Merchant,
		templates:     cfg.Templates,
		itemTemplates: cfg.ItemTemplates,
		html:          cfg.HTML,
		crests:        cfg.Crests,
		skills:        cfg.Skills,
		spellbooks:    cfg.Spellbooks,
		skillTrees:    cfg.SkillTrees,
		cursedWeapons: cfg.CursedWeapons,
		world:         cfg.World,
		npcs:          cfg.NPCs,
		summonItems:   cfg.SummonItems,
		doors:         cfg.Doors,
		petStore:      cfg.PetStore,
		geo:           cfg.Geo,
		zones:         cfg.Zones,
		ids:           cfg.IDs,
		groundItems:   cfg.GroundItems,
		attackStance:  cfg.AttackStance,
		ai:            cfg.AI,
		pvpFlags:      cfg.PvPFlags,
		decay:         cfg.Decay,
		effects:       cfg.Effects,
		positions:     cfg.Positions,
		playerClock:   cfg.PlayerClock,
		gameClock:     cfg.GameClock,
		sevenSigns:    cfg.SevenSigns,
		olympiad:      cfg.Olympiad,
		raidPoints:    cfg.RaidPoints,
		water:         cfg.Water,
		shadowItems:   cfg.ShadowItems,
		autosave:      cfg.Autosave,

		inventoryUpdates: cfg.InventoryUpdates,
		itemInstances:    cfg.ItemInstances,
		persist:          cfg.Persist,
		itemWrites:       cfg.ItemWrites,
		persistWait:      cfg.PersistWait,
		queues:           cfg.Queues,
		restarts:         cfg.Restarts,
		levels:           cfg.Levels,
		admin:            cfg.Admin,
		gmAudit:          cfg.GMAudit,
		chat:             cfg.Chat,
		playerConfig:     cfg.PlayerConfig,
		petConfig:        cfg.PetConfig,
		disableRaidCurse: cfg.DisableRaidCurse,
		enchantRoll:      cfg.EnchantRoll,
		skillEnchantRoll: cfg.SkillEnchantRoll,
		fishing:          fishingDeps{table: cfg.Fish, roll: cfg.FishingRoll},
		inventory:        invops.NewService(cfg.IDs),
		petItems:         petitem.NewService(cfg.IDs),
		trades:           tradebook.NewBook(cfg.TradeClock),
		parties:          party.NewRegistry[*livePlayer](cfg.TradeClock),
		rooms:            partymatch.NewRegistry[*livePlayer](),
		duels:            duel.NewManager[*livePlayer](),
		relations:        cmp.Or(cfg.Relations, relation.NewManager(nil)),
		characters:       cfg.Characters,
		accessLevels:     cfg.AccessLevels,
		punishments:      cfg.Punishments,
		petitions:        cmp.Or(cfg.Petitions, petition.NewManager(petition.DefaultConfig(), nil, nil, nil, nil)),
		enchantState:     enchantflow.NewState(),
		targets:          skilltarget.NewRegistry(skilltarget.WorldKnown{State: cfg.World}),
		skillHandlers: handlerskill.NewDefaultRegistryWithSignet(cfg.Skills, cfg.PlayerConfig.MagicFailures, cfg.HealSps, handlerskill.SignetDeps{
			Templates: cfg.NPCs,
			IDs:       cfg.IDs,
			World:     cfg.World,
			NewSink:   EffectPointSinks(cfg.World),
			Effects:   cfg.Effects,
			Queues:    cfg.Queues,
			Log:       cfg.Log,

			ManorCropRate: cfg.Manor.CropRate,
		}),
		log:          cfg.Log,
		now:          cfg.Now,
		newCipherKey: randomCipherKey,
		noCipher:     cfg.NoCipher,
	}
	link.macros = cfg.Macros
	link.manor = cfg.Manor
	link.recommendations = cfg.Recommendations
	// Built here, not lazily: every client goroutine shares this link.
	enchantCfg := enchantflow.DefaultConfig()
	if cfg.PlayerConfig.Enchant != nil {
		enchantCfg = *cfg.PlayerConfig.Enchant
	}
	link.enchant = enchantflow.NewService(link.enchantState, link.ids, link.rollEnchant, enchantCfg)
	link.enchant.SetArmorSets(cfg.ArmorSets)
	link.symbols = symbolmaker.NewService(cfg.HennaTable, link.nextObjectID)
	link.gatekeeper = gatekeeper.NewService(cfg.Teleports, cfg.InstantTeleports, cfg.FreeTeleport, cfg.TeleportClock)
	link.craft = craft.NewService(cfg.Recipes, !cfg.PlayerConfig.CraftingDisabled, link.nextObjectID, cfg.CraftRoll)
	link.augment = newAugmentService(cfg)
	link.board = communityBoard{cfg: cfg.Board, mail: cfg.Mailbox, forums: cfg.Forums, favorites: cfg.Favorites, serverNews: cfg.ShowServerNews}
	if link.board.forums == nil {
		link.board.forums = bbs.NewForums(nil, nil, cfg.Log)
	}
	if link.board.favorites == nil {
		link.board.favorites = bbs.NewFavorites(nil, nil, cfg.Log)
	}
	link.schemeBuffer = cfg.SchemeBuffer
	if link.schemeBuffer == nil {
		link.schemeBuffer = schemebuffer.New(schemebuffer.DefaultConfig(), nil, nil)
	}
	link.lottery = cfg.Lottery
	if link.lottery == nil {
		link.lottery = lottery.New(lottery.DefaultConfig(), nil, nil, nil, cfg.Queues.NewQueue("lottery"), cfg.Log)
	}
	link.announcements = cfg.Announcements
	if link.announcements == nil {
		link.announcements = announcement.NewRegistry(nil, NewAnnouncer(cfg.World), cfg.Log, cfg.Queues.NewQueue("announcements"))
	}
	link.clans = cfg.Clans
	if link.clans == nil {
		link.clans = clan.NewService(nil, nil, nil, cfg.IDs, clan.DefaultConfig(), nil, cfg.Log)
	}
	link.exchange = exchange.NewService(cfg.Multisells, cfg.PlayerConfig.KeepMaintainedIngredients, link.nextObjectID, link.clans)
	if link.roster != nil {
		clans := link.clans
		link.roster.SetPurged(func(objectID int32) { clans.RemoveDeleted(objectID, time.Now()) })
	}
	link.chance = &actorcast.ChanceProcs{Definitions: link.skills, Targets: link.targets, Skills: link.skillHandlers, Deliver: link.deliverChanceCast}
	if link.zones != nil {
		for _, boss := range zone.OfKind[*zone.Boss](link.zones) {
			boss.Eject = func(a zone.Actor) { link.ejectBossPlayer(boss, a) }
		}
	}
	link.wireZoneOccupantHooks()
	debugStop := func(reason string, at location.Location) {
		link.log.Debug().Str("reason", reason).Interface("at", at).Str("stack", string(debug.Stack())).Msg("movedbg: player walk stopped")
	}
	debugResolve := func(origin, target, destination location.Location, waypoints, outcome int, distance, ticks, speed float64) {
		link.log.Debug().Interface("origin", origin).Interface("target", target).Interface("destination", destination).Int("waypoints", waypoints).Int("outcome", outcome).Float64("distance", distance).Float64("ticks", ticks).Float64("speed", speed).Msg("movedbg: walk resolved")
	}
	move.DebugStop.Store(&debugStop)
	move.DebugResolve.Store(&debugResolve)
	return link, nil
}

// newPet builds a pet with the shared effect, config and skill wiring. A live
// pet inventory must be constructed with
// itemcontainer.NewPetInventoryWithDelivery (delivery and persistence) before
// calling this method; NewPetInventory has neither.
func (l *GameClientLink) newPet(cfg summon.PetConfig) (*summon.Actor, error) {
	cfg.Effects = l.effects
	cfg.Config = &l.petConfig
	cfg.MaxBuffsAmount = l.playerConfig.MaxBuffsAmount
	cfg.RespawnRestoreHP = l.playerConfig.RespawnRestoreHP
	if cfg.SkillDefs == nil {
		cfg.SkillDefs = l.skills
	}
	pet, err := summon.NewPet(cfg)
	if err != nil {
		return nil, err
	}
	pet.SetRaidCursesDisabled(l.disableRaidCurse)
	return pet, nil
}

// nextObjectID allocates a new world object id.
func (l *GameClientLink) nextObjectID() (int32, error) {
	if l.ids == nil {
		return 0, errors.New("network: no object id allocator")
	}
	return l.ids.NextID()
}

func (l *GameClientLink) rollEnchantSkill() int {
	if l.skillEnchantRoll != nil {
		return l.skillEnchantRoll()
	}
	return rnd.Get(100)
}

func randomCipherKey() ([]byte, error) {
	key := make([]byte, gamecipher.KeySize)
	if _, err := rand.Read(key[:8]); err != nil {
		return nil, fmt.Errorf("generate game cipher key: %w", err)
	}
	copy(key[8:], gamecipher.StaticKey[:])
	return key, nil
}

func validProtocolRevision(revision int32) bool {
	switch revision {
	case 737, 740, 744, 746:
		return true
	default:
		return false
	}
}
