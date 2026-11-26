package party

// Command channel authority: the clan level a channel's former must lead,
// the clan skill that is authority enough, and the item that otherwise is.
const (
	channelClanLevel    = 5
	clanImperiumSkillID = 391
	strategyGuideItemID = 8871
)

// ChannelFormer is a player as command channel authority judges it.
type ChannelFormer interface {
	HasSkill(skillID int) bool
}

// ChannelAuthority is whether a player may form a command channel.
type ChannelAuthority uint8

// The command channel authority outcomes, the refusals in the order they
// are checked.
const (
	ChannelAuthorized ChannelAuthority = iota
	// ChannelNotClanLeader refuses a player that leads no clan of level 5
	// or more.
	ChannelNotClanLeader
	// ChannelNoStrategyGuide refuses such a leader holding neither Clan
	// Imperium nor a Strategy Guide.
	ChannelNoStrategyGuide
)

// FormAuthority judges whether f may form a command channel. ledClanLevel
// is the level of the clan f leads, 0 when it leads none: only the leader
// of a clan of level 5 or more may, and then by holding Clan Imperium or a
// Strategy Guide. guide is asked last, with the guide's item id: it
// reports f holds one, or, as the channel forms, destroys one, reporting
// whether it did.
func FormAuthority(f ChannelFormer, ledClanLevel int, guide func(itemID int32) bool) ChannelAuthority {
	switch {
	case ledClanLevel < channelClanLevel:
		return ChannelNotClanLeader
	case f.HasSkill(clanImperiumSkillID), guide(strategyGuideItemID):
		return ChannelAuthorized
	}
	return ChannelNoStrategyGuide
}
