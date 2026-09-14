package player

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// defaultAccessLevel is the access level a freshly created character starts
// at, matching the shipped server default.
const defaultAccessLevel = 0

var _ world.Player = (*Character)(nil)

// Character is one persisted characters-table row plus the runtime state
// needed once that row enters the live world.
type Character struct {
	world.Presence
	world.RelocateScratch
	*creature.Live

	ID          int32
	AccountName string
	Name        string

	ClassID     int
	BaseClassID int
	Race        Race
	Sex         Sex

	// CharLevel is the persisted level. The field is named CharLevel, not
	// Level, so it doesn't collide with the Level() method the cast/target
	// handlers need (Go disallows a field and method sharing one name on
	// the same type) — same class of naming fix as LastHeading below.
	CharLevel int
	Exp       int64
	SP        int

	// ExpBeforeDeath is the persisted exp snapshot taken at the last death,
	// before the death's exp loss was applied (Player.java:2919). A
	// resurrection effect restores a percentage of the exp lost since then
	// via RestoreExp, which also clears this back to 0.
	ExpBeforeDeath int64
	// progressionMu guards CharLevel, Exp, SP and ExpBeforeDeath. Kill
	// rewards and death exp loss mutate them from task/timer goroutines
	// while disconnect/autosave snapshots them through ProgressionValues
	// (#1890).
	progressionMu sync.RWMutex

	maxHP, curHP float64
	maxCP, curCP float64
	maxMP, curMP float64
	// vitalsMu guards maxHP/curHP, maxCP/curCP and maxMP/curMP.
	vitalsMu sync.RWMutex

	Face, HairStyle, HairColor int

	// Location and LastHeading are the character's last known world
	// location. The field is named LastHeading, not Heading, so it doesn't
	// shadow the Heading() method promoted from the embedded world.Presence.
	// locMu guards both fields once the character is live: the
	// position-update ticker (SyncPosition, during an attack chase) and the
	// owning connection's network goroutine (SetLastKnownPosition, during
	// client-reported movement) write them from different goroutines.
	Location    location.Location
	LastHeading int
	locMu       sync.RWMutex

	// KarmaPoints is the persisted karma value. The field is named
	// KarmaPoints, not Karma, so it doesn't collide with the Karma() method
	// cross-package target-validity checks need — same naming fix as
	// CharLevel/LastHeading above.
	KarmaPoints       int
	PvPKills, PKKills int

	ClanID      int
	Title       string
	AccessLevel int

	// DeleteAt is the persisted deletion deadline, in epoch milliseconds;
	// zero means the character is not scheduled for deletion.
	DeleteAt   int64
	LastAccess int64

	// onlineMu guards the session playtime clock below. The clock starts
	// when the row is restored and every save persists the accumulated
	// total, so both run from goroutines that never otherwise meet.
	onlineMu       sync.Mutex
	onlineTimeBase int64
	onlineBegin    time.Time

	runtimeTemplate          *Template
	levelTable               *LevelTable
	allowDelevel             bool
	raidCursesDisabled       bool
	skillDefs                skillDefinitions
	rateKarmaExpLost         float64
	inventory                *itemcontainer.Inventory
	world                    *world.State
	los                      LineOfSight
	zones                    PeaceZoneQuery
	insidePvPZone            atomic.Bool
	insidePeaceZone          atomic.Bool
	insideSiegeZone          atomic.Bool
	insideNoSummonFriendZone atomic.Bool
	abnormalEffectMask       atomic.Int32
	weightPenalty            int
	weightLimitMultiplier    float64
	maxBuffsAmount           int
	awardPKKillPVPPoint      bool
	roll                     func(int) int
	floatRoll                func(float64) float64

	dead atomic.Bool

	// cast is the network-owned live cast controller wired back onto this
	// character so effect hooks (mute, silence, abort-cast, damage-break)
	// can reach it without this domain package importing the cast package
	// that already imports this one.
	cast atomic.Pointer[CastController]

	// sink receives this character's events. Attach sets it once, before
	// the character is published into the world, and it never changes
	// afterwards; nil drops every event (domain tests need no network).
	sink event.Sink
	// sessionDetached is set once the owning session has let go of this
	// character; see DetachSession.
	sessionDetached atomic.Bool

	// summonFriendMu guards the pending SUMMON_FRIEND/SUMMON_PARTY
	// teleport-confirm request state,
	// matching Player._summonTargetRequest/_summonSkillRequest
	// (Player.java:452-453).
	summonFriendMu    sync.Mutex
	summonRequester   any
	summonRequesterID int32
	summonSkill       modelskill.Definition

	// statMu guards statCalcs slot creation; each slot's own Calculator
	// then guards its own Mods independently, so a warm read only ever
	// takes statMu's read lock.
	statMu    sync.RWMutex
	statCalcs [stat.Count]*effect.Calculator

	// stateMu guards transient live flags and item-use disabled timestamps.
	stateMu              sync.RWMutex
	stateInit            bool
	running              bool
	standing             bool
	inCombat             bool
	autoSoulShots        map[int32]bool
	flying               bool
	mountType            int32
	mountNPCID           int32
	mountObjectID        int32
	transformed          bool
	spawnProtected       bool
	damagePermissionSet  bool
	canGiveDamage        bool
	operating            bool
	fishing              bool
	hero                 bool
	disabledItems        map[int32]time.Time
	shortBuffTaskSkillID int32
	weaponGradePenalty   bool
	armorGradePenalty    int
	shortBuffTimer       *time.Timer
	recentFakeDeathUntil time.Time
	groundTarget         location.Location
	castCtrl             bool
	castShift            bool
	target               world.Tracked
	log                  zerolog.Logger

	// pvpFlag is the client-visible PvP flag state; see UpdatePvPFlag.
	pvpFlag task.PvPFlagState

	// charges is the Force/Soul charge counter (increaseCharges/
	// decreaseCharges/clearCharges), auto-cleared by chargeTimer after
	// chargeAutoClearDelay of inactivity.
	charges     int
	chargeTimer *time.Timer

	// deathPenaltyLevel is the persisted death-penalty debuff level (skill
	// 5076), capped at maxDeathPenaltyLevel.
	deathPenaltyLevel  int
	deathPenaltyChance int

	// perfectShieldBlockRate is the players.properties-configured
	// PerfectShieldBlockRate roll threshold for a shield block to upgrade
	// to a perfect block (Formulas.java:859).
	perfectShieldBlockRate int

	skills skillState
	cubics cubic.List
	hennas *henna.List
}

