package festival

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Names are the festivals' names, by festival id.
var Names = [Count]string{"Level 31 or lower", "Level 42 or lower", "Level 53 or lower", "Level 64 or lower", "No Level Limit"}

// Guide is a festival guide or festival witch: the festival and oracle it
// serves, and the seal stones of each color a party pays to sign up there.
type Guide struct {
	FestivalID       int
	Cabal            sevensigns.Cabal
	Blue, Green, Red int
}

// signupStones are the blue, green and red seal stones each festival's
// signup costs.
var signupStones = [Count][3]int{{900, 540, 270}, {1500, 900, 450}, {3000, 1800, 900}, {4500, 2700, 1350}, {6000, 3600, 1800}}

// GuideOf returns the guide or witch npcID is: 31127 to 31131 guide the
// five Dawn festivals and 31132 to 31136 are their witches, 31137 to 31146
// the same for Dusk. ok is false for any other NPC.
func GuideOf(npcID int) (Guide, bool) {
	var cabal sevensigns.Cabal
	var id int
	switch {
	case npcID >= 31127 && npcID <= 31136:
		cabal, id = sevensigns.Dawn, (npcID-31127)%Count
	case npcID >= 31137 && npcID <= 31146:
		cabal, id = sevensigns.Dusk, (npcID-31137)%Count
	default:
		return Guide{}, false
	}
	s := signupStones[id]
	return Guide{FestivalID: id, Cabal: cabal, Blue: s[0], Green: s[1], Red: s[2]}, true
}

// GuideTalker is what a guide's dialog reads of the player talking, and
// how it charges it.
type GuideTalker struct {
	ObjectID int32
	Name     string
	// Winning is set when the player's cabal is the one the Seven Signs
	// scores put ahead; on a tie, a player of no cabal counts as winning.
	Winning bool
	// Party is the player's party; nil when it is in none.
	Party *GuideParty
	// Pay destroys count units of the item itemID the player holds,
	// telling it what disappeared; holding fewer, it destroys nothing,
	// tells the player it has not enough items and reports false.
	Pay func(itemID int32, count int) bool
	// TakeOfferings destroys every blood offering the player holds,
	// without a word, and returns how many there were: held is false when
	// it holds none, taken false when they could not be destroyed.
	TakeOfferings func() (count int, held, taken bool)
}

// GuideParty is the party of a player talking to a guide.
type GuideParty struct {
	Leader int32
	// Members are the party's members in the order they joined.
	Members []Member
	// Level is the level of the party's highest member.
	Level int
}

// GuideReply is a guide's answer to one dialog command.
type GuideReply struct {
	// Aborted stops the handling outright: nothing is sent.
	Aborted bool
	// Message is a plain-text message, sent first.
	Message string
	// NotEnoughItems tells the player it has not enough items, first.
	NotEnoughItems bool
	// Contribution, when positive, tells the player its contribution
	// score rose by that much, after Message.
	Contribution int
	// Expel expels the player from its party once the rest is sent.
	Expel bool
	// Page is the data/html/seven_signs/festival/ page to open and Fill
	// its placeholders as name and value pairs, filled after %objectId%.
	// HTML is a page built instead.
	Page string
	Fill []string
	HTML string
	// Release follows the page with ActionFailed.
	Release bool
}

// The texts a guide answers with.
const (
	bonusDuringCompetitionText = "Bonuses cannot be paid during the competition period."
	childrenOfDusk             = "Children of Dusk"
	childrenOfDawn             = "Children of Dawn"
)

// Description answers "FestivalDesc <val>" at g: description page val.
func (m *Manager) Description(g Guide, val int) GuideReply {
	return m.chatWindow(g, val, "", true)
}

