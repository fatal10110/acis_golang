package party

// channel is a command channel: parties joined under the leader of the one
// that formed it. Registry.mu guards it.
type channel[M Member] struct {
	leader  M
	parties []*group[M]
	level   int
}

// ChannelInviteStatus is the outcome of ChannelInvite.
type ChannelInviteStatus uint8

// Channel invitation statuses.
const (
	// ChannelInviteIgnored: either side has no party, or both are in the
	// same one.
	ChannelInviteIgnored ChannelInviteStatus = iota
	// ChannelInviteNotLeader: the requester leads neither its party nor its
	// party's channel.
	ChannelInviteNotLeader
	// ChannelInviteTargetInChannel: the target's party is already in a
	// channel.
	ChannelInviteTargetInChannel
	// ChannelInviteReady: the invitation goes to the returned leader of the
	// target's party.
	ChannelInviteReady
)

// ChannelInvite checks an invitation from requesterID's party to
// targetID's party into a command channel.
func (r *Registry[M]) ChannelInvite(requesterID, targetID int32) (ChannelInviteStatus, M) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var zero M
	rp, tp := r.byMember[requesterID], r.byMember[targetID]
	if rp == nil || tp == nil || rp.leader.ObjectID() == tp.leader.ObjectID() {
		return ChannelInviteIgnored, zero
	}
	if rp.leader.ObjectID() != requesterID || (rp.channel != nil && rp.channel.leader.ObjectID() != requesterID) {
		return ChannelInviteNotLeader, zero
	}
	if tp.channel != nil {
		return ChannelInviteTargetInChannel, zero
	}
	return ChannelInviteReady, tp.leader
}

// ChannelAnswerForms reports, for an accepted channel invitation, whether
// both parties still exist (ok) and whether the answer would form a new
// channel rather than join the requester's, which the caller must
// authorize first.
func (r *Registry[M]) ChannelAnswerForms(requesterID, targetID int32) (forms, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rp, tp := r.byMember[requesterID], r.byMember[targetID]
	if rp == nil || tp == nil {
		return false, false
	}
	return rp.channel == nil, true
}

// JoinChannel puts target's party into requester's party's channel,
// forming one when there is none. A target party already in a channel, or
// the requester's own, joins nothing.
func (r *Registry[M]) JoinChannel(requester, target M) []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	rp, tp := r.byMember[requester.ObjectID()], r.byMember[target.ObjectID()]
	if rp == nil || tp == nil || rp == tp || tp.channel != nil {
		return nil
	}
	var out notices
	if rp.channel == nil {
		formChannel(&out, rp, tp)
	} else {
		rp.channel.add(&out, tp)
	}
	return out
}

// OustStatus is the outcome of Oust.
type OustStatus uint8

// Oust statuses.
const (
	OustDone OustStatus = iota
	// OustInvalidTarget: either side has no party, or the target's party is
	// not in the requester's channel.
	OustInvalidTarget
	// OustNotAuthorized: the requester leads no channel.
	OustNotAuthorized
)

// Oust dismisses targetID's party from the channel requester leads.
func (r *Registry[M]) Oust(requester M, targetID int32) (OustStatus, []Notice) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rp, tp := r.byMember[requester.ObjectID()], r.byMember[targetID]
	if rp == nil || tp == nil {
		return OustInvalidTarget, nil
	}
	c := rp.channel
	if c == nil || c.leader.ObjectID() != requester.ObjectID() {
		return OustNotAuthorized, nil
	}
	var out notices
	if !c.remove(&out, tp) {
		return OustInvalidTarget, nil
	}
	out.add(Msg[M]{To: tp.members, ID: MsgDismissedFromChannel})
	if rp.channel != nil {
		out.add(Msg[M]{To: rp.channel.members(), ID: MsgPartyDismissedFromChannel, Name: tp.leader.CharacterName()})
	}
	return OustDone, out
}

func formChannel[M Member](out *notices, requester, target *group[M]) {
	c := &channel[M]{leader: requester.leader, parties: []*group[M]{requester, target}}
	requester.channel = c
	target.channel = c
	c.recalculateLevel()
	out.add(Msg[M]{To: requester.members, ID: MsgChannelFormed})
	out.add(ChannelOpen[M]{To: requester.members})
	out.add(Msg[M]{To: target.members, ID: MsgJoinedChannel})
	out.add(ChannelOpen[M]{To: target.members})
}

func (c *channel[M]) add(out *notices, g *group[M]) {
	out.add(ChannelPartyUpdate[M]{To: c.members(), Leader: g.leader, Count: len(g.members), Added: true})
	c.parties = append(c.parties, g)
	if g.level > c.level {
		c.level = g.level
	}
	g.channel = c
	out.add(Msg[M]{To: g.members, ID: MsgJoinedChannel})
	out.add(ChannelOpen[M]{To: g.members})
}

// remove takes g out of c, disbanding c when only two parties were in it,
// and reports whether g was in c.
func (c *channel[M]) remove(out *notices, g *group[M]) bool {
	i := c.index(g)
	if i < 0 {
		return false
	}
	if len(c.parties) == 2 {
		c.disband(out)
		return true
	}
	c.parties = append(c.parties[:i:i], c.parties[i+1:]...)
	g.channel = nil
	out.add(ChannelClose[M]{To: g.members})
	c.recalculateLevel()
	out.add(ChannelPartyUpdate[M]{To: c.members(), Leader: g.leader, Count: len(g.members)})
	return true
}

func (c *channel[M]) disband(out *notices) {
	for _, g := range c.parties {
		g.channel = nil
		out.add(ChannelClose[M]{To: g.members})
		out.add(Msg[M]{To: g.members, ID: MsgChannelDisbanded})
	}
	c.parties = nil
}

func (c *channel[M]) members() []M {
	var out []M
	for _, g := range c.parties {
		out = append(out, g.members...)
	}
	return out
}

func (c *channel[M]) index(g *group[M]) int {
	for i, p := range c.parties {
		if p == g {
			return i
		}
	}
	return -1
}

func (c *channel[M]) recalculateLevel() {
	level := 0
	for _, g := range c.parties {
		if g.level > level {
			level = g.level
		}
	}
	c.level = level
}
