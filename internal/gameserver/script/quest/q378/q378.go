// Package q378 is the quest Magnificent Feast: Ranspo asks for a wine, a
// musical score and a recipe, and pays according to the dishes chosen.
package q378

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q378_MagnificentFeast"

// Npcs.
const ranspo = 30594

// Quest items.
const (
	wine15        = 5956
	wine30        = 5957
	wine60        = 5958
	musicalScore  = 4421
	saladRecipe   = 1455
	sauceRecipe   = 1456
	steakRecipe   = 1457
	ritronDessert = 5959
)

// reward is what Ranspo gives for a score: an item, its count and adena.
type reward struct {
	item, count, adena int32
}

// rewardFor returns the reward of a score, the sum of the wine and the dish
// values, and whether the score has one.
func rewardFor(score string) (reward, bool) {
	switch score {
	case "9":
		return reward{847, 1, 5700}, true
	case "10":
		return reward{846, 2, 0}, true
	case "12":
		return reward{909, 1, 25400}, true
	case "17":
		return reward{846, 2, 1200}, true
	case "18":
		return reward{879, 1, 6900}, true
	case "20":
		return reward{890, 2, 8500}, true
	case "33":
		return reward{879, 1, 8100}, true
	case "34":
		return reward{910, 1, 0}, true
	case "36":
		return reward{848, 1, 2200}, true
	}
	return reward{}, false
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Magnificent Feast",
		QuestID: 378,
		Bind: script.Bindings{
			script.EventQuestStart: {ranspo},
			script.EventTalked:     {ranspo},
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
	case strings.EqualFold(e.Name, "30594-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30594-4a.htm"):
		htmltext = pickWine(s, p, st, wine15, 1, htmltext)
	case strings.EqualFold(e.Name, "30594-4b.htm"):
		htmltext = pickWine(s, p, st, wine30, 2, htmltext)
	case strings.EqualFold(e.Name, "30594-4c.htm"):
		htmltext = pickWine(s, p, st, wine60, 4, htmltext)
	case strings.EqualFold(e.Name, "30594-6.htm"):
		if p.HasItem(musicalScore) {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, musicalScore, 1)
		} else {
			htmltext = "30594-5.htm"
		}
	default:
		score := st.GetInt("score")
		switch {
		case strings.EqualFold(e.Name, "30594-8a.htm"):
			htmltext = pickDish(s, p, st, saladRecipe, score+8, htmltext)
		case strings.EqualFold(e.Name, "30594-8b.htm"):
			htmltext = pickDish(s, p, st, sauceRecipe, score+16, htmltext)
		case strings.EqualFold(e.Name, "30594-8c.htm"):
			htmltext = pickDish(s, p, st, steakRecipe, score+32, htmltext)
		}
	}
	return htmltext
}

// pickWine takes the wine and scores it, or answers that the player has none.
func pickWine(s *script.Script, p *script.Player, st *script.QuestState, wine, score int32, htmltext string) string {
	if !p.HasItem(wine) {
		return "30594-4.htm"
	}
	st.SetCond(2)
	st.Set("score", strconv.Itoa(int(score)))
	s.PlaySound(p, script.SoundMiddle)
	s.TakeItems(p, wine, 1)
	return htmltext
}

// pickDish takes the recipe and adds its value to the score, or answers that
// the player has none.
func pickDish(s *script.Script, p *script.Player, st *script.QuestState, recipe, score int32, htmltext string) string {
	if !p.HasItem(recipe) {
		return "30594-8.htm"
	}
	st.SetCond(4)
	st.Set("score", strconv.Itoa(int(score)))
	s.PlaySound(p, script.SoundMiddle)
	s.TakeItems(p, recipe, 1)
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
		if p.Level() < 20 {
			htmltext = "30594-0.htm"
		} else {
			htmltext = "30594-1.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30594-3.htm"
		case 2:
			if !p.HasItem(musicalScore) {
				htmltext = "30594-5.htm"
			} else {
				htmltext = "30594-5a.htm"
			}
		case 3:
			htmltext = "30594-7.htm"
		case 4:
			score, _ := st.Get("score")
			r, ok := rewardFor(score)
			if ok && p.HasItem(ritronDessert) {
				htmltext = "30594-10.htm"
				s.TakeItems(p, ritronDessert, 1)
				s.GiveItems(p, r.item, r.count)
				if r.adena > 0 {
					s.RewardItems(p, 57, r.adena)
				}
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			} else {
				htmltext = "30594-9.htm"
			}
		}
	}
	return htmltext
}
