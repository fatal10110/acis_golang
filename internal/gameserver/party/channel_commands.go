package party

// ChannelView is a copy of one command channel.
type ChannelView[M Member] struct {
	Leader M
	// MembersCount counts the members of every party in the channel.
	MembersCount int
	Parties      []ChannelPartyView[M]
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
	view := ChannelView[M]{Leader: c.leader, Parties: make([]ChannelPartyView[M], len(c.parties))}
	for i, p := range c.parties {
		view.Parties[i] = ChannelPartyView[M]{Leader: p.leader, Count: len(p.members)}
		view.MembersCount += len(p.members)
	}
	return view, true
}

// LeaveChannel takes the party leader leads out of its command channel,
// disbanding the channel when only two parties were in it. The party is
// told it left, then the parties still in the channel are told whose party
// left. A player leading no party, or a party in no channel, changes
// nothing.
func (r *Registry[M]) LeaveChannel(leader M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[leader.ObjectID()]
	if g == nil || g.leader.ObjectID() != leader.ObjectID() || g.channel == nil {
		return nil
	}
	c := g.channel
	var out notices
	c.remove(&out, g)
	out.add(Msg[M]{To: g.members, ID: MsgLeftChannel})
	if rest := c.members(); len(rest) > 0 {
		out.add(Msg[M]{To: rest, ID: MsgPartyLeftChannel, Name: leader.CharacterName()})
	}
	return out
}

// DisbandChannel disbands the command channel leader leads, both its party
// and the channel. Anyone else changes nothing.
func (r *Registry[M]) DisbandChannel(leader M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[leader.ObjectID()]
	if g == nil || g.leader.ObjectID() != leader.ObjectID() {
		return nil
	}
	c := g.channel
	if c == nil || c.leader.ObjectID() != leader.ObjectID() {
		return nil
	}
	var out notices
	c.disband(&out)
	return out
}
