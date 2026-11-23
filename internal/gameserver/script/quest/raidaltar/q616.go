package raidaltar

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q616Name = "Q616_MagicalPowerOfFire_Part2"

// soulOfFireNastron is the raid the altar calls.
const soulOfFireNastron = 25306

// NPCs.
const (
	udanMardui      = 31379
	ketrasHolyAltar = 31558
)

// Items.
const (
	redTotem           = 7243
	fireHeartOfNastron = 7244
)

// q616 is the quest; the raid mechanics are the site's.
type q616 struct{ site }

// Q616 is the quest Magical Power of Fire, Part 2: Udan takes the
// Red Totem to the altar of the Ketra, which spends it on calling Soul of
// Fire Nastron, and the Fire Heart of Nastron is traded back for a reward.
func Q616() script.Script {
	q := q616{newSite(soulOfFireNastron, script.Loc{X: 142528, Y: -82528, Z: -6496}, 61650, 61651, fireHeartOfNastron)}
	return script.Script{
		Title:   "Magical Power of Fire - Part 2",
		QuestID: 616,
		Items:   []int32{fireHeartOfNastron},
		Bind: script.Bindings{
			script.EventQuestStart: {udanMardui},
			script.EventTalked:     {udanMardui, ketrasHolyAltar},
			script.EventDecayed:    {soulOfFireNastron},
			script.EventMyDying:    {soulOfFireNastron},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnDecayed: q.onDecayed, OnMyDying: q.onMyDying},
	}
}

func (q q616) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q616Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Udan Mardui.
	case strings.EqualFold(e.Name, "31379-04.htm"):
		if p.HasItem(redTotem) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "31379-02.htm"
		}
	case strings.EqualFold(e.Name, "31379-08.htm"):
		if p.HasItem(fireHeartOfNastron) {
			s.TakeItems(p, fireHeartOfNastron, 1)
			s.RewardExpAndSp(p, 10000, 0)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31379-09.htm"
		}
	// Ketra's Holy Altar.
	case strings.EqualFold(e.Name, "31558-02.htm"):
		switch {
		case !p.HasItem(redTotem):
			htmltext = "31558-03.htm"
		case !q.raid.Empty():
			htmltext = "31558-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31558-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, redTotem, 1)
		}
	}
	return htmltext
}

func (q q616) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q616Name)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case !e.Player.HasItem(redTotem):
			htmltext = "31379-02.htm"
		case e.Player.Level() < 75 && e.Player.VarkaKetraAlliance() > -2:
			htmltext = "31379-03.htm"
		default:
			htmltext = "31379-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case udanMardui:
			switch cond {
			case 1:
				htmltext = "31379-05.htm"
			case 2:
				htmltext = "31379-06.htm"
			default:
				htmltext = "31379-07.htm"
			}
		case ketrasHolyAltar:
			switch cond {
			case 1:
				htmltext = "31558-01.htm"
			case 2:
				htmltext = "31558-05.htm"
			}
		}
	}
	return htmltext
}
