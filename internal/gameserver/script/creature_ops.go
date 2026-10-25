package script

import (
	"fmt"
	"math"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// The creature facts every handle answers, and the jumps and aborts a
// behavior makes an NPC or a player do. Each of them panics on a handle on
// nothing, so the invocation aborts there.

// conditionHolder is a creature whose condition view gives its HP ratio and
// facing.
type conditionHolder interface {
	ConditionActor() conditions.Actor
}

// tracked returns what c handles; a handle on nothing panics.
func tracked(c Creature) attackable.Combatant {
	t := combatantOf(c)
	if t == nil {
		panic("script: a handle on nothing")
	}
	return t
}

// conditionOf returns c's condition view, nil when it has none.
func conditionOf(c Creature) conditions.Actor {
	switch o := tracked(c).(type) {
	case conditionHolder:
		return o.ConditionActor()
	case player.CharacterHolder:
		return o.PlayerCharacter().ConditionActor()
	}
	return nil
}

// hpRatio returns c's current HP over its max HP; a creature with no HP
// reports 0.
func hpRatio(c Creature) float64 {
	if a := conditionOf(c); a != nil {
		return a.HPRatio()
	}
	return 0
}

// Name returns the NPC's name.
func (n *NPC) Name() string { return tracked(n).CharacterName() }

// Name returns the player's name.
func (p *Player) Name() string { return tracked(p).CharacterName() }

// Name returns the creature's name.
func (c *creature) Name() string { return tracked(c).CharacterName() }

// Level returns the creature's level.
func (c *creature) Level() int32 { return int32(tracked(c).Level()) }

// Dead reports whether the creature is dead.
func (c *creature) Dead() bool { return tracked(c).Dead() }

// HPRatio returns the NPC's current HP over its max HP.
func (n *NPC) HPRatio() float64 { return hpRatio(n) }

// HPRatio returns the player's current HP over its max HP.
func (p *Player) HPRatio() float64 { return hpRatio(p) }

// HPRatio returns the creature's current HP over its max HP.
func (c *creature) HPRatio() float64 { return hpRatio(c) }

// Playable reports false: an NPC is not player-controlled.
func (n *NPC) Playable() bool { return false }

// Playable reports true: a player is player-controlled.
func (p *Player) Playable() bool { return true }

// Playable reports whether the creature is player-controlled: a summon.
func (c *creature) Playable() bool { return tracked(c).Kind().Playable() }

// Summon reports false: an NPC is no summon.
func (n *NPC) Summon() bool { return false }

// Summon reports false: a player is no summon.
func (p *Player) Summon() bool { return false }

// Summon reports whether the creature is a pet or a servitor.
func (c *creature) Summon() bool { return tracked(c).Kind() == actor.KindSummon }

// SiegeSummon reports false: an NPC is no summon.
func (n *NPC) SiegeSummon() bool { return false }

// SiegeSummon reports false: a player is no summon.
func (p *Player) SiegeSummon() bool { return false }

// SiegeSummon reports whether the creature is a siege summon.
func (c *creature) SiegeSummon() bool {
	s, ok := tracked(c).(interface{ SiegeSummon() bool })
	return ok && s.SiegeSummon()
}

func (c *creature) loc() Loc { return locOf(c) }

// Distance2D returns the NPC's distance to at, a creature or a point, on the
// ground plane.
func (n *NPC) Distance2D(at Place) float64 {
	a, b := locOf(n), at.loc()
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
}

// InRadius2D reports whether c stands strictly within radius of the NPC on
// the ground plane.
func (n *NPC) InRadius2D(c Creature, radius int) bool {
	a, b := locOf(n), locOf(c)
	dx, dy := int64(a.X-b.X), int64(a.Y-b.Y)
	return dx*dx+dy*dy < int64(radius)*int64(radius)
}

// Behind reports whether the NPC stands behind c, as c faces.
func (n *NPC) Behind(c Creature) bool {
	a, b := locOf(n), locOf(c)
	facing := location.OrientedLocation{Location: location.Location{X: b.X, Y: b.Y, Z: b.Z}, Heading: b.Heading}
	return facing.IsBehind(location.Location{X: a.X, Y: a.Y, Z: a.Z})
}

// AbortAll stops the NPC's movement, attack and cast.
func (n *NPC) AbortAll() { n.fighter("abort its actions").AbortAll(false) }

// AbortAll stops the player's movement, attack and cast, keeping its
// target.
func (p *Player) AbortAll() { p.character().AbortAll(false) }

// TeleportTo jumps the NPC to at: it stops its movement, attack and cast,
// lands on the ground at at unless at lies in water, and the players around
// either end see it go and arrive. It keeps the way it faces: at's heading
// is not read (SetHeading turns it). A teleport that starts while another
// is under way is dropped.
func (n *NPC) TeleportTo(at Place) {
	to := groundOf(at)
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.TeleportTo(to)
	case *npc.Folk:
		o.TeleportTo(to)
	default:
		panic("script: the teleport of a handle on nothing")
	}
}

