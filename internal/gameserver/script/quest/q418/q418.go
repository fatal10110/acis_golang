// Package q418 is the quest Path to an Artisan: a dwarven fighter earns the
// Final Pass Certificate with a trial of ratman teeth for Silvera and one of
// the stolen secret box, or with an errand to one of the town's craftsmen.
package q418

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q418_PathToAnArtisan"

// Quest items.
const (
	silveraRing           = 1632
	firstPassCertificate  = 1633
	secondPassCertificate = 1634
	finalPassCertificate  = 1635
	boogleRatmanTooth     = 1636
	boogleRatmanLeader    = 1637
	klutoLetter           = 1638
	footprintOfThief      = 1639
	stolenSecretBox       = 1640
	secretBox             = 1641
)

// Npcs.
const (
	silvera  = 30527
	kluto    = 30317
	pinter   = 30298
	obi      = 32052
	hitchi   = 31963
	lockirin = 30531
	rydel    = 31956
)

// The classes the quest checks.
const (
	classDwarvenFighter = 53
	classArtisan        = 56
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to an Artisan",
		QuestID: 418,
		Items: []int32{
			silveraRing, firstPassCertificate, secondPassCertificate, boogleRatmanTooth, boogleRatmanLeader,
			klutoLetter, footprintOfThief, stolenSecretBox, secretBox,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {silvera},
			script.EventTalked:     {silvera, kluto, pinter, obi, hitchi, lockirin, rydel},
			script.EventMyDying:    {20389, 20390, 20017},
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
	case strings.EqualFold(e.Name, "30527-05.htm"):
		switch {
		case p.ClassID() != classDwarvenFighter:
			if p.ClassID() == classArtisan {
				htmltext = "30527-02a.htm"
			} else {
				htmltext = "30527-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30527-03.htm"
		case p.HasItem(finalPassCertificate):
			htmltext = "30527-04.htm"
		}
	case strings.EqualFold(e.Name, "30527-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, silveraRing, 1)
	case strings.EqualFold(e.Name, "30527-08a.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, boogleRatmanLeader, -1)
		s.TakeItems(p, boogleRatmanTooth, -1)
		s.TakeItems(p, silveraRing, 1)
		s.GiveItems(p, firstPassCertificate, 1)
	case strings.EqualFold(e.Name, "30527-08b.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, boogleRatmanLeader, -1)
		s.TakeItems(p, boogleRatmanTooth, -1)
		s.TakeItems(p, silveraRing, 1)
	case strings.EqualFold(e.Name, "30317-04.htm") || strings.EqualFold(e.Name, "30317-07.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, klutoLetter, 1)
	case strings.EqualFold(e.Name, "30317-10.htm"):
		finish(s, st, p, 6980)
	case strings.EqualFold(e.Name, "30317-12.htm"), strings.EqualFold(e.Name, "30531-05.htm"),
		strings.EqualFold(e.Name, "32052-11.htm"), strings.EqualFold(e.Name, "31963-10.htm"),
		strings.EqualFold(e.Name, "31956-04.htm"):
		finish(s, st, p, 3490)
	case strings.EqualFold(e.Name, "30298-03.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, klutoLetter, -1)
		s.GiveItems(p, footprintOfThief, 1)
	case strings.EqualFold(e.Name, "30298-06.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, footprintOfThief, -1)
		s.TakeItems(p, stolenSecretBox, -1)
		s.GiveItems(p, secondPassCertificate, 1)
		s.GiveItems(p, secretBox, 1)
	case strings.EqualFold(e.Name, "32052-06.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31963-04.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31963-05.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31963-07.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
	}
	return htmltext
}

// finish hands over the Final Pass Certificate for the passes and the secret
// box, rewards the character with experience and sp and ends the quest.
func finish(s *script.Script, st *script.QuestState, p *script.Player, sp int32) {
	s.TakeItems(p, firstPassCertificate, 1)
	s.TakeItems(p, secondPassCertificate, 1)
	s.TakeItems(p, secretBox, 1)
	s.GiveItems(p, finalPassCertificate, 1)
	s.RewardExpAndSp(p, 3200, sp)
	p.SocialAction(3)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(true)
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		htmltext = "30527-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case silvera:
			switch cond {
			case 1:
				htmltext = "30527-07.htm"
			case 2:
				htmltext = "30527-08.htm"
			case 3:
				htmltext = "30527-09.htm"
			case 8:
				htmltext = "30527-09a.htm"
			}
		case kluto:
			switch cond {
			case 3:
				htmltext = "30317-01.htm"
			case 4:
				htmltext = "30317-08.htm"
			case 7:
				htmltext = "30317-09.htm"
			}
		case pinter:
			switch cond {
			case 4:
				htmltext = "30298-01.htm"
			case 5:
				htmltext = "30298-04.htm"
			case 6:
				htmltext = "30298-05.htm"
			case 7:
				htmltext = "30298-07.htm"
			}
		case obi:
			switch cond {
			case 8:
				htmltext = "32052-01.htm"
			case 9:
				htmltext = "32052-06a.htm"
			case 11:
				htmltext = "32052-07.htm"
			}
		case hitchi:
			switch cond {
			case 9:
				htmltext = "31963-01.htm"
			case 10:
				htmltext = "31963-04.htm"
			case 11:
				htmltext = "31963-06a.htm"
			case 12:
				htmltext = "31963-08.htm"
			}
		case lockirin:
			if cond == 10 {
				htmltext = "30531-01.htm"
			}
		case rydel:
			if cond == 12 {
				htmltext = "31956-01.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case 20389:
		if st.Cond() == 1 && s.DropItems(p, boogleRatmanTooth, 1, 10, 700000) && p.ItemCount(boogleRatmanLeader) == 2 {
			st.SetCond(2)
		}
	case 20390:
		if st.Cond() == 1 && s.DropItems(p, boogleRatmanLeader, 1, 2, 500000) && p.ItemCount(boogleRatmanTooth) == 10 {
			st.SetCond(2)
		}
	case 20017:
		if st.Cond() == 5 && s.DropItems(p, stolenSecretBox, 1, 1, 200000) {
			st.SetCond(6)
		}
	}
}
