// Package herocirclet is the monuments that give a hero the hero's circlet.
package herocirclet

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// circlet is the hero's circlet.
const circlet = 6842

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: {31690, 31769, 31770, 31771, 31772}},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

// onTalk gives a hero without the circlet one, answering nothing; anyone
// else is told why not.
func onTalk(s *script.Script, e script.Talk) string {
	p := e.Player
	if !p.IsHero() {
		return "no_hero.htm"
	}
	if p.HasItem(circlet) {
		return "already_have_circlet.htm"
	}
	s.GiveItems(p, 6842, 1)
	return ""
}
