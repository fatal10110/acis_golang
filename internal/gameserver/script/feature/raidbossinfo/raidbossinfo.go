// Package raidbossinfo is the raid information officers, whose pages list
// the raid bosses by level and mark the one a player picks on its radar.
package raidbossinfo

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// officers are the raid information officers.
func officers() []int32 {
	return []int32{
		31729, 31730, 31731, 31732, 31733, 31734, 31735, 31736, 31737, 31738,
		31775, 31776, 31777, 31778, 31779, 31780, 31781, 31782, 31783, 31784,
		31785, 31786, 31787, 31788, 31789, 31790, 31791, 31792, 31793, 31794,
		31795, 31796, 31797, 31798, 31799, 31800, 31801, 31802, 31803, 31804,
		31805, 31806, 31807, 31808, 31809, 31810, 31811, 31812, 31813, 31814,
		31815, 31816, 31817, 31818, 31819, 31820, 31821, 31822, 31823, 31824,
		31825, 31826, 31827, 31828, 31829, 31830, 31831, 31832, 31833, 31834,
		31835, 31836, 31837, 31838, 31839, 31840, 31841, 31991, 31992, 31993,
		31994, 31995,
	}
}

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: officers()},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent marks the raid boss an id names on the player's radar: where
// its spawn places it, else where a live one of it stands; with neither,
// nothing is marked. Either answers nothing. Any other event is the page it
// names.
func onAdvEvent(s *script.Script, e script.Event) string {
	if !script.IsDigit(e.Name) {
		return e.Name
	}
	// An id outside the int32 range aborts the event here.
	id, err := strconv.ParseInt(e.Name, 10, 32)
	if err != nil {
		panic(err)
	}
	raidID := int32(id)
	if spawn := s.Spawner().FindSpawn(raidID); spawn != nil {
		at := spawn.DrawSpawnLocation()
		e.Player.AddRadarMarker(int32(at.X), int32(at.Y), int32(at.Z))
	} else if raid := s.WorldNPC(raidID); raid != nil {
		at := script.LocOf(raid)
		e.Player.AddRadarMarker(int32(at.X), int32(at.Y), int32(at.Z))
	}
	return ""
}

func onTalk(*script.Script, script.Talk) string {
	return "info.htm"
}
