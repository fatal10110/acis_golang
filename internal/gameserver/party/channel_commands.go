package party

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
