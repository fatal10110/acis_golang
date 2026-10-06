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
