// Package q364 is the quest Jovial Accordion: Barbado's accordion is
// recovered by stealing a pair of items out of two locked chests.
package q364

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q364_JovialAccordion"

// Npcs.
const (
	barbado    = 30959
	swan       = 30957
	sabrin     = 30060
	xaber      = 30075
	clothChest = 30961
	beerChest  = 30960
)

// Quest items.
const (
	key1          = 4323
	key2          = 4324
	stolenBeer    = 4321
	stolenClothes = 4322
)

// echo is the reward.
const echo = 4421

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Jovial Accordion",
		QuestID: 364,
		Items:   []int32{key1, key2, stolenBeer, stolenClothes},
		Bind: script.Bindings{
			script.EventQuestStart: {barbado},
			script.EventTalked:     {barbado, swan, sabrin, xaber, clothChest, beerChest},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30959-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("items", strconv.Itoa(0))
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30957-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, key1, 1)
		s.GiveItems(p, key2, 1)
	case strings.EqualFold(e.Name, "30960-04.htm"):
		if p.HasItem(key2) {
			s.TakeItems(p, key2, 1)
			if s.RndBool() {
				htmltext = "30960-02.htm"
				s.GiveItems(p, stolenBeer, 1)
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case strings.EqualFold(e.Name, "30961-04.htm"):
		if p.HasItem(key1) {
			s.TakeItems(p, key1, 1)
			if s.RndBool() {
				htmltext = "30961-02.htm"
				s.GiveItems(p, stolenClothes, 1)
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 15 {
			htmltext = "30959-00.htm"
		} else {
			htmltext = "30959-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		stolenItems := st.GetInt("items")
		switch e.NPC.NpcID() {
		case barbado:
			switch {
			case cond == 1 || cond == 2:
				htmltext = "30959-03.htm"
			case cond == 3:
				htmltext = "30959-04.htm"
				s.GiveItems(p, echo, 1)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case swan:
			switch cond {
			case 1:
				htmltext = "30957-01.htm"
			case 2:
				if stolenItems > 0 {
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
					if stolenItems == 2 {
						htmltext = "30957-04.htm"
						s.RewardItems(p, 57, 100)
					} else {
						htmltext = "30957-05.htm"
					}
				} else if !p.HasItem(key1) && !p.HasItem(key2) {
					htmltext = "30957-06.htm"
					s.PlaySound(p, script.SoundFinish)
					st.Exit(true)
				} else {
					htmltext = "30957-03.htm"
				}
			case 3:
				htmltext = "30957-07.htm"
			}
		case beerChest:
			htmltext = "30960-03.htm"
			if cond == 2 && p.HasItem(key2) {
				htmltext = "30960-01.htm"
			}
		case clothChest:
			htmltext = "30961-03.htm"
			if cond == 2 && p.HasItem(key1) {
				htmltext = "30961-01.htm"
			}
		case sabrin:
			if p.HasItem(stolenBeer) {
				htmltext = "30060-01.htm"
				st.Set("items", strconv.Itoa(int(stolenItems+1)))
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, stolenBeer, 1)
			} else {
				htmltext = "30060-02.htm"
			}
		case xaber:
			if p.HasItem(stolenClothes) {
				htmltext = "30075-01.htm"
				st.Set("items", strconv.Itoa(int(stolenItems+1)))
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, stolenClothes, 1)
			} else {
				htmltext = "30075-02.htm"
			}
		}
	}
	return htmltext
}
