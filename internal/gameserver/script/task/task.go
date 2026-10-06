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

// ClanLadderRefresh ranks the clans again by reputation.
func ClanLadderRefresh() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnStart: func(_ *script.Script, e script.Start) {
			e.Server.RefreshClanLadder()
		},
	}}
}

// RecommendationUpdate gives every player its daily recommendations.
func RecommendationUpdate() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnStart: func(_ *script.Script, e script.Start) {
			e.Server.RefreshRecommendations(e.Ctx)
		},
	}}
}

// minRewardedClanLevel is the clan level from which a clan's members earn
// it reputation by their raid point places.
const minRewardedClanLevel = 5

// RaidPointReset turns the raid point ranking into clan reputation: each
// clan of level 5 or more gains the reputation of its members' places
// among the first 100 players, then every raid point is wiped.
func RaidPointReset() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnStart: func(_ *script.Script, e script.Start) {
			var clans []int32
			rewards := map[int32]int{}
			for i, objectID := range e.Server.RaidPointWinners() {
				clanID, level, ok := e.Server.MemberClan(objectID)
				if !ok || level < minRewardedClanLevel {
					continue
				}
				if _, seen := rewards[clanID]; !seen {
					clans = append(clans, clanID)
				}
				rewards[clanID] += placeReputation(i + 1)
			}
			for _, clanID := range clans {
				e.Server.AddClanReputation(clanID, rewards[clanID])
			}
			e.Server.CleanUpRaidPoints()
		},
	}}
}

// placeReputation is the clan reputation a member's place in the raid
// point ranking earns.
func placeReputation(place int) int {
	switch place {
	case 1:
		return 1250
	case 2:
		return 900
	case 3:
		return 700
	case 4:
		return 600
	case 5:
		return 450
	case 6:
		return 350
	case 7:
		return 300
	case 8:
		return 200
	case 9:
		return 150
	case 10:
		return 100
	}
	if place <= 50 {
		return 25
	}
	return 12
}
