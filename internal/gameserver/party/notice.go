package party

// Notice is one client-visible consequence of a party change. Operations
// return them in the order their packets reach clients.
type Notice interface{ isNotice() }

type notices []Notice

func (n *notices) add(notice Notice) { *n = append(*n, notice) }

// MessageID names a party system message.
type MessageID uint8

// Party system messages.
const (
	// MsgYouJoinedParty: you joined Name's party.
	MsgYouJoinedParty MessageID = iota
	// MsgJoinedParty: Name joined the party.
	MsgJoinedParty
	MsgPartyDispersed
	MsgExpelledFromParty
	// MsgWasExpelled: Name was expelled from the party.
	MsgWasExpelled
	MsgYouLeftParty
	// MsgLeftParty: Name left the party.
	MsgLeftParty
	// MsgBecameLeader: Name became the party leader.
	MsgBecameLeader
	MsgCannotTransferToSelf
	MsgOnlyLeaderTransfers
	// MsgChannelLeaderNow: Name now leads the command channel.
	MsgChannelLeaderNow
	MsgChannelFormed
	MsgJoinedChannel
	MsgChannelDisbanded
	MsgDismissedFromChannel
	// MsgPartyDismissedFromChannel: Name's party was dismissed from the
	// command channel.
	MsgPartyDismissedFromChannel
	MsgLeftChannel
	// MsgPartyLeftChannel: Name's party left the command channel.
	MsgPartyLeftChannel
)

// WindowAll replaces To's party window: its leader, loot rule and the other
// members.
type WindowAll[M Member] struct {
	To     M
	Leader int32
	Loot   LootRule
	Others []M
}

// WindowAdd adds Member to each recipient's party window.
type WindowAdd[M Member] struct {
	To     []M
	Leader int32
	Loot   LootRule
	Member M
}

// WindowDelete takes Member off each recipient's party window.
type WindowDelete[M Member] struct {
	To     []M
	Member M
}

// WindowDeleteAll clears To's party window.
type WindowDeleteAll[M Member] struct{ To M }

// Msg is a party system message; Name fills its one parameter, if any.
type Msg[M Member] struct {
	To   []M
	ID   MessageID
	Name string
}

// InfoRefresh resends Member's full view to itself and its observers.
type InfoRefresh[M Member] struct{ Member M }

// IconRefresh shows To, the members of the party Member just joined or
// formed, Member's effect icons.
type IconRefresh[M Member] struct {
	To     []M
	Member M
}

// FusionStop ends Member's own fusion channel and every fusion channel
// held on Member.
type FusionStop[M Member] struct{ Member M }

// ChannelOpen shows the command channel window.
type ChannelOpen[M Member] struct{ To []M }

// ChannelClose closes the command channel window.
type ChannelClose[M Member] struct{ To []M }

// ChannelPartyUpdate adds (Added) or removes the party Leader leads, of
// Count members, on each recipient's command channel window.
type ChannelPartyUpdate[M Member] struct {
	To     []M
	Leader M
	Count  int
	Added  bool
}

// LeaderChanged reports Leader as its party's new leader, once every
// member has been told.
type LeaderChanged[M Member] struct{ Leader M }

// Formed reports a new party; its members' positions start being shared.
type Formed struct{ ID ID }

// Dispersed reports a party that no longer exists.
type Dispersed struct{ ID ID }

func (WindowAll[M]) isNotice()          {}
func (WindowAdd[M]) isNotice()          {}
func (WindowDelete[M]) isNotice()       {}
func (WindowDeleteAll[M]) isNotice()    {}
func (Msg[M]) isNotice()                {}
func (InfoRefresh[M]) isNotice()        {}
func (IconRefresh[M]) isNotice()        {}
func (FusionStop[M]) isNotice()         {}
func (ChannelOpen[M]) isNotice()        {}
func (ChannelClose[M]) isNotice()       {}
func (ChannelPartyUpdate[M]) isNotice() {}
func (LeaderChanged[M]) isNotice()      {}
func (Formed) isNotice()                {}
func (Dispersed) isNotice()             {}