// Command answers "Festival <val>" at g, sent by t to the NPC objectID;
// command is the whole command:
//   - 0 pays t its share of the accumulated bonus during seal validation
//     (see distributeBonus) and says whether there was one; any other
//     period it says bonuses are not paid;
//   - 1 checks t's party may sign up (see signupRefusal), then shows the
//     fee, or with t already signed up signs its party up again;
//   - 2 <stone> signs t's party up, paid in seal stones (see signUp);
//   - 3 registers the score of t's party (see registerScore);
//   - 4 shows the festival's best score of each cabal this cycle and its
//     best score ever;
//   - 8 raises the challenge of the festival under way (see
//     raiseChallenge);
//   - 9 takes t out of the festival (see leave);
//   - any other val opens page val, 5 with the festivals' standings and 6
//     with their accumulated bonuses.
func (m *Manager) Command(g Guide, t GuideTalker, objectID int32, val int, command string) GuideReply {
	switch val {
	case 0:
		if m.calendar.CurrentPeriod() != sevensigns.SealValidation {
			return GuideReply{Message: bonusDuringCompetitionText}
		}
		if m.distributeBonus(t) > 0 {
			return m.chatWindow(g, 0, "a", false)
		}
		return m.chatWindow(g, 0, "b", false)
	case 1:
		if reply, refused := m.signupRefusal(g, t); refused {
			return reply
		}
		if m.IsParticipant(t.ObjectID) {
			m.mu.Lock()
			m.setParticipantsLocked(g.Cabal, g.FestivalID, t.Party.Members)
			m.mu.Unlock()
			return m.chatWindow(g, 2, "f", false)
		}
		return m.chatWindow(g, 1, "", false)
	case 2:
		return m.signUp(g, t, command)
	case 3:
		return m.registerScore(g, t)
	case 4:
		return m.highScores(g, objectID)
	case 8:
		return m.raiseChallenge(g, t)
	case 9:
		return m.leave(t)
	}
	return m.chatWindow(g, val, "", false)
}

// chatWindow opens page val, with suffix, of the descriptions or of the
// festival pages. Every page names g's festival and the minutes to the
// next festival cycle; page 1 names the seal stones of each color the
// signup costs, page 5 holds the festivals' standings and page 6 their
// accumulated bonuses. A standing that cannot be read aborts.
func (m *Manager) chatWindow(g Guide, val int, suffix string, description bool) GuideReply {
	prefix := "festival_"
	if description {
		prefix = "desc_"
	}
	reply := GuideReply{Page: prefix + strconv.Itoa(val) + suffix + ".htm", Release: true}
	reply.Fill = []string{"%festivalType%", Names[g.FestivalID], "%cycleMins%", strconv.Itoa(m.minutesToNextCycle())}
	if !description && val == 2 && suffix == "b" {
		reply.Fill = append(reply.Fill, "%minFestivalPartyMembers%", strconv.Itoa(m.cfg.MinPlayers))
	}
	switch val {
	case 1:
		reply.Fill = append(reply.Fill, "%blueStoneNeeded%", strconv.Itoa(g.Blue), "%greenStoneNeeded%", strconv.Itoa(g.Green), "%redStoneNeeded%", strconv.Itoa(g.Red))
	case 5:
		table, ok := m.statsTable()
		if !ok {
			return GuideReply{Aborted: true}
		}
		reply.Fill = append(reply.Fill, "%statsTable%", table)
	case 6:
		reply.Fill = append(reply.Fill, "%bonusTable%", m.bonusTable())
	}
	return reply
}

// statsTable lists each festival's best Dusk and Dawn score this cycle and
// the cabal ahead, "None" on a tie. ok is false when a score is not kept.
func (m *Manager) statsTable() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for id := range Count {
		dawn, okDawn := m.highestScoreLocked(sevensigns.Dawn, id)
		dusk, okDusk := m.highestScoreLocked(sevensigns.Dusk, id)
		if !okDawn || !okDusk {
			return "", false
		}
		ahead := childrenOfDusk
		switch {
		case dawn.Score > dusk.Score:
			ahead = childrenOfDawn
		case dawn.Score == dusk.Score:
			ahead = "None"
		}
		b.WriteString(`<tr><td width="100" align="center">` + Names[id] + `</td><td align="center" width="35">` + strconv.Itoa(dusk.Score) +
			`</td><td align="center" width="35">` + strconv.Itoa(dawn.Score) + `</td><td align="center" width="130">` + ahead + `</td></tr>`)
	}
	return b.String(), true
}

