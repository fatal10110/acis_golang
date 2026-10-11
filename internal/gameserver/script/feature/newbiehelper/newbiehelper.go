// Package newbiehelper is the newbie helpers and guides of the starting
// villages: the recommendation letter and beginner shots for a new
// player's first steps, the guides' advice pages, newbie buffs, blessing,
// radar marks and travel tokens, and the tutorial gremlin's dying drop.
package newbiehelper

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	questName         = "NewbieHelper"
	questNameTutorial = "Tutorial"
)

// Quest items.
const (
	recommendation01 = 1067
	recommendation02 = 1068
	leafOfMothertree = 1069
	bloodOfJundin    = 1070
	licenseOfMiner   = 1498
	voucherOfFlame   = 1496
)

// Items rewarded.
const (
	soulshotNoGradeForBeginners   = 5789
	spiritshotNoGradeForBeginners = 5790
	blueGemstone                  = 6353
	newbieTravelToken             = 8542
)

// The starting classes the helper tells apart, and the orc shaman.
const (
	humanFighter   = 0
	humanMystic    = 10
	elvenFighter   = 18
	elvenMystic    = 25
	darkFighter    = 31
	darkMystic     = 38
	orcFighter     = 44
	orcMystic      = 49
	orcShaman      = 50
	dwarvenFighter = 53
)

// blessingOfProtection is the blessing a guide casts on a player below the
// second class change.
func blessingOfProtection() skill.Ref { return skill.Ref{ID: 5182, Level: 1} }

// teleportLoc is the place a travel token named key leads to.
func teleportLoc(key string) (script.Loc, bool) {
	switch key {
	case "30598":
		return script.Loc{X: -84053, Y: 243343, Z: -3729}, true
	case "30599":
		return script.Loc{X: 45470, Y: 48328, Z: -3059}, true
	case "30600":
		return script.Loc{X: 12160, Y: 16554, Z: -4583}, true
	case "30601":
		return script.Loc{X: 115594, Y: -177993, Z: -912}, true
	case "30602":
		return script.Loc{X: -45067, Y: -113563, Z: -199}, true
	}
	return script.Loc{}, false
}

// newbieGuideLoc is the place of the newbie guide npcID marks on the radar;
// nil for any other NPC.
func newbieGuideLoc(npcID int32) *script.Loc {
	switch npcID {
	case 30008, 30017:
		return &script.Loc{X: -84058, Y: 243239, Z: -3730}
	case 30129:
		return &script.Loc{X: 12116, Y: 16666, Z: -4610}
	case 30370:
		return &script.Loc{X: 45491, Y: 48359, Z: -3086}
	case 30528:
		return &script.Loc{X: 115632, Y: -177996, Z: -912}
	case 30573:
		return &script.Loc{X: -45067, Y: -113549, Z: -235}
	}
	return nil
}

// radars are the NPCs a guide marks on the radar when asked where they are.
func radars() []int32 {
	return []int32{
		// Talking Island
		30006, 30039, 30040, 30041, 30042, 30043, 30044, 30045, 30046, 30283, 30003, 30004, 30001, 30002,
		30031, 30033, 30035, 30032, 30036, 30026, 30027, 30029, 30028, 30054, 30055, 30005, 30048, 30312,
		30368, 30049, 30047, 30497, 30050, 30311, 30051,
		// Dark Elf Village
		30134, 30224, 30348, 30355, 30347, 30432, 30356, 30349, 30346, 30433, 30357, 30431, 30430, 30307,
		30138, 30137, 30135, 30136, 30143, 30360, 30145, 30135, 30144, 30358, 30359, 30141, 30139, 30140,
		30350, 30421, 30419, 30130, 30351, 30353, 30354,
		// Elven Village
		30146, 30285, 30284, 30221, 30217, 30219, 30220, 30218, 30216, 30363, 30149, 30150, 30148, 30147,
		30155, 30156, 30157, 30158, 30154, 30153, 30152, 30151, 30423, 30414, 30361, 31853, 30223, 30362,
		30222, 30371, 31852,
		// Dwarven Village
		30540, 30541, 30542, 30543, 30544, 30545, 30546, 30547, 30548, 30531, 30532, 30533, 30534, 30535,
		30536, 30525, 30526, 30527, 30518, 30519, 30516, 30517, 30520, 30521, 30522, 30523, 30524, 30537,
		30650, 30538, 30539, 30671, 30651, 30550, 30554, 30553,
		// Orc Village
		30576, 30577, 30578, 30579, 30580, 30581, 30582, 30583, 30584, 30569, 30570, 30571, 30572, 30564,
		30560, 30561, 30558, 30559, 30562, 30563, 30565, 30566, 30567, 30568, 30585, 30587,
	}
}

