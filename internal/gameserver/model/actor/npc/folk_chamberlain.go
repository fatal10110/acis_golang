package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/clan"

// ChamberlainRefusal is the page a castle chamberlain answers one of its
// dialog commands with when the talker may not run it, "" when the talker
// may: the busy page while its castle's siege runs for a talker of any
// clan; the refusal page for a talker of no clan, of a clan not owning the
// castle, or not holding need.
func (r ResidenceTalk) ChamberlainRefusal(need clan.Privilege) string {
	const dir = "data/html/chamberlain/"
	switch r.castleTalkCond(r.SiegeInProgress, clan.PrivNone) {
	case talkUnderSiege:
		return dir + "busy.htm"
	case talkStranger:
		return dir + "noprivs.htm"
	case talkOwner:
	}
	if need != clan.PrivNone && !r.holds(need) {
		return dir + "noprivs.htm"
	}
	return ""
}
