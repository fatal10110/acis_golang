package player

import "sync"

const (
	// maxRecommendationsHave caps the recommendations a character holds.
	maxRecommendationsHave = 255
	// maxRecommendationsLeft caps the recommendations a character can give.
	maxRecommendationsLeft = 9
	// minRecommendLevel is the level a character needs to recommend.
	minRecommendLevel = 10
)

// recommendationState is a character's recommendation counters and the
// characters it recommended since the last daily refresh. mu guards all
// three: the character's own queue writes left and recommended, another
// player's queue raises have, and CharInfo builders on other queues read
// both counters. Never hold two characters' locks at once.
type recommendationState struct {
	mu          sync.Mutex
	have        int
	left        int
	recommended map[int32]struct{}
}

// RecommendRefusal is why a recommendation was refused.
type RecommendRefusal int

// Recommendation refusals, in the order they are checked.
const (
	RecommendAccepted RecommendRefusal = iota
	// RecommendSelf: a character cannot recommend itself.
	RecommendSelf
	// RecommendLevelTooLow: the recommender is below level 10.
	RecommendLevelTooLow
	// RecommendNoneLeft: the recommender has no recommendation left today.
	RecommendNoneLeft
	// RecommendTargetFull: the target already holds 255.
	RecommendTargetFull
	// RecommendAlreadyGiven: the recommender already recommended the
	// target since the last daily refresh.
	RecommendAlreadyGiven
)

// RecommendationsHave reports how many recommendations c holds.
func (c *Character) RecommendationsHave() int {
	c.recommendations.mu.Lock()
	defer c.recommendations.mu.Unlock()
	return c.recommendations.have
}

// RecommendationsLeft reports how many recommendations c can still give.
func (c *Character) RecommendationsLeft() int {
	c.recommendations.mu.Lock()
	defer c.recommendations.mu.Unlock()
	return c.recommendations.left
}

// SetRecommendationCounts sets c's persisted counters, each clamped to its
// range.
func (c *Character) SetRecommendationCounts(have, left int) {
	r := &c.recommendations
	r.mu.Lock()
	defer r.mu.Unlock()
	r.have = min(max(have, 0), maxRecommendationsHave)
	r.left = min(max(left, 0), maxRecommendationsLeft)
}

// RestoreRecommended sets the characters c recommended since the last daily
// refresh.
func (c *Character) RestoreRecommended(ids []int32) {
	r := &c.recommendations
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recommended = make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		r.recommended[id] = struct{}{}
	}
}

// CheckRecommend reports whether c may recommend target now.
func (c *Character) CheckRecommend(target *Character) RecommendRefusal {
	switch {
	case c == target:
		return RecommendSelf
	case c.Level() < minRecommendLevel:
		return RecommendLevelTooLow
	case c.RecommendationsLeft() <= 0:
		return RecommendNoneLeft
	case target.RecommendationsHave() >= maxRecommendationsHave:
		return RecommendTargetFull
	case c.hasRecommended(target.ID):
		return RecommendAlreadyGiven
	}
	return RecommendAccepted
}

func (c *Character) hasRecommended(id int32) bool {
	c.recommendations.mu.Lock()
	defer c.recommendations.mu.Unlock()
	_, ok := c.recommendations.recommended[id]
	return ok
}

// Recommend gives target one recommendation from c, after CheckRecommend
// accepted it: target's count rises (capped at 255), c's remaining count
// falls, and target is remembered until the next daily refresh. It returns
// both new counts, for persistence.
func (c *Character) Recommend(target *Character) (targetHave, left int) {
	targetHave = target.addRecommendationsHave(1)
	r := &c.recommendations
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.left > 0 {
		r.left--
	}
	if r.recommended == nil {
		r.recommended = make(map[int32]struct{})
	}
	r.recommended[target.ID] = struct{}{}
	return targetHave, r.left
}

func (c *Character) addRecommendationsHave(delta int) int {
	r := &c.recommendations
	r.mu.Lock()
	defer r.mu.Unlock()
	r.have = min(max(r.have+delta, 0), maxRecommendationsHave)
	return r.have
}

// DailyRecommendations returns what the daily refresh gives a character of
// level: the recommendations it can give, and how many it loses of those it
// holds.
func DailyRecommendations(level int) (left, loss int) {
	switch {
	case level < 20:
		return 3, 1
	case level < 40:
		return 6, 2
	default:
		return 9, 3
	}
}

// RefreshDailyRecommendations applies the daily refresh to c at its current
// level: it forgets whom it recommended, resets what it can give, and loses
// part of what it holds (never below zero).
func (c *Character) RefreshDailyRecommendations() {
	left, loss := DailyRecommendations(c.Level())
	r := &c.recommendations
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recommended = nil
	r.left = left
	r.have = max(r.have-loss, 0)
}
