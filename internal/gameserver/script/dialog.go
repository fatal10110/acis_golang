package script

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// The dialog pages the engine answers with.
const (
	noQuestPage          = "<html><body>You are either not on a quest that involves this NPC, or you don't meet this NPC's minimum quest requirements.</body></html>"
	alreadyCompletedPage = "<html><body>This quest has already been completed.</body></html>"
	tooManyQuestsPage    = "<html><body>You have already accepted the maximum number of quests. No more than 25 quests may be undertaken simultaneously.<br>For quest information, enter Alt+U.</body></html>"
)

// maxStartedQuests is the number of started quests a player may hold; a
// quest dialog does not start another one past it.
const maxStartedQuests = 25

// InteractionDistance is the distance a player must stand strictly within,
// centre to centre in 3D, from the NPC a quest event link acts through.
const InteractionDistance = 150

// NoQuestMsg returns the page saying the NPC has nothing for the player.
func NoQuestMsg() string { return noQuestPage }

// AlreadyCompletedMsg returns the page saying the quest is completed.
func AlreadyCompletedMsg() string { return alreadyCompletedPage }

// Interact is a player's talk to an NPC in reach: the NPC becomes the
// player's last quest NPC, then, when exactly one script holds the NPC's
// first talk, that script answers and Interact reports true. Otherwise it
// reports false and the NPC answers with its own window. An empty answer
// releases the client; an aborted one sends nothing.
func (r *Registry) Interact(p *Player, n *NPC) bool {
	c := p.character()
	c.SetLastQuestNPC(n.ObjectID())
	list := r.scripts(n.NpcID(), EventFirstTalk)
	if len(list) != 1 {
		return false
	}
	s := list[0]
	res := r.answer(s, hookFirstTalk, func() string { return s.Hooks.FirstTalk(s, FirstTalk{NPC: n, Player: p}) })
	if res.Kind == ResultNone {
		c.ReleaseDialog()
		return true
	}
	s.show(c, n, res)
	return true
}

// QuestWindow answers a player's "Quest [name]" dialog command to the NPC
// n. With no name it opens the NPC's quest window: the real quests talking
// through the NPC that the player has a state in other than created, then
// the real quests the NPC starts; none gives the no-quest page, one that
// quest's window, more a choice list. With a name it opens the window of
// the script of that name, ignoring case, bound to the NPC or not; an
// unknown name gives the no-quest page.
func (r *Registry) QuestWindow(p *Player, n *NPC, name string) {
	if name != "" {
		r.questWindow(p, n, r.byName[strings.ToLower(name)])
		return
	}
	c := p.character()
	npcID := n.NpcID()
	var quests []*Script
	for _, s := range r.scripts(npcID, EventTalked) {
		if s.QuestID <= 0 || containsScript(quests, s) {
			continue
		}
		if st := c.Quests().State(s.Name); st == nil || st.Status() == questlog.StatusCreated {
			continue
		}
		quests = append(quests, s)
	}
	for _, s := range r.scripts(npcID, EventQuestStart) {
		if s.QuestID <= 0 || containsScript(quests, s) {
			continue
		}
		quests = append(quests, s)
	}
	switch len(quests) {
	case 0:
		r.questWindow(p, n, nil)
	case 1:
		r.questWindow(p, n, quests[0])
	default:
		showQuestChoice(c, n, quests)
	}
}

// questWindow opens s's window on n for p: the no-quest page when s is
// nil. For a real quest, a player carrying too much is told so and nothing
// else happens; a player with no state in it who already holds the most
// started quests gets the too-many page; otherwise a state is created when
// n starts the quest. Then n becomes p's last quest NPC and s's talk hook
// answers.
func (r *Registry) questWindow(p *Player, n *NPC, s *Script) {
	c := p.character()
	if s == nil {
		c.ShowDialogPage(n.ObjectID(), "", noQuestPage)
		return
	}
	if s.QuestID > 0 {
		if c.Overweight() {
			c.NotifyQuestOverweight()
			return
		}
		if c.Quests().State(s.Name) == nil {
			if c.Quests().Started() >= maxStartedQuests {
				c.ShowDialogPage(n.ObjectID(), "", tooManyQuestsPage)
				return
			}
			if containsScript(r.scripts(n.NpcID(), EventQuestStart), s) {
				s.env.Quests.NewState(c, s)
			}
		}
	}
	c.SetLastQuestNPC(n.ObjectID())
	res := r.answer(s, hookTalk, func() string { return s.Hooks.Talk(s, Talk{NPC: n, Player: p}) })
	s.show(c, n, res)
}