// New returns the feature, its guides casting from buffs.
func New(buffs *skill.NewbieBuffTable) script.Script {
	f := feature{buffs: buffs}
	guides := []int32{30009, 30019, 30131, 30400, 30530, 30575, 30008, 30017, 30129, 30370, 30528, 30573, 30598, 30599, 30600, 30601, 30602, 31076, 31077}
	return script.Script{
		QuestID: -1,
		Dir:     "feature",
		Bind: script.Bindings{
			script.EventTalked:    guides,
			script.EventFirstTalk: guides,
			script.EventMyDying:   {18342},
		},
		Hooks: script.Hooks{
			OnTimer:     onTimer,
			OnEvent:     f.onAdvEvent,
			OnFirstTalk: onFirstTalk,
			OnMyDying:   onMyDying,
		},
	}
}

// feature holds what the hooks read: the immutable newbie buff table.
type feature struct{ buffs *skill.NewbieBuffTable }

func onTimer(s *script.Script, e script.Timer) string {
	p := e.Player
	st := s.QuestState(p, questName)
	qs := s.QuestState(p, questNameTutorial)
	if st == nil || qs == nil {
		return ""
	}
	htmltext := e.Name
	if strings.EqualFold(e.Name, "TimerEx_NewbieHelper") {
		switch ex := qs.GetInt("Ex"); ex {
		case 0:
			switch p.ClassID() {
			case humanFighter, elvenFighter, darkFighter, orcFighter, dwarvenFighter:
				p.PlayTutorialVoice("tutorial_voice_009a")
			case humanMystic, elvenMystic, darkMystic:
				p.PlayTutorialVoice("tutorial_voice_009b")
			case orcMystic:
				p.PlayTutorialVoice("tutorial_voice_009c")
			}
			qs.Set("Ex", strconv.Itoa(1))
		case 3:
			switch p.ClassID() {
			case humanFighter:
				p.PlayTutorialVoice("tutorial_voice_010a")
			case humanMystic:
				p.PlayTutorialVoice("tutorial_voice_010b")
			case elvenFighter, elvenMystic:
				p.PlayTutorialVoice("tutorial_voice_010c")
			case darkFighter, darkMystic:
				p.PlayTutorialVoice("tutorial_voice_010d")
			case orcFighter, orcMystic:
				p.PlayTutorialVoice("tutorial_voice_010e")
			case dwarvenFighter:
				p.PlayTutorialVoice("tutorial_voice_010f")
			}
			qs.Set("Ex", strconv.Itoa(4))
		}
		return ""
	}
	if strings.EqualFold(e.Name, "TimerEx_GrandMaster") {
		if qs.GetInt("Ex") >= 4 {
			p.ShowQuestionMark(7)
			s.PlaySound(p, script.SoundTutorial)
			p.PlayTutorialVoice("tutorial_voice_025")
		}
		return ""
	}
	return htmltext
}