// SetHeading turns the NPC to face heading where it stands. Nothing is
// sent: the players around keep the way they saw it face, and those that
// next see it, as when it arrives from a teleport, see it face heading.
func (n *NPC) SetHeading(heading int) {
	h, ok := n.combatant().(interface{ SetHeading(int) })
	if !ok {
		panic("script: the heading of a handle on nothing")
	}
	h.SetHeading(heading)
}

// summonTeleporter is a summon an ordinary teleport moves.
type summonTeleporter interface {
	TeleportTo(target location.Location)
}

// TeleportTo makes c do its ordinary teleport to at, with no scatter: an
// NPC as NPC.TeleportTo does, a player as Player.TeleportTo does with
// offset 0. A summon stops what it was doing and drops its target, lands
// on the ground at at unless at lies in water, the players around either
// end see it go and arrive, and it follows its owner again; each NPC
// around the landing that sees it runs its see-creature hooks for it once.
// A summon's teleport runs on its own queue once the caller's task is
// done, and one that starts while another is under way is dropped. Any
// other creature panics.
func TeleportTo(c Creature, at Place) {
	switch h := c.(type) {
	case *NPC:
		h.TeleportTo(at)
	case *Player:
		h.TeleportTo(at, 0)
	default:
		s, ok := tracked(c).(summonTeleporter)
		if !ok {
			panic(fmt.Sprintf("script: the teleport of %T", tracked(c)))
		}
		s.TeleportTo(groundOf(at))
	}
}

// groundOf is the point of at, without its heading.
func groundOf(at Place) location.Location {
	l := at.loc()
	return location.Location{X: l.X, Y: l.Y, Z: l.Z}
}

// instantTeleporter is a creature an instant teleport moves itself: an
// NPC or a summon.
type instantTeleporter interface {
	InstantTeleportTo(j location.Jump)
}

// instantTeleport makes c jump as j says; a creature no instant teleport
// moves, or a handle on nothing, panics.
func instantTeleport(c Creature, j location.Jump) {
	switch o := tracked(c).(type) {
	case instantTeleporter:
		o.InstantTeleportTo(j)
	case player.CharacterHolder:
		o.PlayerCharacter().InstantTeleportTo(j)
	default:
		panic(fmt.Sprintf("script: the instant teleport of %T", o))
	}
}

// jumpTo is a jump to at with no scatter.
func jumpTo(at Place) location.Jump { return location.Jump{To: groundOf(at)} }

// InstantTeleportTo moves the NPC, hostile or civilian, to at in one step:
// at is grounded unless it lies in water, the players around see the jump
// without its fade, and its path-failure count restarts. It stays in its
// region until it next moves, so what knows it and what it knows are
// unchanged until then.
func (n *NPC) InstantTeleportTo(at Place) { instantTeleport(n, jumpTo(at)) }

// InstantTeleportTo moves the player to at in one step: at is grounded
// unless the player flies or at lies in water, the player and the players
// around see the jump without its fade, the player goes idle, and each NPC
// around the region it is still held in that sees at runs its see-creature
// hooks for it once. It stays in its region until its next position update.
// A player in a teleport already is left to it.
//
// From a hook on the player's own queue (as for TeleportTo of Player) the
// jump runs inside the call. From any other queue it runs on the player's
// own queue once the caller's task is done (docs/script-engine-plan.md
// section 9): code after the call still reads the player's old position.
func (p *Player) InstantTeleportTo(at Place) { instantTeleport(p, jumpTo(at)) }

// InstantTeleportTo moves the creature to at in one step: an NPC jumps as
// InstantTeleportTo of NPC says, and a summon as a player does
// (InstantTeleportTo of Player), on its own queue, and then follows its
// owner again.
func (c *creature) InstantTeleportTo(at Place) { instantTeleport(c, jumpTo(at)) }

// InstantTeleportNear is c's InstantTeleportTo aimed at a point up to
// offset away from at on each axis: the script random source draws the x
// offset, then the y one, each in [-offset, offset], and c lands as far as
// a straight walk from at toward that point gets on the geodata. An offset
// of 0 or less aims at at itself.
func (s *Script) InstantTeleportNear(c Creature, at Place, offset int) {
	j := jumpTo(at)
	if offset > 0 {
		dx := s.env.Rand(2*offset+1) - offset
		dy := s.env.Rand(2*offset+1) - offset
		j.Toward = location.Location{X: j.To.X + dx, Y: j.To.Y + dy, Z: j.To.Z}
		j.Scattered = true
	}
	instantTeleport(c, j)
}