// showQuestChoice opens the list of quests for p to pick one from, each a
// link to its window marked in progress or done by p's state in it.
func showQuestChoice(c *player.Character, n *NPC, quests []*Script) {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for _, s := range quests {
		sb.WriteString(`<a action="bypass -h npc_%objectId%_Quest `)
		sb.WriteString(s.Name)
		sb.WriteString(`">[`)
		sb.WriteString(s.Title)
		switch st := c.Quests().State(s.Name); {
		case st != nil && st.Status() == questlog.StatusStarted:
			sb.WriteString(" (In Progress)]</a><br>")
		case st != nil && st.Status() == questlog.StatusCompleted:
			sb.WriteString(" (Done)]</a><br>")
		default:
			sb.WriteString("]</a><br>")
		}
	}
	sb.WriteString("</body></html>")
	c.ShowDialogPage(n.ObjectID(), "", sb.String())
}

// QuestEvent runs a player's quest event link "Quest <name> <event>"
// through last, the NPC the player last talked to about quests (nil when
// that is no NPC in the world): the event hook of the script of that name,
// ignoring case, runs when the player stands strictly within
// InteractionDistance of last and a script talking through last counts as
// the same script. Anything else does nothing.
func (r *Registry) QuestEvent(p *Player, last *NPC, name, event string) {
	s := r.byName[strings.ToLower(name)]
	if s == nil || last == nil {
		return
	}
	c := p.character()
	x, y, z := c.Position()
	nx, ny, nz := last.combatant().Position()
	if !location.In3DRadius(x, y, z, nx, ny, nz, InteractionDistance) {
		return
	}
	for _, o := range r.scripts(last.NpcID(), EventTalked) {
		if !equal(o, s) {
			continue
		}
		res := r.answer(s, hookEvent, func() string { return s.Hooks.Event(s, Event{Name: event, NPC: last, Player: p}) })
		s.show(c, last, res)
		return
	}
}

// equal reports whether a and b count as one script when a quest event
// names b: two behaviors always; two scripts of the same quest id when
// their names match; otherwise the same script.
func equal(a, b *Script) bool {
	if a.Behavior && b.Behavior {
		return true
	}
	if a.QuestID > 0 && a.QuestID == b.QuestID {
		return a.Name == b.Name
	}
	return a.path == b.path
}

func containsScript(list []*Script, s *Script) bool {
	for _, o := range list {
		if o == s {
			return true
		}
	}
	return false
}

// show sends c what s's hook answered, through the NPC n when n is set: a
// page file of s or a whole page in a dialog window, or a chat line.
func (s *Script) show(c *player.Character, n *NPC, res Result) {
	switch res.Kind {
	case ResultPageFile:
		c.ShowDialogPage(n.ObjectID(), s.pagePath(res.Text), "")
	case ResultPage:
		c.ShowDialogPage(n.ObjectID(), "", res.Text)
	case ResultChat:
		c.NotifyScriptMessage(res.Text)
	}
}

// pagePath is the datapack path of s's page file: a real quest's pages are
// in quest/<Name>, any other script's in <Dir>/<Name>.
func (s *Script) pagePath(file string) string {
	if s.QuestID > 0 {
		return "./data/html/script/quest/" + s.Name + "/" + file
	}
	return "./data/html/script/" + s.Dir + "/" + s.Name + "/" + file
}

// lidiasHeart is the quest whose diary the help page diaryPage marks read.
const (
	lidiasHeart = "Q023_LidiasHeart"
	diaryPage   = "lidias_diary/7064-16.htm"
	diaryItem   = 7064
)

// ReadHelpPage runs what opening the help page file, shown for the item
// itemID, does to c's quests: the last page of Lidia's diary marks the diary
// read in that quest at condition 5 when it is not yet. It reports false
// when c's state in that quest holds a value that is not a number: the
// page is then not shown, as in the reference.
func (q *Quests) ReadHelpPage(c *player.Character, file string, itemID int32) bool {
	if itemID != diaryItem || !strings.EqualFold(file, diaryPage) {
		return true
	}
	st := q.stateNamed(c, lidiasHeart)
	if st == nil {
		return true
	}
	cond, ok := st.intVar(questlog.KeyCond)
	if !ok {
		return false
	}
	diary, ok := st.intVar("diary")
	if !ok {
		return false
	}
	if cond == 5 && diary == 0 {
		st.Set("diary", "1")
	}
	return true
}

// intVar returns the state's variable key as a number, 0 when it is not
// set; ok is false when it is set to something that is not a 32-bit
// integer.
func (qs *QuestState) intVar(key string) (int32, bool) {
	v, set := qs.Get(key)
	if !set {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 32)
	return int32(n), err == nil
}
