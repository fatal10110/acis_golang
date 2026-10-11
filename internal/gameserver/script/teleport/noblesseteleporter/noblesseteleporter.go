// Package noblesseteleporter is the gatekeepers' Noblesse-only teleport:
// a noblesse gets the page leading to the hunting-ground destinations,
// with or without the Noblesse Gate Pass; anyone else the refusal.
package noblesseteleporter

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir: "teleport",
		// The gatekeepers.
		Bind: script.Bindings{script.EventTalked: {
			30006, 30059, 30080, 30134, 30146, 30177, 30233, 30256, 30320, 30540, 30576, 30836, 30848, 30878,
			30899, 31275, 31320, 31964,
		}},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent opens the gatekeeper's list of the destinations of the
// teleport type the event names, ignoring case; an event naming no type
// aborts. It then answers as the base script does: nothing.
func onAdvEvent(s *script.Script, e script.Event) string {
	kind, err := travel.ParseKind(strings.ToUpper(e.Name))
	if err != nil {
		panic(err)
	}
	e.NPC.ShowTeleportWindow(e.Player, kind)
	var base script.Hooks
	return base.Event(s, e)
}

func onTalk(_ *script.Script, e script.Talk) string {
	if e.Player.IsNoble() {
		return "noble.htm"
	}
	return "nobleteleporter-no.htm"
}
