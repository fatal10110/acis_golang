// Package heroweapon is the monuments that give a hero one hero's weapon of
// its choice.
package heroweapon

import (
	"slices"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// weaponIDs are the hero's weapons.
func weaponIDs() []int32 {
	return []int32{6611, 6612, 6613, 6614, 6615, 6616, 6617, 6618, 6619, 6620, 6621}
}

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: {31690, 31769, 31770, 31771, 31772, 31773}},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent gives the hero's weapon the event names; another id gives
// nothing. Either answers nothing.
func onAdvEvent(s *script.Script, e script.Event) string {
	// An event that is not an int32 number aborts here.
	id, err := strconv.ParseInt(e.Name, 10, 32)
	if err != nil {
		panic(err)
	}
	weaponID := int32(id)
	if slices.Contains(weaponIDs(), weaponID) {
		s.GiveItems(e.Player, weaponID, 1)
	}
	return ""
}

// onTalk opens the weapon list to a hero holding no hero's weapon; anyone
// else is told why not.
func onTalk(_ *script.Script, e script.Talk) string {
	p := e.Player
	if !p.IsHero() {
		return "no_hero.htm"
	}
	if p.HasAtLeastOneItem(weaponIDs()...) {
		return "already_have_weapon.htm"
	}
	return "weapon_list.htm"
}
