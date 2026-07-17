package player

import (
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// defaultAccessLevel is the access level a freshly created character starts
// at, matching the shipped server default.
const defaultAccessLevel = 0

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

	Level int
	Exp   int64
	SP    int

	MaxHP, CurHP float64
	MaxCP, CurCP float64
	MaxMP, CurMP float64

	Face, HairStyle, HairColor int

	// Location and LastHeading are the character's last known world
	// location. The field is named LastHeading, not Heading, so it doesn't
	// shadow the Heading() method promoted from the embedded world.Presence.
	Location    location.Location
	LastHeading int

	Karma             int
	PvPKills, PKKills int

	ClanID      int
	Title       string
	AccessLevel int

	// DeleteAt is the persisted deletion deadline, in epoch milliseconds;
	// zero means the character is not scheduled for deletion.
	DeleteAt   int64
	LastAccess int64

	runtimeTemplate *Template
	inventory       *itemcontainer.Inventory
	world           *world.State
	sendFrame       func(wire.Frame) bool
	broadcastAttack func(attack.Snapshot)
	broadcastMove   func(move.Event)
	broadcastStop   func()
	roll            func(int) int

	deathMu sync.Mutex
	dead    bool
	health  creature.Health

	effects *effect.List

	// statMu guards statFuncs.
	statMu    sync.Mutex
	statFuncs []basefunc.Func

	// stateMu guards transient live flags.
	stateMu       sync.RWMutex
	stateInit     bool
	running       bool
	standing      bool
	inCombat      bool
	autoSoulShots map[int32]bool

	skills skillState
}

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

		Level: 1,

		MaxHP: tmpl.HPTable[0], CurHP: tmpl.HPTable[0],
		MaxCP: tmpl.CPTable[0], CurCP: tmpl.CPTable[0],
		MaxMP: tmpl.MPTable[0], CurMP: tmpl.MPTable[0],

		Face: int(face), HairStyle: int(hairStyle), HairColor: int(hairColor),

		AccessLevel: defaultAccessLevel,

		stateInit: true,
		running:   true,
		standing:  true,
	}

	if len(tmpl.Spawns) > 0 {
		c.Location = tmpl.Spawns[rand.IntN(len(tmpl.Spawns))]
	}
	c.health = creature.NewHealth(&c.CurHP)
	c.effects = effect.NewList(c)

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
	return c.LastHeading
}
