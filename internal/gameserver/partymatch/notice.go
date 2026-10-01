package partymatch

// Notice is one client-visible consequence of a party-matching change.
// Operations return them in the order their packets reach clients.
type Notice interface{ isNotice() }

type notices []Notice

func (n *notices) add(notice Notice) { *n = append(*n, notice) }

// MessageID names a party-matching system message.
type MessageID uint8

// Party-matching system messages.
const (
	MsgRoomCreated MessageID = iota
	MsgRoomRevised
	MsgRoomDisbanded
	MsgRoomLeaderChanged
	// MsgEnteredRoom: Name entered the room.
	MsgEnteredRoom
	// MsgLeftRoom: Name left the room.
	MsgLeftRoom
)

// ListMode is how a room's member list is shown, as the client numbers it.
type ListMode int32

// Member list modes.
const (
	ListEntered ListMode = iota
	ListOpened
	ListRevised
)

// ChangeMode is what happened to one row of a room's member list, as the
// client numbers it.
type ChangeMode int32

// Member row changes.
const (
	ChangeAdded ChangeMode = iota
	ChangeUpdated
	ChangeRemoved
)

// Detail shows To the terms of Room.
type Detail[M Member] struct {
	To   M
	Room Room[M]
}

// MemberList shows To the members of Room.
type MemberList[M Member] struct {
	To   M
	Room Room[M]
	Mode ListMode
}

// MemberChange changes Member's row on each recipient's member list.
// Leader is the room's leader at the time: the row marks the leader, and
// the members of the leader's party.
type MemberChange[M Member] struct {
	To     []M
	Member M
	Leader M
	Mode   ChangeMode
}

// Msg is a party-matching system message; Name fills its one parameter, if
// any.
type Msg[M Member] struct {
	To   []M
	ID   MessageID
	Name string
}

// Close closes To's room window.
type Close[M Member] struct{ To M }

// InfoRefresh resends Member's full view to itself and its observers.
type InfoRefresh[M Member] struct{ Member M }

// RoomList shows To the rooms it may browse.
type RoomList[M Member] struct {
	To    M
	Rooms []Room[M]
}

func (Detail[M]) isNotice()       {}
func (MemberList[M]) isNotice()   {}
func (MemberChange[M]) isNotice() {}
func (Msg[M]) isNotice()          {}
func (Close[M]) isNotice()        {}
func (InfoRefresh[M]) isNotice()  {}
func (RoomList[M]) isNotice()     {}
