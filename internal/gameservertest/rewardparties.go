package gameservertest

import gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"

// WithRewardParties wraps the resolver the suite's hostiles share a kill's
// exp and sp through (default: the link's own parties), so a suite can
// reach a group shape no packet forms yet, such as a command channel,
// whose formation needs a clan.
func WithRewardParties(wrap func(link gamemanager.RewardParties) gamemanager.RewardParties) Option {
	return func(o *options) { o.rewardPartiesWrap = wrap }
}

func (o *options) rewardParties(link gamemanager.RewardParties) gamemanager.RewardParties {
	if o.rewardPartiesWrap == nil {
		return link
	}
	return o.rewardPartiesWrap(link)
}

// WithLootChannels wraps the resolver the suite's raid bosses find the
// command channel winning their loot rights through (default: the link's
// own channels), so a suite can reach a channel larger than any packet
// forms cheaply.
func WithLootChannels(wrap func(link gamemanager.LootChannels) gamemanager.LootChannels) Option {
	return func(o *options) { o.lootChannelsWrap = wrap }
}

func (o *options) lootChannels(link gamemanager.LootChannels) gamemanager.LootChannels {
	if o.lootChannelsWrap == nil {
		return link
	}
	return o.lootChannelsWrap(link)
}
