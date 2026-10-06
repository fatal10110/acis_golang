// Package task contains the scheduled tasks: scripts whose start hook the
// schedule runner calls at the times their scripts.xml entry gives.
package task

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// CastleTaxRefresh closes every castle's tax period.
func CastleTaxRefresh() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnStart: func(_ *script.Script, e script.Start) {
			e.Server.UpdateCastleTaxes()
		},
	}}
}

// ClanLeaderTransfer hands every clan with a pending leader nomination to
// its nominee.
func ClanLeaderTransfer() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnStart: func(_ *script.Script, e script.Start) {
			e.Server.TransferClanLeaders()
		},
	}}
}

// SevenSignsUpdate stores the festival scores, except in the seal
// validation period, then the Seven Signs sign-ups and status.
func SevenSignsUpdate() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnStart: func(_ *script.Script, e script.Start) {
			if !e.Server.SealValidationPeriod() {
				e.Server.SaveFestivalScores(e.Ctx)
			}
			e.Server.SaveSevenSigns(e.Ctx)
		},
	}}
}
