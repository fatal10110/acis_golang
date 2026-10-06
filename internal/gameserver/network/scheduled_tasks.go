package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// scheduledSaveTimeout bounds one database save a scheduled task makes, so a
// hung database cannot hold the schedule's queue.
const scheduledSaveTimeout = 30 * time.Second

var _ script.Server = (*GameClientLink)(nil)

// UpdateCastleTaxes closes every castle's tax period
// (castle.Manager.UpdateTaxes).
func (l *GameClientLink) UpdateCastleTaxes() {
	if l.castles != nil {
		l.castles.UpdateTaxes()
	}
}

// SealValidationPeriod reports whether the Seven Signs are in their seal
// validation period.
func (l *GameClientLink) SealValidationPeriod() bool {
	return l.sevenSigns != nil && l.sevenSigns.CurrentPeriod() == sevensigns.SealValidation
}

// SaveFestivalScores stores the festival's scores, logging a failure.
func (l *GameClientLink) SaveFestivalScores(ctx context.Context) {
	if l.festival == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, scheduledSaveTimeout)
	defer cancel()
	if err := l.festival.SaveScores(ctx); err != nil {
		l.log.Error().Err(err).Msg("scheduled save: festival scores")
	}
}

// SaveSevenSigns stores the Seven Signs sign-ups and status
// (sevensigns.State.Save), logging a failure.
func (l *GameClientLink) SaveSevenSigns(ctx context.Context) {
	if l.sevenSigns == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, scheduledSaveTimeout)
	defer cancel()
	if err := l.sevenSigns.Save(ctx); err != nil {
		l.log.Error().Err(err).Msg("scheduled save: seven signs")
	}
}

// RefreshClanLadder ranks the clans again by reputation
// (clan.Table.RefreshLadder).
func (l *GameClientLink) RefreshClanLadder() {
	l.clanService().Table().RefreshLadder()
}

// RefreshRecommendations runs the daily recommendation refresh
// (RefreshDailyRecommendations), logging a failure. Its stored half holds
// every persistence lane while it runs, so it gets scheduledSaveTimeout: a
// hung database cannot stall every save past that.
func (l *GameClientLink) RefreshRecommendations(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, scheduledSaveTimeout)
	defer cancel()
	if err := l.RefreshDailyRecommendations(ctx); err != nil {
		l.log.Error().Err(err).Msg("scheduled refresh: recommendations")
	}
}

// RaidPointWinners returns the first 100 players of the raid point
// ranking (raidpoint.Points.Winners).
func (l *GameClientLink) RaidPointWinners() []int32 {
	if l.raidPoints == nil {
		return nil
	}
	return l.raidPoints.Winners()
}

// MemberClan returns the id and level of the clan objectID is a member of.
func (l *GameClientLink) MemberClan(objectID int32) (int32, int, bool) {
	cl, ok := l.clanService().Table().MemberClan(objectID)
	if !ok {
		return 0, 0, false
	}
	return cl.ID(), cl.Level(), true
}

// AddClanReputation adds points to clan clanID's reputation and shows its
// members in the world the new score, as any reputation change does.
func (l *GameClientLink) AddClanReputation(clanID int32, points int) {
	cl, ok := l.clanService().Table().Get(clanID)
	if !ok {
		return
	}
	if change, changed := l.clanService().AddReputation(cl, points); changed {
		l.sendReputationChange(cl, change, nil)
	}
}

// CleanUpRaidPoints forgets and clears every player's raid points
// (raidpoint.Points.CleanUp).
func (l *GameClientLink) CleanUpRaidPoints() {
	if l.raidPoints != nil {
		l.raidPoints.CleanUp()
	}
}
