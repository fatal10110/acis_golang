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
