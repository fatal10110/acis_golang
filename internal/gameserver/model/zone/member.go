package zone

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// StepsPerRevalidation is how many movement steps pass between two zone
// revalidations of a moving creature.
const StepsPerRevalidation = 5

// Member keeps the zone membership of one creature that is not a player (an
// NPC or a summon) in step with its position. The creature's zone Actor
// embeds it, which gives the actor its flag ledger, and reports each change
// of position to it:
//
//   - Enter when it is placed on the grid: a spawn or a teleport's landing;
//   - Step for each movement step, which revalidates the zones every
//     StepsPerRevalidation steps, or at once when the step changes region;
//   - Settle when a move ends or stops, which revalidates at once;
//   - Place for a position set outside movement (a knockback landing), which
//     revalidates only when it changes region;
//   - Leave when it leaves the grid: a despawn, or a teleport before the jump.
//
// Between Leave and the next Enter no position change enters a zone, and a
// creature mid-teleport (one whose Actor reports Teleporting) counts its
// steps but revalidates nothing.
//
// mu serializes revalidation: movement reports from the creature's own
// queue, while a despawn or a teleport may come from another. The zone
// rules run with mu held, so a zone hook must not change this creature's
// membership synchronously.
type Member struct {
	mu     sync.Mutex
	flags  Flags
	onGrid bool
	steps  int
}

// ZoneFlags is the creature's zone flag ledger.
func (m *Member) ZoneFlags() *Flags { return &m.flags }

// teleporter is an actor that can be mid-teleport.
type teleporter interface{ Teleporting() bool }

func teleporting(a Actor) bool {
	t, ok := a.(teleporter)
	return ok && t.Teleporting()
}

// Enter puts a on the grid and enters the zones at its position.
func (m *Member) Enter(ix *Index, a Actor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onGrid = true
	m.steps = 0
	if ix != nil && !teleporting(a) {
		ix.Revalidate(a)
	}
}

// Step records one movement step of a from previous.
func (m *Member) Step(ix *Index, a Actor, previous location.Location) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return
	}
	m.regionChangedLocked(ix, a, previous)
	m.steps++
	if m.steps >= StepsPerRevalidation {
		m.steps = 0
		if ix != nil && !teleporting(a) {
			ix.Revalidate(a)
		}
	}
}

// Settle revalidates a's zones at once: its move ended or stopped.
func (m *Member) Settle(ix *Index, a Actor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return
	}
	m.steps = 0
	if ix != nil && !teleporting(a) {
		ix.Revalidate(a)
	}
}

// Place records a position of a set from previous outside movement: only a
// change of region revalidates its zones.
func (m *Member) Place(ix *Index, a Actor, previous location.Location) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.onGrid {
		return
	}
	m.regionChangedLocked(ix, a, previous)
}

// regionChangedLocked revalidates the zones of both regions and restarts
// the step count when a left the region containing previous. Callers hold
// mu.
func (m *Member) regionChangedLocked(ix *Index, a Actor, previous location.Location) {
	pos := a.Position()
	if world.RegionKey(previous.X, previous.Y) == world.RegionKey(pos.X, pos.Y) {
		return
	}
	m.steps = 0
	if ix != nil && !teleporting(a) {
		ix.RevalidateMove(a, previous)
	}
}

// Leave takes a off the grid, exiting every zone of the region containing
// (x, y), its position before it left.
func (m *Member) Leave(ix *Index, a Actor, x, y int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onGrid = false
	if ix != nil {
		ix.RemoveFrom(a, x, y)
	}
}
