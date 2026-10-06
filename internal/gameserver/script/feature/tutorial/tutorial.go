// Package tutorial is the newbie tutorial: the tutorial window's pages,
// question marks, voices, radar marks and client events, from a new
// character's first steps in the world to its class changes.
package tutorial

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Tutorial"

// The newbie quests whose state the enter-world step checks.
const (
	quest101 = "Q101_SwordOfSolidarity"
	quest102 = "Q102_SeaOfSporesFever"
	quest103 = "Q103_SpiritOfCraftsman"
	quest104 = "Q104_SpiritOfMirrors"
	quest105 = "Q105_SkirmishWithTheOrcs"
	quest106 = "Q106_ForgottenTruth"
	quest107 = "Q107_MercilessPunishment"
	quest108 = "Q108_JumbleTumbleDiamondFuss"
)

// Items.
const (
	tutorialGuide = 5588
	blueGemstone  = 6353
)

// point is a place marked on the radar; the zero point marks nothing.
type point struct{ x, y, z int32 }

// classEvent is what the tutorial shows a starting class: the first voice
// and page, the page and radar mark of the newbie guide, and the pages and
// marks of later steps.
type classEvent struct {
	initialVoice, initialHtm, ce8Htm string
	ce8Loc                           point
	qmc9Htm                          string
	qmc9Loc                          point
	qmc24Htm, qmc35Htm               string
	ce47Loc                          point
}

// eventOf returns the tutorial of the starting class classID; ok is false
// for any other class.
func eventOf(classID int32) (evt classEvent, ok bool) {
	switch classID {
	case 0:
		return classEvent{"tutorial_voice_001a", "tutorial_human_fighter001.htm", "tutorial_human_fighter007.htm", point{-71424, 258336, -3109}, "tutorial_fighter017.htm", point{-83020, 242553, -3718}, "tutorial_newbie003a.htm", "tutorial_21.htm", point{}}, true
	case 10:
		return classEvent{"tutorial_voice_001b", "tutorial_human_mage001.htm", "tutorial_human_mage007.htm", point{-91036, 248044, -3568}, "tutorial_mage017.htm", point{}, "tutorial_newbie003a.htm", "tutorial_21a.htm", point{-84981, 244764, -3726}}, true
	case 18:
		return classEvent{"tutorial_voice_001c", "tutorial_elven_fighter001.htm", "tutorial_elf007.htm", point{46112, 41200, -3504}, "tutorial_fighter017.htm", point{45061, 52468, -2796}, "tutorial_newbie003b.htm", "tutorial_21b.htm", point{}}, true
	case 25:
		return classEvent{"tutorial_voice_001d", "tutorial_elven_mage001.htm", "tutorial_elf007.htm", point{46112, 41200, -3504}, "tutorial_mage017.htm", point{}, "tutorial_newbie003b.htm", "tutorial_21c.htm", point{45701, 52459, -2796}}, true
	case 31:
		return classEvent{"tutorial_voice_001e", "tutorial_delf_fighter001.htm", "tutorial_delf007.htm", point{28384, 11056, -4233}, "tutorial_fighter017.htm", point{10447, 14620, -4242}, "tutorial_newbie003c.htm", "tutorial_21g.htm", point{}}, true
	case 38:
		return classEvent{"tutorial_voice_001f", "tutorial_delf_mage001.htm", "tutorial_delf007.htm", point{28384, 11056, -4233}, "tutorial_mage017.htm", point{}, "tutorial_newbie003c.htm", "tutorial_21h.htm", point{10344, 14445, -4242}}, true
	case 44:
		return classEvent{"tutorial_voice_001g", "tutorial_orc_fighter001.htm", "tutorial_orc007.htm", point{-56736, -113680, -672}, "tutorial_orc_fighter017.htm", point{-46389, -113905, -21}, "tutorial_newbie003d.htm", "tutorial_21d.htm", point{}}, true
	case 49:
		return classEvent{"tutorial_voice_001h", "tutorial_orc_mage001.htm", "tutorial_orc007.htm", point{-56736, -113680, -672}, "tutorial_mage017.htm", point{}, "tutorial_newbie003d.htm", "tutorial_21e.htm", point{-46225, -113312, -21}}, true
	case 53:
		return classEvent{"tutorial_voice_001i", "tutorial_dwarven_fighter001.htm", "tutorial_dwarven_fighter007.htm", point{108567, -173994, -406}, "tutorial_dwarven017.htm", point{115271, -182692, -1445}, "tutorial_newbie003e.htm", "tutorial_21f.htm", point{}}, true
	}
	return classEvent{}, false
}

