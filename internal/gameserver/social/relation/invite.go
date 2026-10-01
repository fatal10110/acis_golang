package relation

import (
	"sync"
	"time"
)

// InviteTimeout is how long a friend invitation stays answerable. Every
// invitation a player sends restarts its own clock, so all of that player's
// unanswered invitations share the latest deadline, and the first answer to
// any of them ends them all.
const InviteTimeout = 15 * time.Second

// inviter is one login's invitation clock. A relog starts a fresh one; the
// invitations the earlier login sent keep pointing at the old one.
type inviter struct {
	id        int32
	name      string
	expiresAt time.Time
	// left marks a login that left the world after inviting. Its
	// invitations stay answerable until they expire, but that login can no
	// longer hear the answer.
	left bool
}

// Invites holds the friend invitations waiting for an answer, by target.
// mu guards both maps; any goroutine may call any method.
type Invites struct {
	mu  sync.Mutex
	now func() time.Time
	// inviters is each online player's own invitation clock.
	inviters map[int32]*inviter
	// pending is the invitation each target holds: the last one it got.
	pending map[int32]*inviter
}

// NewInvites returns an empty invitation book timed by now; nil means
// time.Now.
func NewInvites(now func() time.Time) *Invites {
	if now == nil {
		now = time.Now
	}
	return &Invites{now: now, inviters: make(map[int32]*inviter), pending: make(map[int32]*inviter)}
}

// Invite is an answered invitation's sender.
type Invite struct {
	RequesterID   int32
	RequesterName string
	// RequesterLeft reports that the login which sent the invitation has
	// left the world since; a later login of the same character is not it.
	RequesterLeft bool
}

// busyLocked reports whether id holds an unanswered invitation or has sent
// one that is still answerable.
func (iv *Invites) busyLocked(id int32, now time.Time) bool {
	if from := iv.pending[id]; from != nil && now.Before(from.expiresAt) {
		return true
	}
	own := iv.inviters[id]
	return own != nil && now.Before(own.expiresAt)
}

// Offer records requester's invitation to target, unless target is busy
// with an invitation or, as targetBusy reports, with some other request.
// It reports whether the invitation was recorded.
func (iv *Invites) Offer(requesterID int32, requesterName string, targetID int32, targetBusy bool) bool {
	iv.mu.Lock()
	defer iv.mu.Unlock()
	now := iv.now()
	if targetBusy || iv.busyLocked(targetID, now) {
		return false
	}
	from := iv.inviters[requesterID]
	if from == nil {
		from = &inviter{id: requesterID}
		iv.inviters[requesterID] = from
	}
	from.name = requesterName
	from.expiresAt = now.Add(InviteTimeout)
	iv.pending[targetID] = from
	return true
}

// Answer takes the invitation targetID holds. It reports false when there
// is none or it has expired. Answering ends every other invitation the same
// login sent.
func (iv *Invites) Answer(targetID int32) (Invite, bool) {
	iv.mu.Lock()
	defer iv.mu.Unlock()
	from := iv.pending[targetID]
	delete(iv.pending, targetID)
	if from == nil || !iv.now().Before(from.expiresAt) {
		return Invite{}, false
	}
	from.expiresAt = time.Time{}
	return Invite{RequesterID: from.id, RequesterName: from.name, RequesterLeft: from.left}, true
}

// Leave forgets the invitation id holds and retires id's own invitation
// clock: invitations id sent stay answerable until they expire, but are
// marked as sent by a login that has left.
func (iv *Invites) Leave(id int32) {
	iv.mu.Lock()
	defer iv.mu.Unlock()
	delete(iv.pending, id)
	if own := iv.inviters[id]; own != nil {
		own.left = true
		delete(iv.inviters, id)
	}
}
