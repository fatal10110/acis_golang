package raidaltar

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q610Name = "Q610_MagicalPowerOfWater_Part2"

// soulOfWaterAshutar is the raid the altar calls.
const soulOfWaterAshutar = 25316

// NPCs.
const (
	asefa           = 31372
	varkasHolyAltar = 31560
)

// Items.
const (
	greenTotem        = 7238
	iceHeartOfAshutar = 7239
)

// q610 is the quest; the raid mechanics are the site's.
type q610 struct{ site }

// Q610 is the quest Magical Power of Water, Part 2: Asefa takes the
// Green Totem to the altar of the Varka, which spends it on calling Soul of
// Water Ashutar, and the Ice Heart of Ashutar is traded back for a reward.
func Q610() script.Script {
	q := q610{newSite(soulOfWaterAshutar, script.Loc{X: 104825, Y: -36926, Z: -1136}, 61050, 61051, iceHeartOfAshutar)}
	return script.Script{
		Title:   "Magical Power of Water - Part 2",
		QuestID: 610,
		Items:   []int32{iceHeartOfAshutar},
		Bind: script.Bindings{
			script.EventQuestStart: {asefa},
			script.EventTalked:     {asefa, varkasHolyAltar},
			script.EventDecayed:    {soulOfWaterAshutar},
			script.EventMyDying:    {soulOfWaterAshutar},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnDecayed: q.onDecayed, OnMyDying: q.onMyDying},
	}
}

func (q q610) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q610Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Asefa.
	case strings.EqualFold(e.Name, "31372-04.htm"):
		if p.HasItem(greenTotem) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "31372-02.htm"
		}
	case strings.EqualFold(e.Name, "31372-07.htm"):
		if p.HasItem(iceHeartOfAshutar) {
			s.TakeItems(p, iceHeartOfAshutar, 1)
			s.RewardExpAndSp(p, 10000, 0)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31372-08.htm"
		}
	// Varka's Holy Altar.
	case strings.EqualFold(e.Name, "31560-02.htm"):
		switch {
		case !p.HasItem(greenTotem):
			htmltext = "31560-03.htm"
		case !q.raid.Empty():
			htmltext = "31560-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31560-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, greenTotem, 1)
		}
	}
	return htmltext
}

func (q q610) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q610Name)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case !e.Player.HasItem(greenTotem):
			htmltext = "31372-02.htm"
		case e.Player.Level() < 75 && e.Player.VarkaKetraAlliance() < 2:
			htmltext = "31372-03.htm"
		default:
			htmltext = "31372-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case asefa:
			if cond < 3 {
				htmltext = "31372-05.htm"
			} else {
				htmltext = "31372-06.htm"
			}
		case varkasHolyAltar:
			switch cond {
			case 1:
				htmltext = "31560-01.htm"
			case 2:
				htmltext = "31560-05.htm"
			}
		}
	}
	return htmltext
}