// closeLinkA is the second class transfer page of tutorial link 26, per
// first class; "" for a class with none.
func closeLinkA(classID int32) string {
	switch classID {
	case 1:
		return "tutorial_22w.htm"
	case 4:
		return "tutorial_22.htm"
	case 7:
		return "tutorial_22b.htm"
	case 11:
		return "tutorial_22c.htm"
	case 15:
		return "tutorial_22d.htm"
	case 19:
		return "tutorial_22e.htm"
	case 22:
		return "tutorial_22f.htm"
	case 26:
		return "tutorial_22g.htm"
	case 29:
		return "tutorial_22h.htm"
	case 32:
		return "tutorial_22n.htm"
	case 35:
		return "tutorial_22o.htm"
	case 39:
		return "tutorial_22p.htm"
	case 42:
		return "tutorial_22q.htm"
	case 45:
		return "tutorial_22i.htm"
	case 47:
		return "tutorial_22j.htm"
	case 50:
		return "tutorial_22k.htm"
	case 54:
		return "tutorial_22l.htm"
	case 56:
		return "tutorial_22m.htm"
	}
	return ""
}

// closeLinkB is the second class transfer page of tutorial link 23.
func closeLinkB(classID int32) string {
	switch classID {
	case 4:
		return "tutorial_22aa.htm"
	case 7:
		return "tutorial_22ba.htm"
	case 11:
		return "tutorial_22ca.htm"
	case 15:
		return "tutorial_22da.htm"
	case 19:
		return "tutorial_22ea.htm"
	case 22:
		return "tutorial_22fa.htm"
	case 26:
		return "tutorial_22ga.htm"
	case 32:
		return "tutorial_22na.htm"
	case 35:
		return "tutorial_22oa.htm"
	case 39:
		return "tutorial_22pa.htm"
	case 50:
		return "tutorial_22ka.htm"
	}
	return ""
}

// closeLinkC is the second class transfer page of tutorial link 24.
func closeLinkC(classID int32) string {
	switch classID {
	case 4:
		return "tutorial_22ab.htm"
	case 7:
		return "tutorial_22bb.htm"
	case 11:
		return "tutorial_22cb.htm"
	case 15:
		return "tutorial_22db.htm"
	case 19:
		return "tutorial_22eb.htm"
	case 22:
		return "tutorial_22fb.htm"
	case 26:
		return "tutorial_22gb.htm"
	case 32:
		return "tutorial_22nb.htm"
	case 35:
		return "tutorial_22ob.htm"
	case 39:
		return "tutorial_22pb.htm"
	case 50:
		return "tutorial_22kb.htm"
	}
	return ""
}

// New returns the feature.
func New() script.Script {
	return script.Script{
		QuestID: -1,
		Dir:     "feature",
		Hooks:   script.Hooks{OnEvent: onAdvEvent, OnTimer: onTimer},
	}
}

// setInt sets the state's variable key to n.
func setInt(st *script.QuestState, key string, n int) { st.Set(key, strconv.Itoa(n)) }

// addMarker marks at on p's radar.
func addMarker(p *script.Player, at point) { p.AddRadarMarker(at.x, at.y, at.z) }

func onTimer(s *script.Script, e script.Timer) string {
	if !strings.HasPrefix(e.Name, "QT") {
		return ""
	}
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return ""
	}
	switch st.GetInt("Ex") {
	case -2:
		evt, ok := eventOf(p.ClassID())
		if !ok {
			return ""
		}
		if !p.HasItem(tutorialGuide) {
			s.GiveItems(p, tutorialGuide, 1)
		}
		setInt(st, "Ex", -3)
		p.PlayTutorialVoice(evt.initialVoice)
		// Every player's pending QT timer is cancelled, not only this
		// player's.
		s.CancelTimers(script.TimerName("QT"))
		s.StartTimer("QT", nil, p, 30000*time.Millisecond)
		p.ShowTutorialHTML(evt.initialHtm)
	case -3:
		setInt(st, "Ex", 0)
		p.PlayTutorialVoice("tutorial_voice_002")
	case -4:
		setInt(st, "Ex", -5)
		p.PlayTutorialVoice("tutorial_voice_008")
	}
	return ""
}

