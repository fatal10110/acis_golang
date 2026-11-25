package ketravarka

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// need is how many soldier, officer and captain badges the raise to mark
// asks for. It is also the most badges of each kind the quest drops at stage
// mark.
func need(mark int) [3]int32 {
	return [5][3]int32{
		{100, 0, 0},
		{200, 100, 0},
		{300, 200, 100},
		{300, 300, 200},
		{400, 400, 200},
	}[mark-1]
}

// alliance is the quest that raises an ally's alliance level one mark at a
// time: it takes the badges the other tribe's warriors drop to its kill hook
// and, for the fourth and fifth marks, the totems of the commander quests.
type alliance struct {
	tribe
	// badges are the soldier, officer and captain badges of the other tribe.
	badges [3]int32
	// valor and wisdom are the totems the fourth and the fifth mark ask for.
	valor, wisdom int32
	// soldiers, officers and captains are the warriors dropping each badge.
	soldiers, officers, captains []int32
	// chance is a warrior's badge drop chance, in millionths.
	chance func(npcID int32) int32
	// trophy is the item of the war quest trophyQuest that the warriors drop
	// here instead; trophyChance is a warrior's chance in millionths, and
	// whether it drops the trophy at all.
	trophy       int32
	trophyQuest  string
	trophyChance func(npcID int32) (int32, bool)
}

// ready reports whether p holds what the raise to mark asks for.
func (q alliance) ready(p *script.Player, mark int) bool {
	for i, n := range need(mark) {
		if p.ItemCount(q.badges[i]) < n {
			return false
		}
	}
	switch mark {
	case 4:
		return p.HasItem(q.valor)
	case 5:
		return p.HasItem(q.wisdom)
	}
	return true
}

// raise trades what the raise to mark asks for for the mark, reporting false
// when p is short of it. The fifth mark takes the badges it counts; the
// others take every badge of the kinds they count.
func (q alliance) raise(s *script.Script, p *script.Player, st *script.QuestState, mark int) bool {
	if !q.ready(p, mark) {
		return false
	}
	st.SetCond(int32(mark) + 1)
	s.PlaySound(p, script.SoundMiddle)
	for i, n := range need(mark) {
		if n == 0 {
			continue
		}
		if mark < 5 {
			n = -1
		}
		s.TakeItems(p, q.badges[i], n)
	}
	switch mark {
	case 4:
		s.TakeItems(p, q.valor, -1)
	case 5:
		s.TakeItems(p, q.wisdom, -1)
	}
	if mark > 1 {
		s.TakeItems(p, q.marks[mark-2], -1)
	}
	s.GiveItems(p, q.marks[mark-1], 1)
	p.SetVarkaKetraAlliance(q.sign * int32(mark))
	return true
}

// quit takes every mark and totem, ends the alliance and the quest.
func (q alliance) quit(s *script.Script, p *script.Player, st *script.QuestState) {
	for _, mark := range q.marks {
		s.TakeItems(p, mark, -1)
	}
	s.TakeItems(p, q.valor, -1)
	s.TakeItems(p, q.wisdom, -1)
	p.SetVarkaKetraAlliance(0)
	st.Exit(true)
}

// onMyDying gives a started member of the killer's party the war quest's
// trophy when it also holds that quest, and else a badge for its stage.
func (q alliance) onMyDying(s *script.Script, e script.MyDying) {
	killer := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(killer, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	p := st.Player()
	npcID := e.NPC.NpcID()

	if war := s.QuestState(p, q.trophyQuest); war != nil && s.RndBool() {
		if c, ok := q.trophyChance(npcID); ok {
			s.DropItems(p, q.trophy, 1, 0, c)
			return
		}
	}

	cond := st.Cond()
	if cond < 1 || cond > 5 {
		return
	}
	for i, warriors := range [3][]int32{q.soldiers, q.officers, q.captains} {
		if !slices.Contains(warriors, npcID) {
			continue
		}
		if most := need(int(cond))[i]; most > 0 {
			s.DropItems(p, q.badges[i], 1, most, q.chance(npcID))
		}
		return
	}
}

// pick is yes when ok holds, and else no.
func pick(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
