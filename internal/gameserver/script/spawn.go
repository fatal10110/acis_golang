package script

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/rs/zerolog"
)

// Population is the live NPC population scripts spawn NPCs into.
type Population interface {
	AddSpawn(npcID int32, x, y, z, heading int, scatter bool, despawn time.Duration) (attackable.Combatant, error)
	CreatePrivate(master *npc.Hostile, npcID int32, at *location.Location, heading int, despawn time.Duration, params [3]int32) (*npc.Hostile, error)
	CreatePrivates(master *npc.Hostile) error
	ScheduleDespawn(live attackable.Combatant, d time.Duration)
}

// Spawner spawns NPCs for scripts. Every NPC it spawns is in the world,
// seen by the players around it, when the call returns.
type Spawner struct {
	pop Population
	log zerolog.Logger
}

// NewSpawner returns the spawner placing NPCs into pop.
func NewSpawner(pop Population, log zerolog.Logger) *Spawner {
	return &Spawner{pop: pop, log: log}
}

// Place is where a spawn stands: a Loc, or a creature's position and
// heading.
type Place interface {
	loc() Loc
}

// Loc is a point in the world and a heading.
type Loc struct {
	X, Y, Z, Heading int
}

func (l Loc) loc() Loc { return l }

func (n *NPC) loc() Loc { return locOf(n) }

func (p *Player) loc() Loc { return locOf(p) }

// locOf returns where c stands and the way it faces. A handle on nothing
// panics.
func locOf(c Creature) Loc {
	at := combatantOf(c)
	if at == nil {
		panic("script: the place of a handle on nothing")
	}
	x, y, z := at.Position()
	return Loc{X: x, Y: y, Z: z, Heading: at.Heading()}
}

// Params are the three spawn parameters a private starts with.
type Params struct {
	P1, P2, P3 int32
}

// AddSpawn spawns an NPC of npcID at at and returns it; nil when npcID has
// no template or the NPC cannot be placed. With randomOffset it stands up
// to 100 away on each axis, where geodata lets it; a negative heading faces
// a random way. The NPC never respawns. A positive despawn deletes it once
// that time has passed, unless it has left the world by then.
func (s *Spawner) AddSpawn(npcID int32, at Place, randomOffset bool, despawn time.Duration) *NPC {
	loc := at.loc()
	live, err := s.pop.AddSpawn(npcID, loc.X, loc.Y, loc.Z, loc.Heading, randomOffset, despawn)
	if err != nil {
		s.logRefused(npcID, err)
		return nil
	}
	return NPCOf(live)
}

// CreateOnePrivate spawns an NPC of npcID around master as one of its
// privates and returns it; nil when npcID has no template or the private
// cannot be placed. A private leaves the world with its master, and one
// that cannot respawn stops being master's private when it decays. A
// positive despawn deletes it once that time has passed, unless it has
// left the world by then.
func (s *Spawner) CreateOnePrivate(master *NPC, npcID int32, despawn time.Duration) *NPC {
	return s.createPrivate(master, npcID, nil, 0, despawn, Params{})
}

// CreateOnePrivateEx is CreateOnePrivate for a private standing at at, on
// the ground below it, that starts with params as its spawn parameters.
func (s *Spawner) CreateOnePrivateEx(master *NPC, npcID int32, at Place, despawn time.Duration, params Params) *NPC {
	loc := at.loc()
	point := location.Location{X: loc.X, Y: loc.Y, Z: loc.Z}
	return s.createPrivate(master, npcID, &point, loc.Heading, despawn, params)
}

func (s *Spawner) createPrivate(master *NPC, npcID int32, at *location.Location, heading int, despawn time.Duration, params Params) *NPC {
	h := hostileOf(master, "create a private")
	pvt, err := s.pop.CreatePrivate(h, npcID, at, heading, despawn, [3]int32{params.P1, params.P2, params.P3})
	if err != nil {
		s.logRefused(npcID, err)
		return nil
	}
	return NPCOf(pvt)
}

// CreatePrivates spawns master's privates: the ones its spawn declares, or
// its template's when the spawn declares none. master first forgets the
// privates it had, which stay in the world. Each private keeps the respawn
// delay and weight point its declaration gives. It panics on a declaration
// naming an NPC id no template has, the privates before it placed.
func (s *Spawner) CreatePrivates(master *NPC) {
	if err := s.pop.CreatePrivates(hostileOf(master, "create privates")); err != nil {
		panic(fmt.Sprintf("script: create privates: %v", err))
	}
}

// ScheduleDespawn deletes n once d has passed, unless it has left the
// world by then. A d that is not positive schedules nothing. A handle on
// nothing panics.
func (s *Spawner) ScheduleDespawn(n *NPC, d time.Duration) {
	c := combatantOf(n)
	if c == nil {
		panic("script: schedule the despawn of a handle on nothing")
	}
	s.pop.ScheduleDespawn(c, d)
}

// logRefused logs a spawn of npcID that placed nothing; a missing template
// places nothing silently.
func (s *Spawner) logRefused(npcID int32, err error) {
	if !errors.Is(err, npc.ErrNoTemplate) {
		s.log.Error().Err(err).Int32("npc_id", npcID).Msg("script: spawn placed nothing")
	}
}

// hostileOf returns the hostile NPC master handles. A master that is no
// hostile NPC panics: only a hostile NPC keeps privates yet (#3530).
func hostileOf(master *NPC, what string) *npc.Hostile {
	h, ok := combatantOf(master).(*npc.Hostile)
	if !ok {
		panic(fmt.Sprintf("script: %s for %T: only a hostile npc keeps privates", what, combatantOf(master)))
	}
	return h
}
