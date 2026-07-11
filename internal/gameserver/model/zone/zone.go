package zone

import (
	"fmt"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Class is the broad actor family a zone occupant belongs to. Zones use it
// to decide which of their rules apply to a given occupant.
type Class uint8

// Occupant families, from least to most player-like.
const (
	// ClassOther covers doors, static objects and anything not listed below.
	ClassOther Class = iota
	// ClassNPC covers world NPCs and monsters.
	ClassNPC
	// ClassSummon covers player-owned summons and pets.
	ClassSummon
	// ClassPlayer covers player characters.
	ClassPlayer
)

// String names the class.
func (c Class) String() string {
	switch c {
	case ClassOther:
		return "Other"
	case ClassNPC:
		return "NPC"
	case ClassSummon:
		return "Summon"
	case ClassPlayer:
		return "Player"
	default:
		return fmt.Sprintf("Class(%d)", uint8(c))
	}
}

// Playable reports whether the class is directly player-controlled or
// player-owned.
func (c Class) Playable() bool { return c == ClassSummon || c == ClassPlayer }

// Actor is what zones need from anything that can stand inside them.
type Actor interface {
	// ObjectID is the actor's world-unique id.
	ObjectID() int32
	// Position is the actor's current world location.
	Position() location.Location
	// ZoneFlags is the actor's zone flag ledger; zones raise and release
	// flags on it as the actor crosses their bounds.
	ZoneFlags() *Flags
	// Class tells the zone which occupant family the actor belongs to.
	Class() Class
}

// Player is the extra capability set zones consult when an occupant is a
// player character. Actors of ClassPlayer are expected to implement it.
type Player interface {
	Actor
	// GM reports whether the player has game-master privileges.
	GM() bool
	// Online reports whether the player's client is still connected.
	Online() bool
	// Race is the player character's race.
	Race() player.Race
	// ClanID is the player's clan id, 0 when clanless.
	ClanID() int32
}

// Owned is implemented by actors that belong to a player (summons, pets).
type Owned interface {
	Actor
	// Owner returns the owning player, if it is still resolvable.
	Owner() (Player, bool)
}

// Zone is the identity, footprint and occupancy state shared by every zone
// kind. Concrete kinds embed it and layer their own rules on top.
//
// mu guards occupants, enterWatchers and exitWatchers.
type Zone struct {
	id   int
	form Form

	mu            sync.RWMutex
	occupants     map[int32]Actor
	enterWatchers []func(Actor)
	exitWatchers  []func(Actor)
}

func newZone(id int, form Form) Zone {
	return Zone{id: id, form: form, occupants: make(map[int32]Actor)}
}

// ID is the zone's id: explicit from data, or assigned at load time.
func (z *Zone) ID() int { return z.id }

// Form is the zone's geometric footprint.
func (z *Zone) Form() Form { return z.form }

// Contains reports whether the location lies inside the zone's volume.
func (z *Zone) Contains(loc location.Location) bool {
	return z.form.Contains(loc.X, loc.Y, loc.Z)
}

// ContainsPoint reports whether (x, y, zz) lies inside the zone's volume.
func (z *Zone) ContainsPoint(x, y, zz int) bool { return z.form.Contains(x, y, zz) }

// ContainsXY reports whether (x, y) lies inside the zone's footprint,
// probed at the volume's upper z bound.
func (z *Zone) ContainsXY(x, y int) bool { return z.form.Contains(x, y, z.form.HighZ()) }

// Inside reports whether a is currently tracked as an occupant.
func (z *Zone) Inside(a Actor) bool {
	z.mu.RLock()
	defer z.mu.RUnlock()
	_, ok := z.occupants[a.ObjectID()]
	return ok
}

// Occupants returns a snapshot of every tracked occupant.
func (z *Zone) Occupants() []Actor {
	z.mu.RLock()
	defer z.mu.RUnlock()
	out := make([]Actor, 0, len(z.occupants))
	for _, a := range z.occupants {
		out = append(out, a)
	}
	return out
}

// OnEnter registers fn to run every time an actor crosses into the zone,
// before the zone's own entry rules apply. Script triggers attach here.
func (z *Zone) OnEnter(fn func(Actor)) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.enterWatchers = append(z.enterWatchers, fn)
}

// OnExit registers fn to run every time an actor leaves the zone, before
// the zone's own exit rules apply. Script triggers attach here.
func (z *Zone) OnExit(fn func(Actor)) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.exitWatchers = append(z.exitWatchers, fn)
}

// admit tracks a as an occupant, reporting whether it was newly added.
func (z *Zone) admit(a Actor) bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	id := a.ObjectID()
	if _, ok := z.occupants[id]; ok {
		return false
	}
	z.occupants[id] = a
	return true
}

// evict stops tracking a, reporting whether it was present.
func (z *Zone) evict(a Actor) bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	id := a.ObjectID()
	if _, ok := z.occupants[id]; !ok {
		return false
	}
	delete(z.occupants, id)
	return true
}

func (z *Zone) snapshotEnterWatchers() []func(Actor) {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.enterWatchers[:len(z.enterWatchers):len(z.enterWatchers)]
}

func (z *Zone) snapshotExitWatchers() []func(Actor) {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.exitWatchers[:len(z.exitWatchers):len(z.exitWatchers)]
}

// playersInside snapshots the occupants of ClassPlayer matching keep (nil
// keeps all).
func (z *Zone) playersInside(keep func(Actor) bool) []Actor {
	z.mu.RLock()
	defer z.mu.RUnlock()
	var out []Actor
	for _, a := range z.occupants {
		if a.Class() == ClassPlayer && (keep == nil || keep(a)) {
			out = append(out, a)
		}
	}
	return out
}

// Kind is one concrete zone: the shared Zone core plus kind-specific entry
// and exit rules. All kinds live in this package; the rule methods stay
// internal so state transitions only happen through Revalidate and Remove.
type Kind interface {
	// Core exposes the shared identity/occupancy state.
	Core() *Zone
	// affects reports whether the zone's rules apply to a at all.
	affects(a Actor) bool
	// enter applies the zone's entry rules to a freshly admitted occupant.
	enter(a Actor)
	// exit applies the zone's exit rules to a just-evicted occupant.
	exit(a Actor)
}

// Revalidate synchronizes k's occupancy with a's current position: an
// affected actor found inside the bounds is admitted (firing enter
// watchers, then the kind's entry rules, exactly once), and one found
// outside is removed via Remove. Actors the zone does not affect are left
// untouched.
func Revalidate(k Kind, a Actor) {
	if !k.affects(a) {
		return
	}
	z := k.Core()
	if z.Contains(a.Position()) {
		if z.admit(a) {
			for _, fn := range z.snapshotEnterWatchers() {
				fn(a)
			}
			k.enter(a)
		}
	} else {
		Remove(k, a)
	}
}

// Remove evicts a from k if it was an occupant, firing exit watchers and
// then the kind's exit rules. Removing a non-occupant is a no-op.
func Remove(k Kind, a Actor) {
	z := k.Core()
	if z.evict(a) {
		for _, fn := range z.snapshotExitWatchers() {
			fn(a)
		}
		k.exit(a)
	}
}
