package festival

import (
	"context"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// maxLevels are the highest party level each festival takes.
var maxLevels = [Count]int{31, 42, 53, 64, 80}

// The seal stones a signup is paid in, and what each adds to the
// festival's bonus.
const (
	blueStoneID  = 6360
	greenStoneID = 6361
	redStoneID   = 6362

	blueStoneBonus  = 3
	greenStoneBonus = 5
	redStoneBonus   = 10
)

// The texts the signup and the score registration answer with.
const (
	signupDuringFestivalText = "You cannot sign up while a festival is in progress."
	scoreDuringFestivalText  = "You cannot register a score while a festival is in progress."
	noOfferingsText          = "You do not have any blood offerings to contribute."
)

// signupRefusal answers why t's party may not sign up for g's festival:
// during seal validation (page 2a); while a festival is under way; for a
// player in no party (2b); for a member who does not lead it (2c); for a
// party of fewer than MinPlayers members (2b); for a party whose highest
// level is above the festival's (2d).
func (m *Manager) signupRefusal(g Guide, t GuideTalker) (GuideReply, bool) {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation {
		return m.chatWindow(g, 2, "a", false), true
	}
	if m.Initialized() {
		return GuideReply{Message: signupDuringFestivalText}, true
	}
	switch p := t.Party; {
	case p == nil:
		return m.chatWindow(g, 2, "b", false), true
	case p.Leader != t.ObjectID:
		return m.chatWindow(g, 2, "c", false), true
	case len(p.Members) < m.cfg.MinPlayers:
		return m.chatWindow(g, 2, "b", false), true
	case p.Level > maxLevels[g.FestivalID]:
		return m.chatWindow(g, 2, "d", false), true
	}
	return GuideReply{}, false
}

// signUp answers "Festival 2 <stone>": t's party signs up for g's
// festival, its leader paying the fee in blue, green or red seal stones,
// which the festival's bonus gains at 3, 5 or 10 a stone (page 2e). The
// signup is refused as page 1's check refuses it; a stone of no other
// color, or too few of them, reads as not enough items. A stone that is
// not a number stops the handling.
func (m *Manager) signUp(g Guide, t GuideTalker, command string) GuideReply {
	arg, ok := commandTail(command, 11)
	if !ok {
		return GuideReply{Aborted: true}
	}
	stone, err := strconv.Atoi(arg)
	if err != nil {
		return GuideReply{Aborted: true}
	}
	if reply, refused := m.signupRefusal(g, t); refused {
		return reply
	}
	var stoneID int32
	var needed, bonus int
	switch stone {
	case blueStoneID:
		stoneID, needed, bonus = blueStoneID, g.Blue, blueStoneBonus
	case greenStoneID:
		stoneID, needed, bonus = greenStoneID, g.Green, greenStoneBonus
	case redStoneID:
		stoneID, needed, bonus = redStoneID, g.Red, redStoneBonus
	default:
		return GuideReply{NotEnoughItems: true}
	}
	if t.Pay == nil || !t.Pay(stoneID, needed) {
		return GuideReply{}
	}
	m.mu.Lock()
	m.setParticipantsLocked(g.Cabal, g.FestivalID, t.Party.Members)
	m.status.Bonuses[g.FestivalID] += needed * bonus
	m.mu.Unlock()
	return m.chatWindow(g, 2, "e", false)
}

// commandTail returns command from byte from on; ok is false when command
// is shorter.
func commandTail(command string, from int) (string, bool) {
	if len(command) < from {
		return "", false
	}
	return command[from:], true
}

// registerScore answers "Festival 3": the leader of a party that ran g's
// festival last registers its score, five points per blood offering it
// holds, all of them destroyed. Refused during seal validation (page 3a);
// while a festival is under way; for a player in no party, or who did not
// lead a party of g's last festival (3b); for a player holding no blood
// offering. The score beats the cabal's best of the cycle (3c) or not
// (3d); either way the player's contribution score is said to rise by it.
func (m *Manager) registerScore(g Guide, t GuideTalker) GuideReply {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation {
		return m.chatWindow(g, 3, "a", false)
	}
	m.mu.Lock()
	inProgress := m.inProgress
	previous := m.previous[side(g.Cabal)][g.FestivalID]
	m.mu.Unlock()
	if inProgress {
		return GuideReply{Message: scoreDuringFestivalText}
	}
	if t.Party == nil || len(previous) == 0 || previous[0].ObjectID != t.ObjectID {
		return m.chatWindow(g, 3, "b", false)
	}
	count, held, taken := t.TakeOfferings()
	if !held {
		return GuideReply{Message: noOfferingsText}
	}
	if !taken {
		// Offerings that could not be destroyed register nothing, as
		// silently as their destruction failed; the dialog's release
		// still answers the command.
		return GuideReply{}
	}
	score := int(int32(count) * offeringScoreValue)
	best, ok := m.setFinalScore(g.Cabal, g.FestivalID, score)
	if !ok {
		return GuideReply{Aborted: true}
	}
	suffix := "d"
	if best {
		suffix = "c"
	}
	reply := m.chatWindow(g, 3, suffix, false)
	reply.Contribution = score
	return reply
}

// setFinalScore registers score for festival id of cabal, set by the party
// of its last festival. It beats the cabal's best of the cycle unless the
// rival's best is higher still: then the cycle's best of the cabal becomes
// score, dated now and naming the party, the cabal gains the festival's
// worth in festival score, and both the scores and the Seven Signs status
// are saved. best reports whether it did; ok is false when no best score
// of the cycle is kept.
func (m *Manager) setFinalScore(cabal sevensigns.Cabal, id, score int) (best, ok bool) {
	m.mu.Lock()
	dawn, okDawn := m.highestScoreLocked(sevensigns.Dawn, id)
	dusk, okDusk := m.highestScoreLocked(sevensigns.Dusk, id)
	if !okDawn || !okDusk {
		m.mu.Unlock()
		return false, false
	}
	own, rival := dawn, dusk
	if cabal == sevensigns.Dusk {
		own, rival = dusk, dawn
	}
	if score <= own.Score || own.Score < rival.Score {
		m.mu.Unlock()
		return false, true
	}
	previous := m.previous[side(cabal)][id]
	names := make([]string, len(previous))
	for i, p := range previous {
		names[i] = p.Name
	}
	own.Date, own.Score, own.Members = m.nowMillis(), score, strings.Join(names, ",")
	m.scores[m.signsCycle][scoreKey(cabal, id)] = own
	m.mu.Unlock()

	m.calendar.AddFestivalScore(cabal, MaxScores[id])
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := m.SaveScores(ctx); err != nil {
		m.log.Error().Err(err).Msg("festival: save the scores")
	}
	if err := m.calendar.SaveStatus(ctx); err != nil {
		m.log.Error().Err(err).Msg("festival: save the seven signs status")
	}
	return true, true
}

// raiseChallenge answers "Festival 8": while the festival is under way,
// the leader of a party raises the challenge of g's arena once (page 8b;
// 8c when it was raised already). A member who does not lead its party is
// refused (8a). A player in no party, or a festival not under way, gets
// no answer of its own, only the release the dialog ends every command
// with. An arena not open stops the handling.
func (m *Manager) raiseChallenge(g Guide, t GuideTalker) GuideReply {
	if t.Party == nil {
		return GuideReply{}
	}
	m.mu.Lock()
	inProgress := m.inProgress
	m.mu.Unlock()
	if !inProgress {
		return GuideReply{}
	}
	if t.Party.Leader != t.ObjectID {
		return m.chatWindow(g, 8, "a", false)
	}
	increased, ok := m.IncreaseChallenge(g.Cabal, g.FestivalID)
	if !ok {
		return GuideReply{Aborted: true}
	}
	if increased {
		return m.chatWindow(g, 8, "b", false)
	}
	return m.chatWindow(g, 8, "c", false)
}

// leave answers "Festival 9": a party leader takes its whole party out of
// the festival; any other member leaves it alone, and is expelled from the
// party. A player in no party gets nothing more. Neither answers with a
// page of its own: the arena's removal notices and the release the dialog
// ends every command with are all the player sees.
func (m *Manager) leave(t GuideTalker) GuideReply {
	if t.Party == nil {
		return GuideReply{}
	}
	if t.Party.Leader == t.ObjectID {
		m.UpdateParticipants(t.ObjectID, nil)
		return GuideReply{}
	}
	m.UpdateParticipants(t.ObjectID, t.Party.Members)
	return GuideReply{Expel: true}
}