// eventNumber reads the number after an event's two-letter prefix; ok is
// false, and the event does nothing more, when it is not a 32-bit integer.
func eventNumber(event string) (n int64, ok bool) {
	n, err := commons.ParseInt(event[2:], 32)
	return n, err == nil
}

func onAdvEvent(s *script.Script, e script.Event) string {
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return ""
	}
	event := e.Name
	classID := p.ClassID()
	html := ""
	switch {
	case strings.HasPrefix(event, "UC"):
		enterWorld(s, st, p)
	case strings.HasPrefix(event, "TE"):
		s.CancelTimers(script.TimerName("TE"))
		if event != "TE" {
			n, ok := eventNumber(event)
			if !ok {
				return ""
			}
			html = closeLink(s, st, p, classID, n)
		}
	case strings.HasPrefix(event, "CE"):
		level := p.Level()
		n, ok := eventNumber(event)
		if !ok {
			return ""
		}
		html = clientEvent(s, st, p, classID, level, n)
	case strings.HasPrefix(event, "QM"):
		n, ok := eventNumber(event)
		if !ok {
			return ""
		}
		html = questionMark(st, p, classID, n)
	}
	if html != "" {
		p.ShowTutorialHTML(html)
	}
	return ""
}

// enterWorld is the tutorial step of a player below level 6 entering the
// world, chosen by how far its tutorial has gone.
func enterWorld(s *script.Script, st *script.QuestState, p *script.Player) {
	if p.Level() >= 6 || st.GetInt("onlyone") != 0 {
		return
	}
	switch st.GetInt("ucMemo") {
	case 0:
		setInt(st, "Ex", -2)
		setInt(st, "ucMemo", 1)
		s.StartTimer("QT", nil, p, 10000*time.Millisecond)
	case 1:
		p.ShowQuestionMark(1)
		s.PlaySound(p, script.SoundTutorial)
		p.PlayTutorialVoice("tutorial_voice_006")
	case 2:
		started := false
		for _, q := range []string{quest101, quest102, quest103, quest104, quest105, quest106, quest107, quest108} {
			started = started || s.QuestState(p, q) != nil
		}
		if started {
			setInt(st, "ucMemo", 5)
			p.ShowQuestionMark(6)
		} else {
			p.ShowQuestionMark(2)
		}
		s.PlaySound(p, script.SoundTutorial)
	case 3:
		if st.GetInt("Ex") == 2 {
			p.ShowQuestionMark(3)
		} else if p.HasItem(blueGemstone) {
			p.ShowQuestionMark(5)
		}
		s.PlaySound(p, script.SoundTutorial)
	case 4:
		p.ShowQuestionMark(12)
		s.PlaySound(p, script.SoundTutorial)
		p.PlayTutorialVoice("tutorial_voice_025")
	}
	p.EnableTutorialEvent(0)
}

// closeLink answers the tutorial window link TE<n>: the page it opens, ""
// for none.
func closeLink(s *script.Script, st *script.QuestState, p *script.Player, classID int32, n int64) string {
	switch n {
	case 0, 12:
		p.CloseTutorialHTML()
	case 1:
		setInt(st, "Ex", -4)
		p.CloseTutorialHTML()
		p.ShowQuestionMark(1)
		s.PlaySound(p, script.SoundTutorial)
		p.PlayTutorialVoice("tutorial_voice_006")
		s.StartTimer("QT", nil, p, 30000*time.Millisecond)
	case 2:
		setInt(st, "Ex", -5)
		p.EnableTutorialEvent(1)
		p.PlayTutorialVoice("tutorial_voice_003")
		return "tutorial_02.htm"
	case 3:
		p.EnableTutorialEvent(2)
		return "tutorial_03.htm"
	case 4:
		p.EnableTutorialEvent(4)
		return "tutorial_04.htm"
	case 5:
		p.EnableTutorialEvent(8)
		return "tutorial_05.htm"
	case 6:
		p.EnableTutorialEvent(16)
		return "tutorial_06.htm"
	case 7:
		p.EnableTutorialEvent(0)
		return "tutorial_100.htm"
	case 8:
		p.EnableTutorialEvent(0)
		return "tutorial_101.htm"
	case 9:
		p.EnableTutorialEvent(0)
		return "tutorial_102.htm"
	case 10:
		p.EnableTutorialEvent(0)
		return "tutorial_103.htm"
	case 11:
		p.EnableTutorialEvent(0)
		return "tutorial_104.htm"
	case 23:
		return closeLinkB(classID)
	case 24:
		return closeLinkC(classID)
	case 25:
		return "tutorial_22cc.htm"
	case 26:
		return closeLinkA(classID)
	case 27:
		return "tutorial_29.htm"
	case 28:
		return "tutorial_28.htm"
	case 29, 30:
		return "tutorial_07a.htm"
	}
	return ""
}