// bonusTable lists each festival's accumulated bonus.
func (m *Manager) bonusTable() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for id := range Count {
		b.WriteString(`<tr><td align="center" width="150">` + Names[id] + `</td><td align="center" width="150">` + strconv.Itoa(m.status.Bonuses[id]) + `</td></tr>`)
	}
	return b.String()
}

// highScores builds the page of g's festival's best score of each cabal
// this cycle, with the party that set it, and its best score ever. The
// side named for the best score ever is always the Children of Dusk,
// whichever cabal set it. A score of this cycle that is not kept aborts.
func (m *Manager) highScores(g Guide, objectID int32) GuideReply {
	m.mu.Lock()
	dawn, okDawn := m.highestScoreLocked(sevensigns.Dawn, g.FestivalID)
	dusk, okDusk := m.highestScoreLocked(sevensigns.Dusk, g.FestivalID)
	overall, okOverall := m.overallHighestLocked(g.FestivalID)
	m.mu.Unlock()
	if !okDawn || !okDusk {
		return GuideReply{Aborted: true}
	}
	var b strings.Builder
	b.WriteString("<html><body>Festival Guide:<br>These are the top scores of the week, for the " + Names[g.FestivalID] + " festival.<br>")
	for _, side := range [...]struct {
		name  string
		score Score
	}{{"Dawn", dawn}, {"Dusk", dusk}} {
		if side.score.Score > 0 {
			b.WriteString(side.name + ": " + scoreDate(side.score.Date) + ". Score " + strconv.Itoa(side.score.Score) + "<br>" + side.score.Members + "<br>")
		} else {
			b.WriteString(side.name + ": No record exists. Score 0<br>")
		}
	}
	if okOverall {
		b.WriteString("Consecutive top scores: " + scoreDate(overall.Date) + ". Score " + strconv.Itoa(overall.Score) + "<br>Affilated side: " + childrenOfDusk + "<br>" + overall.Members + "<br>")
	} else {
		b.WriteString("Consecutive top scores: No record exists. Score 0<br>")
	}
	b.WriteString(`<a action="bypass -h npc_` + strconv.Itoa(int(objectID)) + `_Chat 0">Go back.</a></body></html>`)
	return GuideReply{HTML: b.String()}
}

// scoreDate writes a score's date, in Unix milliseconds, as year/month/day
// in the server's time zone, the month counted from 0 for January.
func scoreDate(millis int64) string {
	t := time.UnixMilli(millis).In(time.Local)
	return strconv.Itoa(t.Year()) + "/" + strconv.Itoa(int(t.Month())-1) + "/" + strconv.Itoa(t.Day())
}

// distributeBonus pays t its party's share of an accumulated bonus and
// returns it, 0 when t is not on the winning side or set no score this
// cycle. The first of this cycle's scores, Dusk's festivals then Dawn's,
// whose members contain t's name anywhere in the text yields its
// festival's bonus divided by the number of members, which leaves the
// bonus. The share is taken from the bonus; nothing is handed to t.
func (m *Manager) distributeBonus(t GuideTalker) int {
	if !t.Winning {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cycle, ok := m.scores[m.signsCycle]
	if !ok {
		return 0
	}
	for key := range 2 * Count {
		s, ok := cycle[key]
		if !ok || s.FestivalID < 0 || s.FestivalID >= Count || !strings.Contains(s.Members, t.Name) {
			continue
		}
		bonus := m.status.Bonuses[s.FestivalID]
		share := bonus / len(s.MemberNames())
		m.status.Bonuses[s.FestivalID] = bonus - share
		return share
	}
	return 0
}
