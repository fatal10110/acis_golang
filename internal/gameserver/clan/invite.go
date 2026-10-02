package clan

import (
	"sync"
	"time"
)

// InviteTimeout is how long a clan invitation keeps both sides busy.
const InviteTimeout = 15 * time.Second

// InviteKind names the request a pending invitation stands for. Clan and
// alliance invitations share one book: a player busy with either is busy
// for both.
type InviteKind int

// The invitation kinds.
const (
	InviteJoinPledge InviteKind = iota + 1
	InviteJoinAlly
)

// Invite is one side's view of a pending invitation.
type Invite struct {
	Kind InviteKind
	// PartnerID is the other side: the invited player for the requester,
	// the requester for the invited player.
	PartnerID int32
	// RequesterID is the player that sent the invitation, RequesterName
	// its name, kept for answers that arrive once it has left.
	RequesterID   int32
	RequesterName string
	// PledgeType is the sub-unit a JoinPledge invitation is for.
	PledgeType int
	expiresAt  time.Time
	// left marks a side whose login is gone: its partner may still answer,
	// but a new login under the same id is not busy.
	left bool
}

// InviteStatus is the outcome of sending an invitation.
type InviteStatus int

// The invitation outcomes.
const (
	InviteSent InviteStatus = iota
	// InviteTargetBusy refuses an invitation to a player already taking
	// part in one.
	InviteTargetBusy
	// InviteRequesterBusy refuses a second invitation while the first is
	// pending.
	InviteRequesterBusy
)

// Invites holds each player's pending invitation. Both sides of one hold an
// entry; each lapses on its own after InviteTimeout. Answering clears only
// the answering side, so the requester stays busy until its own entry
// lapses. mu guards entries.
type Invites struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[int32]Invite
}

// NewInvites returns an empty book reading time from now (time.Now when
// nil).
func NewInvites(now func() time.Time) *Invites {
	if now == nil {
		now = time.Now
	}
	return &Invites{now: now, entries: map[int32]Invite{}}
}

// activeLocked returns objectID's live entry: one that has not lapsed and
// whose login is still there.
func (b *Invites) activeLocked(objectID int32, at time.Time) (Invite, bool) {
	e, ok := b.entries[objectID]
	if !ok || !at.Before(e.expiresAt) {
		delete(b.entries, objectID)
		return Invite{}, false
	}
	return e, !e.left
}

// Send records an invitation from requesterID to targetID.
func (b *Invites) Send(kind InviteKind, requesterID int32, requesterName string, targetID int32, pledgeType int) InviteStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	at := b.now()
	if _, busy := b.activeLocked(targetID, at); busy {
		return InviteTargetBusy
	}
	if _, busy := b.activeLocked(requesterID, at); busy {
		return InviteRequesterBusy
	}
	expires := at.Add(InviteTimeout)
	sent := Invite{Kind: kind, RequesterID: requesterID, RequesterName: requesterName, PledgeType: pledgeType, expiresAt: expires}
	sent.PartnerID = targetID
	b.entries[requesterID] = sent
	sent.PartnerID = requesterID
	b.entries[targetID] = sent
	return InviteSent
}

// Partner returns the pending invitation objectID was sent, if it still
// stands.
func (b *Invites) Partner(objectID int32) (Invite, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.activeLocked(objectID, b.now())
	if !ok || e.RequesterID == objectID {
		return Invite{}, false
	}
	return e, true
}

// Requested returns the invitation requesterID sent, if its own side has
// not lapsed; a requester that has since left still counts.
func (b *Invites) Requested(requesterID int32) (Invite, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[requesterID]
	if !ok || !b.now().Before(e.expiresAt) || e.RequesterID != requesterID {
		return Invite{}, false
	}
	return e, true
}

// Answered clears objectID's own entry once it has answered.
func (b *Invites) Answered(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, objectID)
}

// Left marks objectID's entry as belonging to a login that is gone.
func (b *Invites) Left(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e, ok := b.entries[objectID]; ok {
		e.left = true
		b.entries[objectID] = e
	}
}
