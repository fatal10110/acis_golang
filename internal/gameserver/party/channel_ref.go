package party

// ChannelRef names one command channel for as long as it is held, even
// once the channel disbands: two refs are equal exactly when they name the
// same channel.
type ChannelRef[M Member] struct {
	r *Registry[M]
	c *channel[M]
}

// ChannelRef returns the command channel memberID's party is in.
func (r *Registry[M]) ChannelRef(memberID int32) (ChannelRef[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil || g.channel == nil {
		return ChannelRef[M]{}, false
	}
	return ChannelRef[M]{r: r, c: g.channel}, true
}

// Leader returns the channel's current leader; a disbanded channel keeps
// the one it last had.
func (ref ChannelRef[M]) Leader() M {
	ref.r.mu.Lock()
	defer ref.r.mu.Unlock()
	return ref.c.leader
}

// MembersCount counts the members of every party in the channel; a
// disbanded channel has none.
func (ref ChannelRef[M]) MembersCount() int {
	ref.r.mu.Lock()
	defer ref.r.mu.Unlock()
	n := 0
	for _, g := range ref.c.parties {
		n += len(g.members)
	}
	return n
}
