package festival

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Member is one party member as a festival keeps it.
type Member struct {
	ObjectID int32
	Name     string
}

// roster is who signed up for each festival of one oracle, by festival id.
// A festival present with a nil list was signed up for and then dropped:
// it still counts as taken when the signup period ends, but no arena
// opens for it.
type roster map[int][]Member

func (r roster) holds(objectID int32) bool {
	for _, members := range r {
		if slices.ContainsFunc(members, func(m Member) bool { return m.ObjectID == objectID }) {
			return true
		}
	}
	return false
}

// side is where a cabal's festivals are kept in a pair of rosters: Dusk's
// first.
func side(cabal sevensigns.Cabal) int {
	if cabal == sevensigns.Dawn {
		return 1
	}
	return 0
}

// IsParticipant reports whether the player objectID is signed up for a
// festival this cycle. Nobody is during seal validation, nor before a
// festival schedule has ever started.
func (m *Manager) IsParticipant(objectID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isParticipantLocked(objectID)
}

func (m *Manager) isParticipantLocked(objectID int32) bool {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation || !m.started {
		return false
	}
	return m.participants[0].holds(objectID) || m.participants[1].holds(objectID)
}

// Initialized reports whether a festival has begun: its parties are in
// their arenas, and no party may sign up until it ends.
func (m *Manager) Initialized() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initialized
}

// festivalOfLocked returns the oracle and festival the participant
// objectID signed up for. The search looks at the Dawn festivals 0, 2 and 4
// and the Dusk festivals 1 and 3 only: a participant of any other festival
// is never found, and nothing that needs its festival reaches it.
func (m *Manager) festivalOfLocked(objectID int32) (sevensigns.Cabal, int, bool) {
	for id := 0; id < Count; id += 2 {
		if m.participants[side(sevensigns.Dawn)].holdsIn(id, objectID) {
			return sevensigns.Dawn, id, true
		}
		if m.participants[side(sevensigns.Dusk)].holdsIn(id+1, objectID) {
			return sevensigns.Dusk, id + 1, true
		}
	}
	return sevensigns.NoCabal, -1, false
}

func (r roster) holdsIn(id int, objectID int32) bool {
	return slices.ContainsFunc(r[id], func(m Member) bool { return m.ObjectID == objectID })
}

// setParticipantsLocked signs party up for festival id of cabal in place
// of whoever was, nil dropping the festival's party but keeping the
// festival taken.
func (m *Manager) setParticipantsLocked(cabal sevensigns.Cabal, id int, party []Member) {
	m.participants[side(cabal)][id] = slices.Clone(party)
}

// UpdateParticipants follows a change of the party of the participant
// objectID: party is its party's members as they now stand, nil when the
// participant leads the party out of the festival. Once the festival has
// begun, the participant, or with a nil party every participant online, is
// taken out of the arena. Then party is signed up in place of the old one;
// a party of fewer than MinPlayers members loses its run, and expel then
// asks for the participant to be expelled from its party. A player who is
// no participant, or whose festival is never found (see festivalOfLocked),
// changes nothing.
func (m *Manager) UpdateParticipants(objectID int32, party []Member) (expel bool) {
	m.mu.Lock()
	var moves []relocation
	expel = m.updateParticipantsLocked(objectID, party, &moves)
	world := m.world
	m.mu.Unlock()
	m.relocate(world, moves)
	return expel
}

func (m *Manager) updateParticipantsLocked(objectID int32, party []Member, moves *[]relocation) (expel bool) {
	if !m.isParticipantLocked(objectID) {
		return false
	}
	cabal, id, ok := m.festivalOfLocked(objectID)
	if !ok {
		return false
	}
	if m.initialized {
		if a := m.arenas[side(cabal)][id]; a != nil {
			if party == nil {
				for _, p := range m.participants[side(cabal)][id] {
					*moves = append(*moves, relocation{arena: a, objectID: p.ObjectID, removing: true})
				}
			} else {
				*moves = append(*moves, relocation{arena: a, objectID: objectID, removing: true})
			}
		}
	}
	m.setParticipantsLocked(cabal, id, party)
	if party != nil && len(party) < m.cfg.MinPlayers {
		m.updateParticipantsLocked(objectID, nil, moves)
		return true
	}
	return false
}

// Departure is where the participant objectID, leaving the world while
// in a party, is taken back to as its party lets it go: its arena's record
// of where it came from, forgotten as it is read, or nil for the nearest
// town when the arena never took it in. ok is false when the leave takes
// it out of no arena: it is no participant, its festival is never found
// (see festivalOfLocked), or no festival has begun.
func (m *Manager) Departure(objectID int32) (at *Spot, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isParticipantLocked(objectID) || !m.initialized {
		return nil, false
	}
	cabal, id, found := m.festivalOfLocked(objectID)
	if !found {
		return nil, false
	}
	a := m.arenas[side(cabal)][id]
	if a == nil {
		return nil, false
	}
	origin, known := a.origins[objectID]
	if !known {
		return nil, true
	}
	delete(a.origins, objectID)
	return &origin, true
}

// resetParticipantsLocked forgets every party signed up and every party
// waiting to register its score.
func (m *Manager) resetParticipantsLocked() {
	m.participants = [2]roster{{}, {}}
	m.previous = [2]roster{{}, {}}
}