var _ effect.StatOwner = (*Character)(nil)

// NewCharacter builds a freshly created Character of profession tmpl for
// accountName, seeded with the profession's level-1 base stats and a
// random spawn point from its template. name, hairStyle, hairColor, face
// and sex are the client-supplied appearance fields; the caller is
// responsible for validating them (name charset/length, hair/face bounds)
// before calling this, since those are wire-format concerns, not modeling
// ones.
//
// objectID must already be allocated by the caller (character creation
// needs the id before the row is inserted, to grant items owned by it).
func NewCharacter(objectID int32, tmpl *Template, accountName, name string, hairStyle, hairColor, face byte, sex Sex) (*Character, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("player: new character: nil template")
	}
	race, ok := ClassRace(tmpl.ID)
	if !ok {
		return nil, fmt.Errorf("player: new character: class %d has no known race", tmpl.ID)
	}
	if len(tmpl.HPTable) == 0 || len(tmpl.MPTable) == 0 || len(tmpl.CPTable) == 0 {
		return nil, fmt.Errorf("player: new character: class %d template has no level tables", tmpl.ID)
	}

	c := &Character{
		ID:          objectID,
		AccountName: accountName,
		Name:        name,

		ClassID:     tmpl.ID,
		BaseClassID: tmpl.ID,
		Race:        race,
		Sex:         sex,

		CharLevel: 1,

		// The stored max fields are raw calculator bases — the level-table
		// values, exactly what AddLevel's refill persists. The stat finalize
		// applies the CON/MEN bonus once on read, so seeding the bonus here
		// as well would double it for every freshly created character. The
		// current values instead start at the computed effective maximum
		// (HP and MP full, CP empty).
		maxHP: tmpl.HPTable[0], curHP: float64(int(tmpl.HPTable[0] * statbonus.CONBonus[tmpl.CON])),
		maxCP: tmpl.CPTable[0],
		maxMP: tmpl.MPTable[0], curMP: float64(int(tmpl.MPTable[0] * statbonus.MENBonus[tmpl.MEN])),

		Face: int(face), HairStyle: int(hairStyle), HairColor: int(hairColor),

		AccessLevel: defaultAccessLevel,

		stateInit:      true,
		running:        true,
		standing:       true,
		maxBuffsAmount: defaultMaxBuffsAmount,
	}

	if len(tmpl.Spawns) > 0 {
		c.Location = tmpl.Spawns[rand.IntN(len(tmpl.Spawns))]
	}

	return c, nil
}

// CurrentLocation returns the synchronized live world position when c is
// spawned, otherwise the persisted last-known location.
func (c *Character) CurrentLocation() location.Location {
	x, y, z := c.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// CurrentHeading returns the synchronized live heading when c is spawned,
// otherwise the persisted last-known heading.
func (c *Character) CurrentHeading() int {
	if c.Visible() {
		return c.Presence.Heading()
	}
	c.locMu.RLock()
	defer c.locMu.RUnlock()
	return c.LastHeading
}

// SetOnlineTime seeds the session playtime clock with seconds already
// accumulated by earlier sessions; the in-session elapsed time is measured
// from now, so the first save after this call persists base plus elapsed.
func (c *Character) SetOnlineTime(seconds int64, now time.Time) {
	c.onlineMu.Lock()
	defer c.onlineMu.Unlock()
	c.onlineTimeBase = seconds
	c.onlineBegin = now
}

// TotalOnlineTime returns the character's lifetime playtime in seconds:
// the base restored from the characters row plus everything accrued since
// the clock started.
func (c *Character) TotalOnlineTime(now time.Time) int64 {
	c.onlineMu.Lock()
	defer c.onlineMu.Unlock()
	total := c.onlineTimeBase
	if !c.onlineBegin.IsZero() && now.After(c.onlineBegin) {
		total += int64(now.Sub(c.onlineBegin) / time.Second)
	}
	return total
}
