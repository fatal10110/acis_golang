package player

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// defaultAccessLevel is the access level a freshly created character starts
// at, matching the shipped server default.
const defaultAccessLevel = 0

var _ world.Player = (*Character)(nil)

// Character is one persisted characters-table row plus the runtime state
// needed once that row enters the live world.
type Character struct {
	world.Presence
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

	runtimeTemplate    *Template
	inventory          *itemcontainer.Inventory
	world              *world.State
	los                LineOfSight
	sendFrame          func(wire.Frame) bool
	broadcastAttack    func(attack.Snapshot)
	broadcastMove      func(move.Event)
	broadcastStop      func()
	broadcastDie       func()
	broadcastStatus    func()
	broadcastShortBuff func(ShortBuffUpdate)
	roll               func(int) int

	deathMu sync.Mutex
	dead    bool

	// statMu guards statCalcs map creation. Each Calculator owns its Funcs.
	statMu    sync.Mutex
	statCalcs map[stat.Stat]*basefunc.Calculator

	// stateMu guards transient live flags and runtime send/broadcast hooks.
	stateMu              sync.RWMutex
	stateInit            bool
	running              bool
	standing             bool
	inCombat             bool
	autoSoulShots        map[int32]bool
	shortBuffTaskSkillID int32
	shortBuffTimer       *time.Timer

	skills skillState
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

		maxHP: tmpl.HPTable[0], curHP: tmpl.HPTable[0],
		maxCP: tmpl.CPTable[0], curCP: tmpl.CPTable[0],
		maxMP: tmpl.MPTable[0], curMP: tmpl.MPTable[0],

		Face: int(face), HairStyle: int(hairStyle), HairColor: int(hairColor),

		AccessLevel: defaultAccessLevel,

		stateInit: true,
		running:   true,
		standing:  true,
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
