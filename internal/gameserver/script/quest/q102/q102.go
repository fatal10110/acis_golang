// Package q102 is the quest Sea of Spores Fever: an elf carries Alberius'
// letter to Cobendell, gathers dryad tears for his medicine and hands the
// medicine to four sick guards, for a sentinel's weapon.
package q102

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q102_SeaOfSporesFever"

// Items.
const (
	alberiusLetter     = 964
	evergreenAmulet    = 965
	dryadTears         = 966
	alberiusList       = 746
	cobendellMedicine1 = 1130
	cobendellMedicine2 = 1131
	cobendellMedicine3 = 1132
	cobendellMedicine4 = 1133
	cobendellMedicine5 = 1134
)

// Rewards.
const (
	swordOfSentinel   = 743
	staffOfSentinel   = 744
	lesserHealingPot  = 1060
	soulshotNoGrade   = 1835
	spiritshotNoGrade = 2509
	echoBattle        = 4412
	echoLove          = 4413
	echoSolitude      = 4414
	echoFeast         = 4415
	echoCelebration   = 4416
)

// NPCs.
const (
	alberius    = 30284
	cobendell   = 30156
	berros      = 30217
	veltress    = 30219
	rayen       = 30221
	gartrandell = 30285
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sea of Spores Fever",
		QuestID: 102,
		Items: []int32{
			alberiusLetter, evergreenAmulet, dryadTears, cobendellMedicine1, cobendellMedicine2,
			cobendellMedicine3, cobendellMedicine4, cobendellMedicine5, alberiusList,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {alberius},
			script.EventTalked:     {alberius, cobendell, berros, rayen, gartrandell, veltress},
			script.EventMyDying:    {20013, 20019},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30284-02.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, alberiusLetter, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceElf:
			htmltext = "30284-00.htm"
		case p.Level() < 12:
			htmltext = "30284-08.htm"
		default:
			htmltext = "30284-07.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case alberius:
			switch cond {
			case 1:
				htmltext = "30284-03.htm"
			case 2, 3:
				htmltext = "30284-09.htm"
			case 4:
				htmltext = "30284-04.htm"
				st.SetCond(5)
				st.Set("medicines", strconv.Itoa(4))
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, cobendellMedicine1, 1)
				s.GiveItems(p, alberiusList, 1)
			case 5:
				htmltext = "30284-05.htm"
			case 6:
				htmltext = "30284-06.htm"
				s.TakeItems(p, alberiusList, 1)

				if p.IsMageClass() {
					s.GiveItems(p, staffOfSentinel, 1)
					s.RewardItems(p, spiritshotNoGrade, 500)
				} else {
					s.GiveItems(p, swordOfSentinel, 1)
					s.RewardItems(p, soulshotNoGrade, 1000)
				}

				s.RewardItems(p, lesserHealingPot, 100)
				s.RewardItems(p, echoBattle, 10)
				s.RewardItems(p, echoLove, 10)
				s.RewardItems(p, echoSolitude, 10)
				s.RewardItems(p, echoFeast, 10)
				s.RewardItems(p, echoCelebration, 10)

				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case cobendell:
			switch cond {
			case 1:
				htmltext = "30156-03.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, alberiusLetter, 1)
				s.GiveItems(p, evergreenAmulet, 1)
			case 2:
				htmltext = "30156-04.htm"
			case 5:
				htmltext = "30156-07.htm"
			case 3:
				htmltext = "30156-05.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, dryadTears, -1)
				s.TakeItems(p, evergreenAmulet, 1)
				s.GiveItems(p, cobendellMedicine1, 1)
				s.GiveItems(p, cobendellMedicine2, 1)
				s.GiveItems(p, cobendellMedicine3, 1)
				s.GiveItems(p, cobendellMedicine4, 1)
				s.GiveItems(p, cobendellMedicine5, 1)
			case 4:
				htmltext = "30156-06.htm"
			}
		case berros:
			if cond == 5 {
				htmltext = "30217-01.htm"
				checkItem(s, p, st, cobendellMedicine2)
			}
		case veltress:
			if cond == 5 {
				htmltext = "30219-01.htm"
				checkItem(s, p, st, cobendellMedicine3)
			}
		case rayen:
			if cond == 5 {
				htmltext = "30221-01.htm"
				checkItem(s, p, st, cobendellMedicine4)
			}
		case gartrandell:
			if cond == 5 {
				htmltext = "30285-01.htm"
				checkItem(s, p, st, cobendellMedicine5)
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if s.DropItems(p, dryadTears, 1, 10, 300000) {
		st.SetCond(3)
	}
}

// checkItem takes the guard's medicine, when the player holds it, and
// moves the quest on once the last of the four is delivered.
func checkItem(s *script.Script, p *script.Player, st *script.QuestState, itemID int32) {
	if p.HasItem(itemID) {
		s.TakeItems(p, itemID, 1)

		medicinesLeft := st.GetInt("medicines") - 1
		if medicinesLeft == 0 {
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			st.Set("medicines", strconv.Itoa(int(medicinesLeft)))
		}
	}
}
