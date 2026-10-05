package clan

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// AcademyCircletID is the circlet an academy graduate is handed.
const AcademyCircletID = 8181

// Graduation is an academy member's graduation from its clan.
type Graduation struct {
	Clan *Clan
	// Member is the graduate's roster entry as it left the clan.
	Member Member
	// Points is the reputation the clan earned.
	Points int
	// Reputation is the clan's new score; ReputationMoved is false when it
	// did not move, a clan below level 5 earning none.
	Reputation      ReputationChanged
	ReputationMoved bool
}

// graduationPoints is the reputation a member that joined the academy at
// level joined earns its clan by graduating: 400 at level 16 or below, 170
// at 39 or above, 10 less per level in between.
func graduationPoints(joined int) int {
	switch {
	case joined <= 16:
		return 400
	case joined >= 39:
		return 170
	default:
		return 400 - (joined-16)*10
	}
}

// Graduate graduates c, which has just taken its second occupation, from
// its clan when it joined the clan's academy: the clan earns the
// graduation's reputation and c leaves the roster, as one step, so no
// other change to the roster runs between them. It reports false, having
// done nothing, for a character outside any clan or that never joined an
// academy. The caller clears c's clan state with ApplyGraduated.
func (s *Service) Graduate(c *player.Character, now time.Time) (Graduation, bool) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return Graduation{}, false
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	m, ok := cl.members[c.ID]
	if !ok || m.LvlJoinedAcademy == 0 {
		return Graduation{}, false
	}
	g := Graduation{Clan: cl, Points: graduationPoints(m.LvlJoinedAcademy)}
	g.Reputation, g.ReputationMoved = s.addReputationLocked(cl, g.Points)
	g.Member, _ = s.removeLocked(cl, c.ID, 0, c, now)
	return g, true
}

// ApplyGraduated clears the clan state of m, which graduated, on its live
// character; it runs on that character's queue. A graduate carries no
// join penalty: an academy member keeps whatever it had, and one moved out
// of the academy since has its penalty cleared.
func (s *Service) ApplyGraduated(c *player.Character, m Member) {
	s.applyLeft(c, m, 0)
}
