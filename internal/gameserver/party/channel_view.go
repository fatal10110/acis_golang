package party

// ChannelView is a copy of one command channel.
type ChannelView[M Member] struct {
	// Leader leads the channel.
	Leader M
	// Members are every member of every party in the channel, party by
	// party in the order the parties joined, each party's in join order.
	Members []M
}

// Channel returns the command channel the player's party is in.
func (r *Registry[M]) Channel(memberID int32) (ChannelView[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil || g.channel == nil {
		return ChannelView[M]{}, false
	}
	return ChannelView[M]{Leader: g.channel.leader, Members: g.channel.members()}, true
}