func (f feature) onAdvEvent(s *script.Script, e script.Event) string {
	p, n, event := e.Player, e.NPC, e.Name
	st := s.QuestState(p, questName)
	qs := s.QuestState(p, questNameTutorial)
	if st == nil || qs == nil {
		return ""
	}
	htmltext := event
	switch {
	case strings.EqualFold(event, "30008-03.htm") || strings.EqualFold(event, "30017-03.htm") || strings.EqualFold(event, "30129-03.htm") ||
		strings.EqualFold(event, "30370-03.htm") || strings.EqualFold(event, "30528-03.htm") || strings.EqualFold(event, "30573-03.htm"):
		// The guide of another NPC has no place: the event aborts here.
		loc := newbieGuideLoc(n.NpcID())
		p.AddRadarMarker(int32(loc.X), int32(loc.Y), int32(loc.Z))

		itemID := itemIDOf(n.NpcID())
		if p.HasItem(itemID) && st.GetInt("onlyone") == 0 {
			s.TakeItems(p, itemID, 1)
			s.RewardExpAndSp(p, 0, 50)

			s.StartTimer("TimerEx_GrandMaster", nil, p, 60000*time.Millisecond)

			if qs.GetInt("Ex") <= 3 {
				qs.Set("Ex", strconv.Itoa(4))
			}

			if p.IsMageClass() && p.ClassID() != orcMystic {
				p.PlayTutorialVoice("tutorial_voice_027")
				s.GiveItems(p, spiritshotNoGradeForBeginners, 100)
			} else {
				p.PlayTutorialVoice("tutorial_voice_026")
				s.GiveItems(p, soulshotNoGradeForBeginners, 200)
			}

			st.Unset("step")
			st.Set("onlyone", strconv.Itoa(1))
		}
	case strings.HasPrefix(event, "AskAdvice"):
		switch n.Race() {
		case npc.RaceHuman:
			if p.Race() != player.RaceHuman {
				return "human/guide_human_cnacelot003.htm"
			}
			htmltext = "human/guide_human_cnacelot"
		case npc.RaceElf:
			if p.Race() != player.RaceElf {
				return "elf/guide_elf_roios003.htm"
			}
			htmltext = "elf/guide_elf_roios"
		case npc.RaceDarkElf:
			if p.Race() != player.RaceDarkElf {
				return "darkelf/guide_delf_frankia003.htm"
			}
			htmltext = "darkelf/guide_delf_frankia"
		case npc.RaceOrc:
			if p.Race() != player.RaceOrc {
				return "orc/guide_orc_tanai003.htm"
			}
			htmltext = "orc/guide_orc_tanai"
		case npc.RaceDwarf:
			if p.Race() != player.RaceDwarf {
				return "dwarf/guide_dwarf_gullin003.htm"
			}
			htmltext = "dwarf/guide_dwarf_gullin"
		}

		level := p.Level()
		switch {
		// Already too high or different class level.
		case level >= 20 || p.ClassLevel() != 0:
			htmltext += "002.htm"
		// Fighter related pages.
		case !p.IsMageClass():
			switch {
			case level <= 5:
				htmltext += "_f05.htm"
			case level <= 10:
				htmltext += "_f10.htm"
			case level <= 15:
				htmltext += "_f15.htm"
			default:
				htmltext += "_f20.htm"
			}
		// Mage related pages.
		case level <= 7:
			htmltext += "_m07.htm"
		case level <= 14:
			htmltext += "_m14.htm"
		default:
			htmltext += "_m20.htm"
		}
	case strings.HasPrefix(event, "SupportMagic"):
		// A cursed weapon wielder is not buffed.
		if p.CursedWeaponEquipped() {
			return ""
		}
		// Orc mystics and shamans get the fighter buffs, although they are
		// mage classes.
		isMage := p.IsMageClass() && p.ClassID() != orcMystic && p.ClassID() != orcShaman
		playerLevel := int(p.Level())
		switch {
		case playerLevel < f.buffs.LowestBuffLevel(isMage):
			htmltext = "guide_for_newbie002.htm"
		case !p.IsNewbie(false):
			htmltext = "guide_for_newbie003.htm"
		default:
			// Each buff is cast a second after the one before.
			for i, buff := range f.buffs.ValidBuffs(isMage, playerLevel) {
				p.After(1000*time.Millisecond*time.Duration(i), func() { s.CallSkill(p, p, buff.Skill) })
			}
			return ""
		}
	case event == "NewbieToken":
		if !p.IsNewbie(false) {
			htmltext = invalidHtm(n)
		} else {
			n.ShowTeleportWindow(p, travel.KindNewbieToken)
			return ""
		}
	case strings.HasPrefix(event, "NewbieToken"):
		if !p.HasAtLeastOneItem(newbieTravelToken) {
			htmltext = "newbie_guide_no_token.htm"
		} else {
			// The places are keyed by the guide's id alone, so no
			// NewbieToken<id> event finds one.
			if loc, ok := teleportLoc(event); ok {
				s.TakeItems(p, newbieTravelToken, 1)
				p.TeleportTo(loc, 0)
			}
			return ""
		}
	case strings.HasPrefix(event, "GiveBlessing"):
		if p.Level() > 39 || p.ClassLevel() >= 2 {
			htmltext = invalidHtm(n)
		} else {
			p.After(1000*time.Millisecond, func() { s.CallSkill(p, p, blessingOfProtection()) })
			return ""
		}
	case strings.HasPrefix(event, "NpcLocationInfo"):
		// A malformed id, or one outside the int32 range, aborts the event
		// here.
		npcID, err := strconv.ParseInt(event[16:], 10, 32)
		if err != nil {
			panic(err)
		}
		id := int32(npcID)
		if !slices.Contains(radars(), id) {
			return ""
		}
		if target := s.WorldNPC(id); target != nil {
			at := target.SpawnLocation()
			p.AddRadarMarker(int32(at.X), int32(at.Y), int32(at.Z))
		}
		htmltext = "newbie_guide_move_to_loc.htm"
	}
	return htmltext
}

