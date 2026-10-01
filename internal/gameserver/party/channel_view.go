package party

// ChannelView is a copy of one command channel.
type ChannelView[M Member] struct {
	// Leader leads the channel.
	Leader M
	// Members are every member of every party in the channel, party by
	// party in the order the parties joined, each party's in join order.
	Members []M
	// MembersCount counts the members of every party in the channel.
	MembersCount int
	// Parties are the channel's parties in the order they joined.
	Parties []ChannelPartyView[M]
}

// ChannelPartyView is one party of a command channel.
type ChannelPartyView[M Member] struct {
	Leader M
	Count  int
}

// Channel returns a copy of the command channel memberID's party is in.
func (r *Registry[M]) Channel(memberID int32) (ChannelView[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil || g.channel == nil {
		return ChannelView[M]{}, false
	}
	c := g.channel
	view := ChannelView[M]{
		Leader:  c.leader,
		Members: c.members(),
		Parties: make([]ChannelPartyView[M], len(c.parties)),
	}
	for i, p := range c.parties {
		view.Parties[i] = ChannelPartyView[M]{Leader: p.leader, Count: len(p.members)}
		view.MembersCount += len(p.members)
	}
	return view, true
}
