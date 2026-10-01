package party

// RewardGroup is the group a party member shares a kill with: its party's
// members, or every member of its command channel party by party while the
// party is in one, with the channel's level.
type RewardGroup[M Member] struct {
	Members      []M
	InChannel    bool
	ChannelLevel int
}

// RewardGroup returns the group the player shares a kill with; ok is false
// when the player is in no party.
func (r *Registry[M]) RewardGroup(memberID int32) (RewardGroup[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil {
		return RewardGroup[M]{}, false
	}
	if c := g.channel; c != nil {
		return RewardGroup[M]{Members: c.members(), InChannel: true, ChannelLevel: c.level}, true
	}
	return RewardGroup[M]{Members: append([]M(nil), g.members...)}, true
}
