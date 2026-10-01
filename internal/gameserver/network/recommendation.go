package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// recommendationStore reads and writes who recommended whom and the
// characters' recommendation counters.
type recommendationStore interface {
	ListRecommended(ctx context.Context, charID int32) ([]int32, error)
	Give(ctx context.Context, giverID, targetID int32, targetHave, giverLeft int) error
	RefreshDaily(ctx context.Context) error
}

// Recommendation messages.
const (
	systemMessageSelectTarget                = 242  // Select target.
	systemMessageCannotRecommendYourself     = 829  // You cannot recommend yourself.
	systemMessageRecommendedS1LeftS2         = 830  // You have recommended $s1. You have $s2 recommendations left.
	systemMessageRecommendedByS1             = 831  // You have been recommended by $s1.
	systemMessageAlreadyRecommended          = 832  // That character has already been recommended.
	systemMessageNoRecommendationsLeft       = 833  // You are not authorized to make further recommendations at this time.
	systemMessageRecommendLevelTooLow        = 898  // Only characters of level 10 or above are authorized to make recommendations.
	systemMessageTargetRecommendationsCapped = 1188 // Your target cannot receive any more recommendations.
)

// restoreRecommended loads whom c recommended since the last daily refresh.
// A failed read leaves the record empty and is logged.
func (l *GameClientLink) restoreRecommended(ctx context.Context, c *player.Character) {
	if l.recommendations == nil {
		return
	}
	ids, err := l.recommendations.ListRecommended(ctx, c.ID)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: list recommendations")
		return
	}
	c.RestoreRecommended(ids)
}

// evaluate answers RequestEvaluate: live recommends the online player it
// has selected. A target that is not live's current selection is ignored
// without an answer, as the reference does: the request comes from a chat
// command or the target window's button, which registers no pending client
// action, and nothing about the selection changed.
func (l *GameClientLink) evaluate(live *livePlayer, req clientpackets.RequestEvaluate) {
	if live == nil {
		return
	}
	target, ok := l.livePlayerByID(req.TargetID)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(systemMessageSelectTarget))
		return
	}
	if live.Target() != world.Tracked(target) {
		return
	}
	if msg, refused := recommendRefusalMessage(live.CheckRecommend(target.Character)); refused {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		return
	}
	targetHave, left := live.Recommend(target.Character)
	if l.recommendations != nil {
		targetID := target.ObjectID()
		l.queueRowWrite(live.ObjectID(), "save recommendation", func(ctx context.Context, ownerID int32) error {
			return l.recommendations.Give(ctx, ownerID, targetID, targetHave, left)
		})
	}
	live.SendFrame(serverpackets.FrameSystemMessageParams(systemMessageRecommendedS1LeftS2,
		serverpackets.TextParam(target.Name), serverpackets.NumberParam(int32(left))))
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))

	// The target's own view and its observers' are refreshed on the
	// target's queue, which owns what UserInfo and CharInfo read.
	giver := live.Name
	postLive(target, func() {
		target.SendFrame(serverpackets.FrameSystemMessageString(systemMessageRecommendedByS1, giver))
		if l.liveInWorld(target) {
			l.broadcastCharacterInfo(target)
		}
	})
}

func recommendRefusalMessage(r player.RecommendRefusal) (int, bool) {
	switch r {
	case player.RecommendSelf:
		return systemMessageCannotRecommendYourself, true
	case player.RecommendLevelTooLow:
		return systemMessageRecommendLevelTooLow, true
	case player.RecommendNoneLeft:
		return systemMessageNoRecommendationsLeft, true
	case player.RecommendTargetFull:
		return systemMessageTargetRecommendationsCapped, true
	case player.RecommendAlreadyGiven:
		return systemMessageAlreadyRecommended, true
	}
	return 0, false
}

// RefreshDailyRecommendations runs the daily recommendation refresh: every
// online player, on its own queue, forgets whom it recommended, gets its
// recommendations to give back by level, loses part of those it holds, and
// is sent its UserInfo; then every stored character gets the same refresh
// from its stored level. It is the body of the daily recommendation job,
// whose schedule belongs to the scheduled-script runtime.
func (l *GameClientLink) RefreshDailyRecommendations(ctx context.Context) error {
	if l.world != nil {
		for _, p := range l.world.Players() {
			live, ok := p.(*livePlayer)
			if !ok {
				continue
			}
			postLive(live, func() {
				live.RefreshDailyRecommendations()
				live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
			})
		}
	}
	if l.recommendations == nil {
		return nil
	}
	return l.recommendations.RefreshDaily(ctx)
}