// clientEvent answers the client event CE<n> of a player at level: the
// page it opens, "" for none.
func clientEvent(s *script.Script, st *script.QuestState, p *script.Player, classID, level int32, n int64) string {
	switch n {
	case 1:
		if level < 6 {
			p.EnableTutorialEvent(2)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_004")
			return "tutorial_03.htm"
		}
	case 2:
		if level < 6 {
			p.EnableTutorialEvent(8)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_005")
			return "tutorial_05.htm"
		}
	case 8:
		if level < 6 {
			evt, ok := eventOf(classID)
			if !ok {
				return ""
			}
			setInt(st, "Ex", -5)
			setInt(st, "ucMemo", 1)
			addMarker(p, evt.ce8Loc)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_007")
			return evt.ce8Htm
		}
	case 30:
		if level < 10 && st.GetInt("Die") == 0 {
			setInt(st, "Die", 1)
			p.ShowQuestionMark(8)
			p.EnableTutorialEvent(0)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_016")
		}
	case 40:
		levelReached(s, st, p, classID, level)
	case 45:
		if level < 6 && st.GetInt("HP") == 0 {
			setInt(st, "HP", 1)
			setInt(st, "sit", 8388608)
			p.ShowQuestionMark(10)
			p.EnableTutorialEvent(8388608)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_017")
		}
	case 57:
		if level < 6 && st.GetInt("Adena") == 0 {
			setInt(st, "Adena", 1)
			p.ShowQuestionMark(23)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_012")
		}
	case 6353:
		if level < 6 && st.GetInt("Gemstone") == 0 {
			setInt(st, "Gemstone", 1)
			p.ShowQuestionMark(5)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_013")
		}
	case 1048576:
		if level < 6 {
			p.ShowQuestionMark(5)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_013")
		}
	case 8388608:
		if level < 6 && st.GetInt("sit") == 8388608 {
			setInt(st, "sit", 1)
			p.EnableTutorialEvent(0)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_018")
			return "tutorial_21z.htm"
		}
	}
	return ""
}

// levelReached is the client event of a level gain: the step of the level
// reached, once per level.
func levelReached(s *script.Script, st *script.QuestState, p *script.Player, classID, level int32) {
	qLvl := st.GetInt("lvl")
	switch level {
	case 5:
		if qLvl < 5 {
			setInt(st, "lvl", 5)
			p.ShowQuestionMark(9)
			s.PlaySound(p, script.SoundTutorial)
			if p.IsMageClass() {
				p.PlayTutorialVoice("tutorial_voice_015")
			} else {
				p.PlayTutorialVoice("tutorial_voice_014")
			}
		}
	case 6:
		if qLvl < 6 {
			setInt(st, "lvl", 6)
			p.ShowQuestionMark(24)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_020")
		}
	case 7:
		if qLvl < 7 && p.IsMageClass() {
			setInt(st, "lvl", 7)
			p.ShowQuestionMark(11)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_019")
			if evt, ok := eventOf(classID); ok {
				addMarker(p, evt.ce47Loc)
			}
		}
	case 9:
		if qLvl < 9 && classID == 0 {
			setInt(st, "lvl", 9)
			p.ShowQuestionMark(25)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_021")
		}
	case 10:
		if qLvl < 10 && classID != 0 && classID != 44 && classID != 49 {
			setInt(st, "lvl", 10)
			p.ShowQuestionMark(25)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_021")
		}
	case 12:
		// An orc mystic hears it whatever level it was told of last.
		if (qLvl < 12 && classID == 44) || classID == 49 {
			setInt(st, "lvl", 12)
			p.ShowQuestionMark(25)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_021")
		}
	case 15:
		if qLvl < 15 {
			setInt(st, "lvl", 15)
			p.ShowQuestionMark(17)
			s.PlaySound(p, script.SoundTutorial)
		}
	case 19:
		if qLvl < 19 {
			setInt(st, "lvl", 19)
			p.ShowQuestionMark(13)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_022")
		}
	case 35:
		if qLvl < 35 {
			setInt(st, "lvl", 35)
			p.ShowQuestionMark(15)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_023")
		}
	case 75:
		if qLvl < 75 {
			setInt(st, "lvl", 75)
			p.ShowQuestionMark(16)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_024")
		}
	}
}

