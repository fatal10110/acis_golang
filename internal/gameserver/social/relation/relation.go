// Package relation owns the friend and block relations between characters:
// who is on whose friend list, who ignores whom, and the friend invitations
// waiting for an answer.
//
// Relations live in memory for the whole run. They are loaded once at boot
// from character_relations and written back as a whole at shutdown (see
// Manager.Rows), so a change made during the run reaches the table only
// then.
//
// Other systems ask this package two questions about a message or request
// one player sends another: Manager.IsBlocked (is the sender on the
// recipient's block list) and the recipient's own block-everything state,
// which lives on the character (player.Character.BlockingAll).
package relation

import (
	"slices"
	"sync"
)

// The relation flags stored for one pair of characters. A pair is keyed by
// its lower id first, so the two block flags say which side does the
// blocking.
const (
	flagFriends       int32 = 1 // both characters are on each other's friend list
	flagLowBlocksHigh int32 = 2 // the lower id ignores the higher id
	flagHighBlocksLow int32 = 4 // the higher id ignores the lower id
)

// Row is one character_relations row: the pair's lower id, its higher id and
// the pair's relation flags. A Relation of 0 marks a pair whose last flag was
// cleared during the run; its row is deleted when the relations are saved.
type Row struct {
	CharID   int32
	FriendID int32
	Relation int32
}

type pair struct{ low, high int32 }

func makePair(a, b int32) pair {
	if a > b {
		a, b = b, a
	}
	return pair{a, b}
}

// Manager holds every character's relations. mu guards relations; any
// goroutine may call any method.
type Manager struct {
	mu sync.RWMutex
	// relations keeps a pair whose flags were all cleared, with 0, until
	// the run ends: the save deletes its row.
	relations map[pair]int32
}

// NewManager returns a manager holding rows. A pair listed twice, in either
// order, keeps the first row's flags.
func NewManager(rows []Row) *Manager {
	m := &Manager{relations: make(map[pair]int32, len(rows))}
	for _, r := range rows {
		k := makePair(r.CharID, r.FriendID)
		if _, ok := m.relations[k]; !ok {
			m.relations[k] = r.Relation
		}
	}
	return m
}

// AreFriends reports whether a and b are on each other's friend list.
func (m *Manager) AreFriends(a, b int32) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.relations[makePair(a, b)]&flagFriends != 0
}

// IsBlocked reports whether target is on owner's block list: a message or
// request target sends owner is refused.
func (m *Manager) IsBlocked(owner, target int32) bool {
	k := makePair(owner, target)
	m.mu.RLock()
	rel := m.relations[k]
	m.mu.RUnlock()
	return (owner == k.low && rel&flagLowBlocksHigh != 0) || (owner == k.high && rel&flagHighBlocksLow != 0)
}

// FriendIDs returns the ids on id's friend list, in the order the client is
// sent them (see clientOrder).
func (m *Manager) FriendIDs(id int32) []int32 {
	m.mu.RLock()
	var ids []int32
	for k, rel := range m.relations {
		if rel&flagFriends == 0 {
			continue
		}
		switch id {
		case k.low:
			ids = append(ids, k.high)
		case k.high:
			ids = append(ids, k.low)
		}
	}
	m.mu.RUnlock()
	return clientOrder(ids)
}

// BlockedIDs returns the ids on id's block list, in the order the client is
// sent them (see clientOrder).
func (m *Manager) BlockedIDs(id int32) []int32 {
	m.mu.RLock()
	var ids []int32
	for k, rel := range m.relations {
		switch {
		case id == k.low && rel&flagLowBlocksHigh != 0:
			ids = append(ids, k.high)
		case id == k.high && rel&flagHighBlocksLow != 0:
			ids = append(ids, k.low)
		}
	}
	m.mu.RUnlock()
	return clientOrder(ids)
}

// AddFriend puts a and b on each other's friend list.
func (m *Manager) AddFriend(a, b int32) {
	m.update(a, b, flagFriends, true)
}

// RemoveFriend takes a and b off each other's friend list. It reports false,
// changing nothing, when they were not friends.
func (m *Manager) RemoveFriend(a, b int32) bool {
	return m.update(a, b, flagFriends, false)
}

// Block puts target on owner's block list.
func (m *Manager) Block(owner, target int32) {
	m.update(owner, target, blockFlag(owner, target), true)
}

// Unblock takes target off owner's block list. It reports false, changing
// nothing, when target was not on it.
func (m *Manager) Unblock(owner, target int32) bool {
	return m.update(owner, target, blockFlag(owner, target), false)
}

func blockFlag(owner, target int32) int32 {
	if owner < target {
		return flagLowBlocksHigh
	}
	return flagHighBlocksLow
}

// update sets or clears flag on the pair a, b. Clearing reports false when
// the flag was not set; a character has no relation with itself.
func (m *Manager) update(a, b, flag int32, set bool) bool {
	if a == b {
		return false
	}
	k := makePair(a, b)
	m.mu.Lock()
	defer m.mu.Unlock()
	rel, ok := m.relations[k]
	if set {
		m.relations[k] = rel | flag
		return true
	}
	if !ok || rel&flag == 0 {
		return false
	}
	m.relations[k] = rel &^ flag
	return true
}

// Rows returns every pair the manager holds, lower id first, sorted by that
// pair: the ones with flags to upsert and the cleared ones (Relation 0)
// whose rows to delete.
func (m *Manager) Rows() []Row {
	m.mu.RLock()
	rows := make([]Row, 0, len(m.relations))
	for k, rel := range m.relations {
		rows = append(rows, Row{CharID: k.low, FriendID: k.high, Relation: rel})
	}
	m.mu.RUnlock()
	slices.SortFunc(rows, func(a, b Row) int {
		if a.CharID != b.CharID {
			return int(a.CharID) - int(b.CharID)
		}
		return int(a.FriendID) - int(b.FriendID)
	})
	return rows
}
