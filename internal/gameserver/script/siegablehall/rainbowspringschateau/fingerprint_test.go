package rainbowspringschateau

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// RainbowSpringsChateau class. The reference names its system messages by
// their SystemMessageId constants, which the port writes as ids, and adds
// a countdown's number through SystemMessage.addNumber, which the port sets
// on the message. Its notices to the chosen clans go to each one's online
// members (Clan.getOnlineMembers), which the port asks of the hall
// (Hall.TellClans); it clears the attackers through their list
// (getAttackerClans), the port through Hall.ClearAttackers. It keeps the
// decrees in SQL of its own and logs; the port's hall runtime stores them,
// and nothing is logged. Its delayed steps are pool tasks, which the port
// runs as script timers of its own names, a countdown's naming its number,
// parsed back as a 32-bit int, and its timer hook. Its party's member list
// matches the name of the Quest party helper the port does not call. It
// writes the deadline's hour on a 12-hour clock (Calendar.HOUR), the port
// as the hour modulo 12. The port panics where the reference reads a maker
// it lacks.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.siegablehall.RainbowSpringsChateau"},
		"number 844", "number 856", "number 1823", "number 1824", "number 1825", "number 1826", "number 1828", "number 1841",
		"call SystemMessage.addNumber",
		"call Clan.getOnlineMembers", "call ClanHallSiege.getAttackerClans",
		`string "Couldn't add attacker."`, `string "Couldn't clear attackers."`, `string "Couldn't load attackers."`, `string "Couldn't remove attacker."`,
		`string "DELETE FROM rainbowsprings_attacker_list WHERE clanId = ?"`, `string "DELETE FROM rainbowsprings_attacker_list"`,
		`string "INSERT INTO rainbowsprings_attacker_list VALUES (?,?)"`, `string "SELECT * FROM rainbowsprings_attacker_list"`,
		`string "clanId"`, `string "war_decrees_count"`,
		`string "No date was set for Rainbow Springs Chateau siege. Siege is canceled."`,
		"call ThreadPool.schedule", "call Quest.cancelQuestTimer", "hook onTimer none", "number 32",
		`string "rainbow-end-now"`, `string "rainbow-final-attackers"`, `string "rainbow-minutes-"`, `string "rainbow-registration-ended"`,
		`string "rainbow-seconds-"`, `string "rainbow-siege-end"`, `string "rainbow-siege-start"`, `string "rainbow-teleport-out"`,
		"call Quest.getPartyMembers",
		"number 12",
		`string "rainbowspringschateau: no npcmaker "`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