// ClassTypeOf returns the teaching line of the class classID: fighter,
// mystic or priest.
func ClassTypeOf(classID int32) player.ClassType {
	t, _ := player.ClassTypeOf(int(classID))
	return t
}

// AlliedWithKetra reports whether the player is allied with the Ketra
// Orcs.
func (p *Player) AlliedWithKetra() bool { return p.character().AlliedWithKetra() }

// AlliedWithVarka reports whether the player is allied with the Varka
// Silenos.
func (p *Player) AlliedWithVarka() bool { return p.character().AlliedWithVarka() }

// VarkaKetraAlliance returns the player's standing with the two factions:
// positive is the Ketra Orc alliance level, negative the Varka Silenos one,
// 0 neither.
func (p *Player) VarkaKetraAlliance() int32 { return int32(p.character().VarkaKetraAlliance()) }

// The class groups an NPC's hate parameters name, by class id.
var classGroups = map[string][]int{
	"@attacker_group":      {0, 1, 2, 3, 7, 8, 9, 18, 22, 23, 24, 31, 35, 36, 37, 44, 45, 46, 47, 48, 53, 88, 89, 92, 93, 101, 102, 108, 109, 113, 114},
	"@fighter_group":       {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 18, 19, 20, 21, 22, 23, 24, 31, 32, 33, 34, 35, 36, 37, 44, 45, 46, 47, 48, 53, 54, 55, 56, 57, 88, 89, 90, 91, 92, 93, 99, 100, 101, 102, 106, 107, 108, 109, 113, 114, 117, 118},
	"@mage_group":          {10, 11, 12, 13, 14, 15, 16, 17, 25, 26, 27, 28, 29, 30, 38, 39, 40, 41, 42, 43, 49, 50, 51, 52, 94, 95, 96, 97, 98, 103, 104, 105, 110, 111, 112, 115, 116},
	"@wizard_group":        {10, 11, 12, 13, 14, 25, 26, 27, 28, 38, 39, 40, 41, 49, 50, 51, 52, 94, 95, 96, 103, 104, 110, 111, 115, 116},
	"@cleric_group":        {10, 15, 16, 17, 25, 29, 30, 38, 42, 43, 97, 98, 105},
	"@subjob_group_knight": {5, 6, 20, 33, 90, 91, 99, 106},
}

// classOccupations are the single classes an NPC's hate parameters name.
var classOccupations = map[string]int{"@fighter": 0, "@mage": 10, "@swordsinger": 21, "@elven_wizard": 26}

// raceNames are the races an NPC's hate parameters name.
var raceNames = map[string]player.Race{
	"@race_human":    player.RaceHuman,
	"@race_elf":      player.RaceElf,
	"@race_dark_elf": player.RaceDarkElf,
	"@race_orc":      player.RaceOrc,
	"@race_dwarf":    player.RaceDwarf,
}

// InClassGroup reports whether the class the player plays is in the class
// group named group ("@fighter_group", ...); an unknown name holds none.
func (p *Player) InClassGroup(group string) bool {
	ids, ok := classGroups[group]
	if !ok {
		return false
	}
	return slices.Contains(ids, p.character().ClassID())
}

// SameOccupation reports whether the player plays the class named
// occupation ("@fighter", ...); an unknown name is no class.
func (p *Player) SameOccupation(occupation string) bool {
	id, ok := classOccupations[occupation]
	return ok && p.character().ClassID() == id
}

// SameRace reports whether the player is of the race named race
// ("@race_human", ...); an unknown name is no race.
func (p *Player) SameRace(race string) bool {
	r, ok := raceNames[race]
	return ok && p.character().Race == r
}

// Object returns the creature with the world object id id, nil when there
// is none.
func (s *Script) Object(id int32) Creature {
	if s.env == nil || s.env.World == nil {
		return nil
	}
	o, ok := s.env.World.Object(id)
	if !ok {
		return nil
	}
	c, ok := o.(attackable.Combatant)
	if !ok {
		return nil
	}
	return creatureOf(c)
}

// GameHour returns the in-game hour, 0 to 23.
func (s *Script) GameHour() int { return s.env.GameHour() }

// LocOf returns where c stands and the way it faces.
func LocOf(c Creature) Loc { return locOf(c) }

// Same reports whether a and b handle the same creature, a door included;
// a handle on nothing is the same as no creature at all.
func Same(a, b Creature) bool { return identity(a) == identity(b) }

// identity is c's world object id, 0 for no creature or a handle on
// nothing.
func identity(c Creature) int32 {
	if c == nil {
		return 0
	}
	return c.ObjectID()
}
