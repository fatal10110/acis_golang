package script

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Env is the server state the script helpers act through. Build hands the
// same Env to every registered script; nothing changes it afterwards.
type Env struct {
	// Quests writes the quest journals whose states the checks return.
	Quests *Quests
	// Rates scale quest drops and rewards.
	Rates Rates
	// PartyRange is the distance a player must stand strictly within, from
	// the NPC, to pass a quest-state check.
	PartyRange int
	// MultipleItemDrop gives a non-stackable item counted above one as that
	// many instances; unset, as one.
	MultipleItemDrop bool
	// NewItemID allocates an item instance's object id.
	NewItemID func() (int32, error)
	// Spawner spawns the NPCs scripts place in the world.
	Spawner *Spawner
	// Rand returns a uniform random int in [0, n). It panics when n is not
	// positive. Every random draw of a script and its helpers is made
	// through it.
	Rand func(n int) int
	// World is the world a script looks NPCs up in; nil finds none.
	World *world.State
	// Skills are the skill definitions a script reads a skill's power,
	// aggro points and effects from.
	Skills *skill.Table
	// GameHour returns the in-game hour, 0 to 23.
	GameHour func() int
	// Uptime returns the real time elapsed since the server started.
	Uptime func() time.Duration
	// Items are the item templates a script reads an item's name from.
	Items *item.Table
	// Page returns the datapack page at path, relative to the server root
	// as a script names it ("./data/html/..."); false when there is none.
	// nil finds none.
	Page func(path string) (string, bool)
	// SevenSigns is the Seven Signs state a script reads the players'
	// cabals and the standings from.
	SevenSigns *sevensigns.State
	// BossWaits are the grand boss wait times.
	BossWaits BossWaitTimes
	// Doors are the doors a script opens and closes by name; nil finds
	// none.
	Doors Doors
	// Castles are the castles an NPC belongs to; nil finds none.
	Castles Castles
	// SiegableHall reports whether a residence id names a siegable clan
	// hall; nil names none.
	SiegableHall func(id int) bool
	// Sieges are the castles' sieges; nil finds none.
	Sieges Sieges
}

// Doors finds the doors of the world by name and opens and closes them.
type Doors interface {
	// DoorByName returns the door named name, ignoring case.
	DoorByName(name string) (*door.Object, bool)
	// SetDoorOpen opens or closes the door id, as every state change of a
	// door does.
	SetDoorOpen(id int, open bool) bool
}

// Castles finds the castle an NPC belongs to.
type Castles interface {
	// ForNPC returns the castle the NPC objectID, of template npcID and
	// placed by maker (nil for none), belongs to: spawnCastle when a siege
	// gave it that castle as it spawned (0 for none), else the castle that
	// placed it as a holy artifact, else the one the maker's spawn time
	// names, none when that names a siegable clan hall, else the castle
	// whose residence NPCs include npcID.
	ForNPC(objectID int32, npcID, spawnCastle int, maker *spawn.Maker, siegableHall func(id int) bool) (*castle.Castle, bool)
}

// Sieges finds a castle's siege.
type Sieges interface {
	// Get returns the siege of the castle castleID.
	Get(castleID int) (*siege.Siege, bool)
}

// BossWaitTimes are how long a grand boss lair waits, once it is entered,
// before its boss appears: npcs.properties AntharasWaitTime,
// ValakasWaitTime and FrintezzaWaitTime.
type BossWaitTimes struct {
	Antharas, Valakas, Frintezza time.Duration
}

// DefaultBossWaitTimes are the wait times npcs.properties gives when it
// sets none: 30, 20 and 10 minutes.
func DefaultBossWaitTimes() BossWaitTimes {
	return BossWaitTimes{Antharas: 30 * time.Minute, Valakas: 20 * time.Minute, Frintezza: 10 * time.Minute}
}

// Rates are the quest drop and reward multipliers.
type Rates struct {
	// Drop scales quest drops, per drop type.
	Drop float64
	// Reward scales items rewarded, Adena scales adena rewarded.
	Reward, RewardAdena float64
	// XP and SP scale rewarded experience and skill points.
	XP, SP float64
}

// character resolves p to the character the world tracks. A handle on
// nothing panics, so the script's invocation aborts there.
func (p *Player) character() *player.Character {
	return p.self.(player.CharacterHolder).PlayerCharacter()
}

// Rnd returns a uniform random int in [0, n); it panics when n is not
// positive.
func (s *Script) Rnd(n int) int { return s.env.Rand(n) }

// RndBool returns true or false with even odds.
func (s *Script) RndBool() bool { return s.env.Rand(2) == 0 }

// Spawner returns the spawner the script places NPCs in the world with.
func (s *Script) Spawner() *Spawner { return s.env.Spawner }

// BossWaitTimes returns the grand boss wait times.
func (s *Script) BossWaitTimes() BossWaitTimes { return s.env.BossWaits }
