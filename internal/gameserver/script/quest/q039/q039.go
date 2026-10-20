// Package q039 is the quest Red-Eyed Invaders: Babenco sends a player to
// Bathis, who wants the Maille lizardmen's bone necklaces and then their
// incense pouches and gems, and pays in fishing gear.
package q039

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q039_RedEyedInvaders"

// NPCs.
const (
	babenco = 30334
	bathis  = 30332
)

// Mobs.
const (
	mailleLizardman      = 20919
	mailleLizardmanScout = 20920
	mailleLizardmanGuard = 20921
	araneid              = 20925
)

// Items.
const (
	blackBoneNecklace = 7178
	redBoneNecklace   = 7179
	incensePouch      = 7180
	gemOfMaille       = 7181
)

// Rewards.
const (
	greenColoredLureHG = 6521
	babyDuckRode       = 6529
	fishingShotNG      = 6535
)

// firstDrop is a monster's necklace drop: the item it drops and the item
// whose full count, with it, moves the quest on.
type firstDrop struct{ item, pair int32 }

// firstDroplist returns the necklace drops.
func firstDroplist() map[int32]firstDrop {
	return map[int32]firstDrop{
		mailleLizardmanGuard: {redBoneNecklace, blackBoneNecklace},
		mailleLizardman:      {blackBoneNecklace, redBoneNecklace},
		mailleLizardmanScout: {blackBoneNecklace, redBoneNecklace},
	}
}

// secondDrop is a monster's pouch or gem drop: the item, the item paired
// with it, and the chance.
type secondDrop struct{ item, pair, chance int32 }

// secondDroplist returns the pouch and gem drops.
func secondDroplist() map[int32]secondDrop {
	return map[int32]secondDrop{
		araneid:              {gemOfMaille, incensePouch, 500000},
		mailleLizardmanGuard: {incensePouch, gemOfMaille, 300000},
		mailleLizardmanScout: {incensePouch, gemOfMaille, 250000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Red-Eyed Invaders",
		QuestID: 39,
		Items:   []int32{blackBoneNecklace, redBoneNecklace, incensePouch, gemOfMaille},
		Bind: script.Bindings{
			script.EventQuestStart: {babenco},
			script.EventTalked:     {babenco, bathis},
			script.EventMyDying:    {mailleLizardman, mailleLizardmanScout, mailleLizardmanGuard, araneid},
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
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30334-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30332-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30332-3.htm"):
		st.SetCond(4)
		s.TakeItems(p, blackBoneNecklace, -1)
		s.TakeItems(p, redBoneNecklace, -1)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30332-5.htm"):
		s.TakeItems(p, incensePouch, -1)
		s.TakeItems(p, gemOfMaille, -1)
		s.GiveItems(p, greenColoredLureHG, 60)
		s.GiveItems(p, babyDuckRode, 1)
		s.GiveItems(p, fishingShotNG, 500)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 20 {
			htmltext = "30334-2.htm"
		} else {
			htmltext = "30334-0.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case babenco:
			htmltext = "30334-3.htm"
		case bathis:
			switch cond {
			case 1:
				htmltext = "30332-0.htm"
			case 2:
				htmltext = "30332-2a.htm"
			case 3:
				htmltext = "30332-2.htm"
			case 4:
				htmltext = "30332-3a.htm"
			case 5:
				htmltext = "30332-4.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	npcID := e.NPC.NpcID()
	if st := s.RandomPartyMember(p, e.NPC, script.ByCond(2)); st != nil && npcID != araneid {
		d := firstDroplist()[npcID]
		member := st.Player()
		if s.DropItems(member, d.item, 1, 100, 500000) && member.ItemCount(d.pair) == 100 {
			st.SetCond(3)
		}
		return
	}
	if st := s.RandomPartyMember(p, e.NPC, script.ByCond(4)); st != nil && npcID != mailleLizardman {
		d := secondDroplist()[npcID]
		member := st.Player()
		if s.DropItems(member, d.item, 1, 30, d.chance) && member.ItemCount(d.pair) == 30 {
			st.SetCond(5)
		}
	}
}