// questionMark answers a click on the question mark QM<n>: the page it
// opens, "" for none.
func questionMark(st *script.QuestState, p *script.Player, classID int32, n int64) string {
	switch n {
	case 1:
		evt, ok := eventOf(classID)
		if !ok {
			return ""
		}
		setInt(st, "Ex", -5)
		setInt(st, "ucMemo", 2)
		addMarker(p, evt.ce8Loc)
		p.PlayTutorialVoice("tutorial_voice_007")
		return evt.ce8Htm
	case 2:
		switch classID {
		case 0:
			return "tutorial_human_fighter008.htm"
		case 10:
			return "tutorial_human_mage008.htm"
		case 18, 25:
			return "tutorial_elf008.htm"
		case 31, 38:
			return "tutorial_delf008.htm"
		case 44, 49:
			return "tutorial_orc008.htm"
		case 53:
			return "tutorial_dwarven_fighter008.htm"
		}
	case 3:
		p.EnableTutorialEvent(1048576)
		return "tutorial_09.htm"
	case 4:
		return "tutorial_10.htm"
	case 5:
		evt, ok := eventOf(classID)
		if !ok {
			return ""
		}
		addMarker(p, evt.ce8Loc)
		return "tutorial_11.htm"
	case 7:
		setInt(st, "ucMemo", 5)
		return "tutorial_15.htm"
	case 8:
		return "tutorial_18.htm"
	case 9:
		evt, ok := eventOf(classID)
		if !ok {
			return ""
		}
		if evt.qmc9Loc != (point{}) {
			addMarker(p, evt.qmc9Loc)
		}
		return evt.qmc9Htm
	case 10:
		return "tutorial_19.htm"
	case 11:
		switch p.Race() {
		case player.RaceHuman:
			return "tutorial_mage020.htm"
		case player.RaceElf, player.RaceDarkElf:
			return "tutorial_mage_elf020.htm"
		case player.RaceOrc:
			return "tutorial_mage_orc020.htm"
		}
	case 12:
		setInt(st, "ucMemo", 4)
		return "tutorial_15.htm"
	case 13:
		evt, ok := eventOf(classID)
		if !ok {
			return ""
		}
		return evt.qmc35Htm
	case 15:
		return "tutorial_28.htm"
	case 16:
		return "tutorial_30.htm"
	case 17:
		return "tutorial_27.htm"
	case 19:
		return "tutorial_07.htm"
	case 22:
		return "tutorial_14.htm"
	case 23:
		return "tutorial_24.htm"
	case 24:
		evt, ok := eventOf(classID)
		if !ok {
			return ""
		}
		return evt.qmc24Htm
	case 25:
		switch classID {
		case 0:
			return "tutorial_newbie002a.htm"
		case 10:
			return "tutorial_newbie002b.htm"
		case 18, 25:
			return "tutorial_newbie002c.htm"
		case 31:
			return "tutorial_newbie002e.htm"
		case 38:
			return "tutorial_newbie002d.htm"
		case 44, 49:
			return "tutorial_newbie002f.htm"
		case 53:
			return "tutorial_newbie002g.htm"
		}
	case 26:
		if p.IsMageClass() && classID != 49 {
			return "tutorial_newbie004a.htm"
		}
		return "tutorial_newbie004b.htm"
	}
	return ""
}
