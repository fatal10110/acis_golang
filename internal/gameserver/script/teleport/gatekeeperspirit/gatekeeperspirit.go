// Package gatekeeperspirit is the gatekeeper spirits of the Seven Signs
// necropolis of Avarice. The entrance spirit gives a player of the cabal
// that owns the Seal of Avarice and won the competition its cabal's page,
// anyone else its own chat window. Ten seconds after Lilith or Anakim dies
// an exit spirit appears by the raid boss's lair for fifteen minutes.
package gatekeeperspirit

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// The spirits.
const (
	enterGK = 31111
	exitGK  = 31112
)

// The raid bosses.
const (
	lilith = 25283
	anakim = 25286
)

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir: "teleport",
		Bind: script.Bindings{
			script.EventFirstTalk: {enterGK},
			script.EventTalked:    {enterGK},
			script.EventMyDying:   {lilith, anakim},
		},
		Hooks: script.Hooks{OnFirstTalk: onFirstTalk, OnTimer: onTimer, OnMyDying: onMyDying},
	}
}

func onFirstTalk(s *script.Script, e script.FirstTalk) string {
	playerCabal := s.PlayerCabal(e.Player.ObjectID())
	sealAvariceOwner := s.SealOwner(sevensigns.Avarice)
	winningCabal := s.WinningCabal()

	if playerCabal == sealAvariceOwner && playerCabal == winningCabal {
		switch sealAvariceOwner {
		case sevensigns.Dawn:
			return "dawn.htm"

		case sevensigns.Dusk:
			return "dusk.htm"
		}
	}

	e.NPC.ShowChatWindow(e.Player)
	return ""
}

// onTimer spawns the exit spirit of the raid boss the timer names, never
// respawning and gone after fifteen minutes.
func onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "lilith_exit") {
		s.Spawner().AddSpawn(exitGK, script.Loc{X: 184446, Y: -10112, Z: -5488, Heading: 0}, false, 900000*time.Millisecond)
	} else if strings.EqualFold(e.Name, "anakim_exit") {
		s.Spawner().AddSpawn(exitGK, script.Loc{X: 184466, Y: -13106, Z: -5488, Heading: 0}, false, 900000*time.Millisecond)
	}

	return ""
}

// onMyDying starts the dead raid boss's exit timer, bound to no NPC and no
// player.
func onMyDying(s *script.Script, e script.MyDying) {
	switch e.NPC.NpcID() {
	case lilith:
		s.StartTimer("lilith_exit", nil, nil, 10000*time.Millisecond)

	case anakim:
		s.StartTimer("anakim_exit", nil, nil, 10000*time.Millisecond)
	}
}
