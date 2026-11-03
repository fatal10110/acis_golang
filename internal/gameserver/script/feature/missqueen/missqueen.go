// Package missqueen is Miss Queen, who gives a young player of a starting
// class the trainee's coupon and one of a first occupation the traveler's
// coupon, each once per character.
package missqueen

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The rewards.
const (
	traineesCoupon  = 7832
	travelersCoupon = 7833
)

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: {31760}},
		Hooks: script.Hooks{OnEvent: onAdvEvent},
	}
}

// onAdvEvent answers the coupon links. A player of the link's class tier,
// level 6 to 25, with no PK, gets the coupon once, the memo of that coupon
// set; asked again it is told it has it; any other player is told it does
// not qualify. Any other event is the page it names.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	if strings.EqualFold(e.Name, "newbie_coupon") {
		if p.ClassLevel() == 0 && p.Level() >= 6 && p.Level() <= 25 && p.PkKills() <= 0 {
			if _, ok := p.Memo("MissQueen_Trainees"); ok {
				htmltext = "31760-01.htm"
			} else {
				htmltext = "31760-02.htm"
				p.SetMemo("MissQueen_Trainees", strconv.FormatBool(true))
				s.GiveItems(p, traineesCoupon, 1)
			}
		} else {
			htmltext = "31760-03.htm"
		}
	} else if strings.EqualFold(e.Name, "traveller_coupon") {
		if p.ClassLevel() == 1 && p.Level() >= 6 && p.Level() <= 25 && p.PkKills() <= 0 {
			if _, ok := p.Memo("MissQueen_Traveler"); ok {
				htmltext = "31760-04.htm"
			} else {
				htmltext = "31760-05.htm"
				p.SetMemo("MissQueen_Traveler", strconv.FormatBool(true))
				s.GiveItems(p, travelersCoupon, 1)
			}
		} else {
			htmltext = "31760-06.htm"
		}
	}
	return htmltext
}
