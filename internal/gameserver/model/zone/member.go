package zone

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// StepsPerRevalidation is how many movement steps pass between two zone
// revalidations of a moving creature.
const StepsPerRevalidation = 5

// Member keeps the zone membership of one creature (a player, an NPC or a
// summon) in step with its position. The creature's zone Actor embeds it,
// which gives the actor its flag ledger, and reports each change of
// position to it:
//
//   - Enter when it is placed on the grid: a spawn or a teleport's landing;
//   - Step for each movement step, which revalidates the zones every
//     StepsPerRevalidation steps, or at once when the step changes region;
//   - Settle when a move ends or stops, which revalidates at once;
//   - Moved for a position update that revalidates at once, across both
//     regions when it changed region;
//   - Place for a position set outside movement (a knockback landing), which
//     revalidates only when it changes region;
//   - Leave when it leaves the grid: a despawn, or a teleport before the jump;
//   - Remove when it leaves the grid for good (a logout): no Enter puts it
//     back.
//
// An instant teleport reports nothing: the creature stays held in its
// region, and its zones are those of that region, until its next Step or
// Place moves it to the region of its position.
//
// Between Leave and the next Enter no position change enters a zone; the
// position reports return false meanwhile. A creature mid-teleport (one
// whose Actor reports Teleporting) counts its steps but revalidates
// nothing.
//
// mu serializes revalidation: movement reports from the creature's own
// queue, while a despawn or a teleport may come from another. The zone
// rules and their Effects run with mu held, so an effect must not change
// this creature's membership synchronously.
type Member struct {
	mu     sync.Mutex
	flags  Flags
	onGrid bool
	// removed is set for good by Remove.
	removed bool
	steps   int
	// held is a point of the region holding the creature: its position at
	// the last Enter, Step or Place.
	held location.Location
}

// ZoneFlags is the creature's zone flag ledger.
func (m *Member) ZoneFlags() *Flags { return &m.flags }

// ReplayZoneRules runs fn, a zone switching its rules replaying them on the
// creature, with the creature's zone membership locked.
func (m *Member) ReplayZoneRules(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn()
}

// teleporter is an actor that can be mid-teleport.
type teleporter interface{ Teleporting() bool }

func teleporting(a Actor) bool {
	t, ok := a.(teleporter)
	return ok && t.Teleporting()
}

// Enter puts a on the grid and enters the zones at its position. It
// reports false, entering nothing, once a was removed.
func (m *Member) Enter(ix *Index, a Actor) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.removed {
		return false
	}
	m.onGrid = true
	m.steps = 0
	m.held = a.Position()
	if ix != nil && !teleporting(a) {
		ix.Revalidate(a)
	}
	return true
}

// Step records one movement step of a. It reports whether a is on the grid.
func (m *Member) Step(ix *Index, a Actor) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return false
	}
	m.regionChangedLocked(ix, a)
	m.steps++
	if m.steps >= StepsPerRevalidation {
		m.steps = 0
		if ix != nil && !teleporting(a) {
			ix.RevalidateIn(a, m.held)
		}
	}
	return true
}

// Settle revalidates a's zones at once, against those of the region
// holding it: its move ended or stopped. It reports whether a is on the
// grid.
func (m *Member) Settle(ix *Index, a Actor) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return false
	}
	m.steps = 0
	if ix != nil && !teleporting(a) {
		ix.RevalidateIn(a, m.held)
	}
	return true
}

// Moved revalidates a's zones at once after a position update: those of
// both regions when the update left the region holding a, else those of
// that region. It reports whether a is on the grid.
func (m *Member) Moved(ix *Index, a Actor) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return false
	}
	changed := m.regionChangedLocked(ix, a)
	m.steps = 0
	if !changed && ix != nil && !teleporting(a) {
		ix.RevalidateIn(a, m.held)
	}
	return true
}

// Place records a position of a set outside movement: only a change of
// region revalidates its zones. It reports whether a is on the grid.
func (m *Member) Place(ix *Index, a Actor) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return false
	}
	m.regionChangedLocked(ix, a)
	return true
}

// regionChangedLocked moves a to the region of its position: when that is
// not the region holding it, it revalidates the zones of both regions and
// restarts the step count, reporting true. Callers hold mu.
func (m *Member) regionChangedLocked(ix *Index, a Actor) bool {
	pos := a.Position()
	held := m.held
	m.held = pos
	if world.RegionKey(held.X, held.Y) == world.RegionKey(pos.X, pos.Y) {
		return false
	}
	m.steps = 0
	if ix != nil && !teleporting(a) {
		ix.RevalidateMove(a, held)
	}
	return true
}

// Leave takes a off the grid, exiting every zone of the region holding it.
func (m *Member) Leave(ix *Index, a Actor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onGrid = false
	if ix != nil {
		ix.RemoveFrom(a, m.held.X, m.held.Y)
	}
}

// Remove takes a off the grid for good, exiting every zone of the region
// holding it: a reload that still lists a must not admit it into the
// reloaded zones, and no later Enter puts it back.
func (m *Member) Remove(ix *Index, a Actor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onGrid, m.removed = false, true
	if ix != nil {
		ix.RemoveFrom(a, m.held.X, m.held.Y)
	}
}
