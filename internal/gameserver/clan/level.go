package clan

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// reputationLevel is the clan level from which a clan earns reputation; a
// leader is told so when its clan reaches the next level.
const reputationLevel = 4

// LevelPayer pays a clan level's price out of its leader's SP, adena and
// items, reporting each payment to the leader as it happens.
type LevelPayer interface {
	SP() int
	// PayAdena takes count adena, or reports the shortfall; it reports
	// whether it paid.
	PayAdena(count int) bool
	// PayItem takes count of itemID, or reports the shortfall; it reports
	// whether it paid.
	PayItem(itemID int32, count int) bool
	// TakeSP removes sp from the leader.
	TakeSP(sp int)
}

// levelPrice is what raising a clan from one level to the next costs.
type levelPrice struct {
	sp, adena  int
	itemID     int32
	reputation int
	members    int
}

var levelPrices = map[int]levelPrice{
	0: {sp: 30000, adena: 650000},
	1: {sp: 150000, adena: 2500000},
	2: {sp: 500000, itemID: 1419},  // Blood Mark
	3: {sp: 1400000, itemID: 3874}, // Alliance Manifesto
	4: {sp: 3500000, itemID: 3870}, // Seal of Aspiration
	5: {reputation: 10000, members: 30},
	6: {reputation: 20000, members: 80},
	7: {reputation: 40000, members: 120},
}

// Level-up notices, in the order they happen.
type (
	// LevelNotLeader refuses a level-up asked by anyone but the leader.
	LevelNotLeader struct{}
	// LevelDissolving refuses a level-up while a dissolution is pending.
	LevelDissolving struct{}
	// LevelFailed reports a price the leader could not pay.
	LevelFailed struct{}
	// ReputationChanged reports the clan's new reputation score; Crossed
	// is +1 when it rose above 0, -1 when it fell to 0 or below, else 0.
	ReputationChanged struct {
		Score   int
		Crossed int
	}
	// ReputationDeducted names the reputation a level-up took.
	ReputationDeducted struct{ Points int }
	// LevelRaised reports the clan's new level.
	LevelRaised struct{ Level int }
)

// RaiseLevel raises c's clan by one level, c paying the price through pay.
func (s *Service) RaiseLevel(c *player.Character, pay LevelPayer, now time.Time) []any {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return []any{LevelNotLeader{}}
	}
	info := cl.Info()
	if now.UnixMilli() < info.DissolvingExpiry {
		return []any{LevelDissolving{}}
	}
	price, ok := levelPrices[info.Level]
	if !ok {
		return []any{LevelFailed{}}
	}
	if price.reputation > 0 {
		return s.raiseLevelForReputation(cl, info.Level, price)
	}
	if pay.SP() < price.sp {
		return []any{LevelFailed{}}
	}
	paid := false
	if price.itemID != 0 {
		paid = pay.PayItem(price.itemID, 1)
	} else {
		paid = pay.PayAdena(price.adena)
	}
	if !paid {
		return []any{LevelFailed{}}
	}
	pay.TakeSP(price.sp)
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return []any{LevelRaised{Level: s.setLevelLocked(cl, info.Level+1)}}
}

// raiseLevelForReputation raises cl from level, which costs reputation.
// The price check, the deduction and the new level happen under one hold
// of cl.mu, so no other reputation change slips between them.
func (s *Service) raiseLevelForReputation(cl *Clan, level int, price levelPrice) []any {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.level != level || cl.reputation < price.reputation || len(cl.members) < price.members {
		return []any{LevelFailed{}}
	}
	var notices []any
	if change, changed := s.addReputationLocked(cl, -price.reputation); changed {
		notices = append(notices, change)
	}
	notices = append(notices, ReputationDeducted{Points: price.reputation})
	return append(notices, LevelRaised{Level: s.setLevelLocked(cl, level+1)})
}

// setLevelLocked stores level as cl's level; cl.mu is held.
func (s *Service) setLevelLocked(cl *Clan, level int) int {
	cl.level = level
	s.write(cl.id, "update clan level", func(ctx context.Context, st Store) error { return st.UpdateLevel(ctx, cl.id, level) })
	return level
}

// TellsLeaderAboutReputation reports whether reaching level earns the
// leader the reputation notice.
func TellsLeaderAboutReputation(level int) bool { return level > reputationLevel }

// addReputationLocked moves cl's reputation by delta; cl.mu is held. A
// clan below level 5 neither gains nor loses any; it reports no change
// then.
func (s *Service) addReputationLocked(cl *Clan, delta int) (ReputationChanged, bool) {
	if cl.level < minReputationLevel {
		return ReputationChanged{}, false
	}
	before := cl.reputation
	after := clampReputation(before + delta)
	cl.reputation = after
	change := ReputationChanged{Score: after}
	switch {
	case before > 0 && after <= 0:
		change.Crossed = -1
	case after > 0 && before <= 0:
		change.Crossed = 1
	}
	s.write(cl.id, "update clan reputation", func(ctx context.Context, st Store) error {
		return st.UpdateReputation(ctx, cl.id, after)
	})
	return change, true
}
