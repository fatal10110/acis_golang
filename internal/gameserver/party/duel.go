package party

// Edited reports a change to the members of the party Leader leads: a
// member joining or leaving it, or the party breaking up. A party duel that
// party fights is cancelled.
type Edited[M Member] struct{ Leader M }

func (Edited[M]) isNotice() {}

// duellist is a member that may be in a duel.
type duellist interface{ InDuel() bool }

// inDuel reports whether m is in a duel.
func inDuel[M Member](m M) bool {
	d, ok := any(m).(duellist)
	return ok && d.InDuel()
}

// LeaveChannelSilently takes the party memberID is in out of its command
// channel, as a party duel does to both parties: the channel disbands when
// only two parties were in it, and otherwise only the window updates are
// sent. A player in no party, or a party in no channel, changes nothing.
func (r *Registry[M]) LeaveChannelSilently(memberID int32) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil || g.channel == nil {
		return nil
	}
	var out notices
	g.channel.remove(&out, g)
	return out
}