func onFirstTalk(s *script.Script, e script.FirstTalk) string {
	p, n := e.Player, e.NPC
	qs := s.QuestState(p, questNameTutorial)
	if qs == nil {
		return ""
	}
	st := s.QuestState(p, questName)
	if st == nil {
		st = s.NewQuestState(p)
	}
	npcID := n.NpcID()
	page := strconv.Itoa(int(npcID))

	if npcID == 31076 || npcID == 31077 {
		return page + ".htm"
	}

	if npcID >= 30598 && npcID <= 30602 {
		if !st.IsCompleted() {
			if p.IsMageClass() {
				p.PlayTutorialVoice("tutorial_voice_027")
				s.GiveItems(p, spiritshotNoGradeForBeginners, 100)
			} else {
				p.PlayTutorialVoice("tutorial_voice_026")
				s.GiveItems(p, soulshotNoGradeForBeginners, 200)
			}
			s.GiveItems(p, newbieTravelToken, 12)
			st.SetStatus(questlog.StatusCompleted)
		}
		return page + ".htm"
	}

	if npcID == 30008 || npcID == 30017 || npcID == 30129 || npcID == 30370 || npcID == 30528 || npcID == 30573 {
		if st.IsCompleted() {
			return page + "-04.htm"
		}
		step := st.GetInt("step")
		if step == 0 {
			n.ShowChatWindow(p)
			return ""
		}
		if step == 1 {
			return page + "-01.htm"
		}
		if step == 2 {
			return page + "-02.htm"
		}
		if step == 3 || qs.GetInt("ucMemo") >= 3 {
			return page + "-04.htm"
		}
	}

	if npcID == 30009 || npcID == 30019 || npcID == 30131 || npcID == 30400 || npcID == 30530 || npcID == 30575 {
		if p.Level() >= 10 || st.GetInt("onlyone") == 1 {
			return "newbiehelper_03.htm"
		}
		htmltext := "newbiehelper_fig_01.htm"
		switch step := st.GetInt("step"); {
		case step == 0:
			qs.Set("Ex", strconv.Itoa(0))

			st.Set("step", strconv.Itoa(1))
			st.SetStatus(questlog.StatusStarted)

			s.StartTimer("TimerEx_NewbieHelper", nil, p, 30000*time.Millisecond)

			if p.IsMageClass() {
				if p.ClassID() == orcMystic {
					htmltext = "newbiehelper_mage_02.htm"
				} else {
					htmltext = "newbiehelper_mage_01.htm"
				}
			}
		case step == 1 && qs.GetInt("Ex") <= 2:
			switch {
			case p.HasAtLeastOneItem(blueGemstone):
				qs.Set("Ex", strconv.Itoa(3))
				qs.Set("ucMemo", strconv.Itoa(3))

				st.Set("step", strconv.Itoa(2))
				s.TakeItems(p, blueGemstone, -1)
				s.GiveItems(p, itemIDOf(npcID), 1)

				s.StartTimer("TimerEx_NewbieHelper", nil, p, 30000*time.Millisecond)

				if p.IsMageClass() && p.ClassID() != orcMystic {
					if npcID == 30009 || npcID == 30530 {
						htmltext = page + "-03.htm"
					} else {
						htmltext = page + "-03a.htm"
					}
					s.GiveItems(p, spiritshotNoGradeForBeginners, 100)
					p.PlayTutorialVoice("tutorial_voice_027")
				} else {
					htmltext = page + "-03.htm"
					s.GiveItems(p, soulshotNoGradeForBeginners, 200)
					p.PlayTutorialVoice("tutorial_voice_026")
				}
			case p.IsMageClass():
				if p.ClassID() == orcMystic {
					htmltext = "newbiehelper_mage_02a.htm"
				} else {
					htmltext = "newbiehelper_mage_01a.htm"
				}
			default:
				htmltext = "newbiehelper_fig_01a.htm"
			}
		case step == 2:
			htmltext = page + "-04.htm"
		}
		return htmltext
	}
	return ""
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)

	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	qs := s.QuestState(p, questNameTutorial)
	if qs == nil {
		return
	}

	ex := qs.GetInt("Ex")
	if ex <= 1 {
		qs.Set("Ex", strconv.Itoa(2))
		p.ShowQuestionMark(3)
		p.PlayTutorialVoice("tutorial_voice_011")
	}

	if ex <= 2 && qs.GetInt("Gemstone") == 0 && s.Rnd(100) < 25 {
		e.NPC.DropItem(p, blueGemstone, 1)
		s.PlaySound(p, script.SoundTutorial)
	}
}

// invalidHtm is the page a guide of n's race answers a player it cannot
// help with; "" for a guide of another race.
func invalidHtm(n *script.NPC) string {
	switch n.Race() {
	case npc.RaceHuman:
		return "human/guide_human_cnacelot002.htm"
	case npc.RaceElf:
		return "elf/guide_elf_roios002.htm"
	case npc.RaceDarkElf:
		return "darkelf/guide_delf_frankia002.htm"
	case npc.RaceOrc:
		return "orc/guide_orc_tanai002.htm"
	case npc.RaceDwarf:
		return "dwarf/guide_dwarf_gullin002.htm"
	}
	return ""
}

// itemIDOf is the recommendation item of the helper or guide npcID; 0 for
// any other NPC.
func itemIDOf(npcID int32) int32 {
	switch npcID {
	case 30008, 30009:
		return recommendation01
	case 30017, 30019:
		return recommendation02
	case 30129, 30131:
		return bloodOfJundin
	case 30370, 30400:
		return leafOfMothertree
	case 30528, 30530:
		return licenseOfMiner
	case 30573, 30575:
		return voucherOfFlame
	}
	return 0
}
