package script

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// DropRules answers the drop settings in force, which //reload config may
// change between two reads.
type DropRules interface {
	// PartyRange is the distance a player must stand strictly within, from
	// the NPC, to pass a quest-state check.
	PartyRange() int
	// MultipleItemDrop reports whether a non-stackable item counted above
	// one is given as that many instances rather than as one.
	MultipleItemDrop() bool
}

// partyRange is the party range in force.
func (e *Env) partyRange() int {
	if e.Drops == nil {
		return 0
	}
	return e.Drops.PartyRange()
}

// multipleItemDrop reports whether the multiple item drop rule is in force.
func (e *Env) multipleItemDrop() bool {
	return e.Drops != nil && e.Drops.MultipleItemDrop()
}

// Env is the server state the script helpers act through. Build hands the
// same Env to every registered script; nothing changes it afterwards.
type Env struct {
	// Quests writes the quest journals whose states the checks return.
	Quests *Quests
	// Rates scale quest drops and rewards.
	Rates Rates
	// Drops answers the party range and the multiple item drop rule in
	// force, read at each use; nil reads a zero range and gives a
	// non-stackable counted above one as one instance.
	Drops DropRules
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
	// Now returns the local wall-clock time, from the clock the scripts'
	// timers run on.
	Now func() time.Time
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
	// Halls are the clan halls, with the siegable halls' sieges; nil finds
	// none.
	Halls *clanhall.Halls
	// Clans are the clans a script looks up by id; nil finds none.
	Clans Clans
	// Zones are the zones of the world a script looks up; nil finds none.
	Zones *zone.Index
}

// Doors finds the doors of the world by id or name and opens and closes
// them.
type Doors interface {
	// Door returns the door whose id is id.
	Door(id int) (*door.Object, bool)
	// DoorByName returns the door named name, ignoring case.
	DoorByName(name string) (*door.Object, bool)
	// SetDoorOpen opens or closes the door id, as every state change of a
	// door does.
	SetDoorOpen(id int, open bool) bool
	// WatchDoor has fn called with the standing door id after each later
	// change of its open state: the script watchers, then the maker
	// watchers, each in registration order; a key registered again under
	// kind moves last. It reports whether the door stands in the world.
	WatchDoor(id int, kind door.WatchKind, key any, fn func(*door.Object)) bool
	// SetClanCall installs c as the call a hit on any door, standing now
	// or built by a later reload, makes to the NPCs around it.
	SetClanCall(c door.ClanCall)
}

// Clans finds a clan by id.
type Clans interface {
	Get(id int32) (*clan.Clan, bool)
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

// Now returns the local wall-clock time, read from the clock the script's
// timers run on.
func (s *Script) Now() time.Time { return s.env.Now() }

// Spawner returns the spawner the script places NPCs in the world with.
func (s *Script) Spawner() *Spawner { return s.env.Spawner }

// BossWaitTimes returns the grand boss wait times.
func (s *Script) BossWaitTimes() BossWaitTimes { return s.env.BossWaits }
