// Package clan is the village masters' clan management dialog: founding,
// raising, dissolving and restoring a clan, delegating its leadership and
// founding its sub-units, with the leader-only pages refused to members.
package clan

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir: "feature",
		// The village masters.
		Bind: script.Bindings{script.EventTalked: {
			30026, 30031, 30037, 30066, 30070, 30109, 30115, 30120, 30154, 30174, 30175, 30176, 30187, 30191,
			30195, 30288, 30289, 30290, 30297, 30358, 30373, 30462, 30474, 30498, 30499, 30500, 30503, 30504,
			30505, 30508, 30511, 30512, 30513, 30520, 30525, 30565, 30594, 30595, 30676, 30677, 30681, 30685,
			30687, 30689, 30694, 30699, 30704, 30845, 30847, 30849, 30854, 30857, 30862, 30865, 30894, 30897,
			30900, 30905, 30910, 30913, 31269, 31272, 31276, 31279, 31285, 31288, 31314, 31317, 31321, 31324,
			31326, 31328, 31331, 31334, 31336, 31755, 31958, 31961, 31965, 31968, 31974, 31977, 31996, 32092,
			32093, 32094, 32095, 32096, 32097, 32098,
		}},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent opens the page the link names; a player who leads no clan
// asking for a leader's page gets its refusal page instead.
func onAdvEvent(_ *script.Script, e script.Event) string {
	switch e.Name {
	case "9000-03.htm":
		if !e.Player.IsClanLeader() {
			return "9000-03-no.htm"
		}
	case "9000-04.htm":
		if !e.Player.IsClanLeader() {
			return "9000-04-no.htm"
		}
	case "9000-05.htm":
		if !e.Player.IsClanLeader() {
			return "9000-05-no.htm"
		}
	case "9000-07.htm", "9000-08.htm", "9000-12a.htm", "9000-13a.htm", "9000-13b.htm", "9000-14a.htm", "9000-15.htm":
		if !e.Player.IsClanLeader() {
			return "9000-07-no.htm"
		}
	}
	return e.Name
}

func onTalk(*script.Script, script.Talk) string {
	return "9000-01.htm"
}
