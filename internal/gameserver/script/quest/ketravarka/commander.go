package ketravarka

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// commander is the quest that asks a level 75 ally for the head of a
// commander of the other tribe, a raid boss, and pays for it with a totem
// the alliance quest asks for. Every ally of the killer's party near the
// commander gets a head.
type commander struct {
	tribe
	name string
	// level is the alliance level the quest asks for; the ally also holds
	// the alliance mark of that level.
	level int32
	head  int32
	// totem is the reward, which a taker may not hold yet.
	totem int32
}

// offered reports whether p may take the quest.
func (q commander) offered(p *script.Player) bool {
	return q.allied(p, q.level) && p.HasItem(q.marks[q.level-1]) && !p.HasItem(q.totem)
}

// start starts the quest; the start link checks nothing.
func (q commander) start(s *script.Script, p *script.Player, st *script.QuestState) {
	st.SetStatus(questlog.StatusStarted)
	st.SetCond(1)
	s.PlaySound(p, script.SoundAccept)
}

// trade trades the head for the totem and ends the quest, reporting false
// when p has no head: the quest then goes back to cond 1.
func (q commander) trade(s *script.Script, p *script.Player, st *script.QuestState) bool {
	if !p.HasItem(q.head) {
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		return false
	}
	s.TakeItems(p, q.head, -1)
	s.GiveItems(p, q.totem, 1)
	s.RewardExpAndSp(p, 10000, 0)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(true)
	return true
}

// talk answers the quest's taker by the head it holds, and offers the
// quest to a level 75 ally; the quest's pages are low, offer, refuse,
// holding and empty.
func (q commander) talk(s *script.Script, e script.Talk, low, offer, refuse, holding, empty string) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, q.name)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case e.Player.Level() < 75:
			htmltext = low
		case q.offered(e.Player):
			htmltext = offer
		default:
			htmltext = refuse
		}
	case questlog.StatusStarted:
		htmltext = pick(e.Player.HasItem(q.head), holding, empty)
	}
	return htmltext
}

// onMyDying gives a head to every party member at cond 1 near the commander
// who is an ally of the quest's alliance level and holds its mark, and moves
// each to cond 2.
func (q commander) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	for _, st := range s.PartyMembers(p, e.NPC, script.ByCond(1)) {
		pm := st.Player()
		if q.allied(pm, q.level) && pm.HasItem(q.marks[q.level-1]) {
			st.SetCond(2)
			s.PlaySound(pm, script.SoundMiddle)
			s.GiveItems(pm, q.head, 1)
		}
	}
}
