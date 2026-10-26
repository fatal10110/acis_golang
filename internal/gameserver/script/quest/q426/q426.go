// Package q426 is the quest Quest for Fishing Shot: any member of the
// fishing guild lets a player hunt for sweet fluid. The monsters of the
// land drop it to the party member whose quest is started. Ending the quest
// pays nothing and takes the fluid with it.
package q426

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q426_QuestForFishingShot"

// sweetFluid is the item the monsters drop.
const sweetFluid = 7586

// drop returns the chance, out of 1000, that the monster npcID drops one
// more sweet fluid, and the number of fluids it drops for sure.
func drop(npcID int32) (chance, base int32) {
	switch npcID {
	case 20005:
		return 45, 0
	case 20013:
		return 100, 0
	case 20016:
		return 100, 0
	case 20017:
		return 115, 0
	case 20030:
		return 105, 0
	case 20132:
		return 70, 0
	case 20038:
		return 135, 0
	case 20044:
		return 125, 0
	case 20046:
		return 100, 0
	case 20047:
		return 100, 0
	case 20050:
		return 140, 0
	case 20058:
		return 140, 0
	case 20063:
		return 160, 0
	case 20066:
		return 170, 0
	case 20070:
		return 180, 0
	case 20074:
		return 195, 0
	case 20077:
		return 205, 0
	case 20078:
		return 205, 0
	case 20079:
		return 205, 0
	case 20080:
		return 220, 0
	case 20081:
		return 370, 0
	case 20083:
		return 245, 0
	case 20084:
		return 255, 0
	case 20085:
		return 265, 0
	case 20087:
		return 565, 0
	case 20088:
		return 605, 0
	case 20089:
		return 250, 0
	case 20100:
		return 85, 0
	case 20103:
		return 110, 0
	case 20105:
		return 110, 0
	case 20115:
		return 190, 0
	case 20120:
		return 20, 0
	case 20131:
		return 45, 0
	case 20135:
		return 360, 0
	case 20157:
		return 235, 0
	case 20162:
		return 195, 0
	case 20176:
		return 280, 0
	case 20211:
		return 170, 0
	case 20225:
		return 160, 0
	case 20227:
		return 180, 0
	case 20230:
		return 260, 0
	case 20232:
		return 245, 0
	case 20234:
		return 290, 0
	case 20241:
		return 700, 0
	case 20267:
		return 215, 0
	case 20268:
		return 295, 0
	case 20269:
		return 255, 0
	case 20270:
		return 365, 0
	case 20271:
		return 295, 0
	case 20286:
		return 700, 0
	case 20308:
		return 110, 0
	case 20312:
		return 45, 0
	case 20317:
		return 20, 0
	case 20324:
		return 85, 0
	case 20333:
		return 100, 0
	case 20341:
		return 100, 0
	case 20346:
		return 85, 0
	case 20349:
		return 850, 0
	case 20356:
		return 165, 0
	case 20357:
		return 140, 0
	case 20363:
		return 70, 0
	case 20368:
		return 85, 0
	case 20371:
		return 100, 0
	case 20386:
		return 85, 0
	case 20389:
		return 90, 0
	case 20403:
		return 110, 0
	case 20404:
		return 95, 0
	case 20433:
		return 100, 0
	case 20436:
		return 140, 0
	case 20448:
		return 45, 0
	case 20456:
		return 20, 0
	case 20463:
		return 85, 0
	case 20470:
		return 45, 0
	case 20471:
		return 85, 0
	case 20475:
		return 20, 0
	case 20478:
		return 110, 0
	case 20487:
		return 90, 0
	case 20511:
		return 100, 0
	case 20525:
		return 20, 0
	case 20528:
		return 100, 0
	case 20536:
		return 15, 0
	case 20537:
		return 15, 0
	case 20538:
		return 15, 0
	case 20539:
		return 15, 0
	case 20544:
		return 15, 0
	case 20550:
		return 300, 0
	case 20551:
		return 300, 0
	case 20552:
		return 650, 0
	case 20553:
		return 335, 0
	case 20554:
		return 390, 0
	case 20555:
		return 350, 0
	case 20557:
		return 390, 0
	case 20559:
		return 420, 0
	case 20560:
		return 440, 0
	case 20562:
		return 485, 0
	case 20573:
		return 545, 0
	case 20575:
		return 645, 0
	case 20630:
		return 350, 0
	case 20632:
		return 475, 0
	case 20634:
		return 960, 0
	case 20636:
		return 495, 0
	case 20638:
		return 540, 0
	case 20641:
		return 680, 0
	case 20643:
		return 660, 0
	case 20644:
		return 645, 0
	case 20659:
		return 440, 0
	case 20661:
		return 575, 0
	case 20663:
		return 525, 0
	case 20665:
		return 680, 0
	case 20667:
		return 730, 0
	case 20766:
		return 210, 0
	case 20781:
		return 270, 0
	case 20783:
		return 140, 0
	case 20784:
		return 155, 0
	case 20786:
		return 170, 0
	case 20788:
		return 325, 0
	case 20790:
		return 390, 0
	case 20792:
		return 620, 0
	case 20794:
		return 635, 0
	case 20796:
		return 640, 0
	case 20798:
		return 850, 0
	case 20800:
		return 740, 0
	case 20802:
		return 900, 0
	case 20804:
		return 775, 0
	case 20806:
		return 805, 0
	case 20833:
		return 455, 0
	case 20834:
		return 680, 0
	case 20836:
		return 785, 0
	case 20837:
		return 835, 0
	case 20839:
		return 430, 0
	case 20841:
		return 460, 0
	case 20845:
		return 605, 0
	case 20847:
		return 570, 0
	case 20849:
		return 585, 0
	case 20936:
		return 290, 0
	case 20937:
		return 315, 0
	case 20939:
		return 385, 0
	case 20940:
		return 500, 0
	case 20941:
		return 460, 0
	case 20943:
		return 345, 0
	case 20944:
		return 335, 0
	case 21100:
		return 125, 0
	case 21101:
		return 155, 0
	case 21103:
		return 215, 0
	case 21105:
		return 310, 0
	case 21107:
		return 600, 0
	case 21117:
		return 120, 0
	case 21023:
		return 170, 0
	case 21024:
		return 175, 0
	case 21025:
		return 185, 0
	case 21026:
		return 200, 0
	case 21034:
		return 195, 0
	case 21125:
		return 12, 0
	case 21263:
		return 650, 0
	case 21520:
		return 880, 0
	case 21526:
		return 970, 0
	case 21536:
		return 985, 0
	case 21602:
		return 555, 0
	case 21603:
		return 750, 0
	case 21605:
		return 620, 0
	case 21606:
		return 875, 0
	case 21611:
		return 590, 0
	case 21612:
		return 835, 0
	case 21617:
		return 615, 0
	case 21618:
		return 875, 0
	case 21635:
		return 775, 0
	case 21638:
		return 165, 0
	case 21639:
		return 185, 0
	case 21641:
		return 195, 0
	case 21644:
		return 170, 0
	case 20579:
		return 420, 1
	case 20639:
		return 280, 1
	case 20646:
		return 145, 1
	case 20648:
		return 120, 1
	case 20650:
		return 460, 1
	case 20651:
		return 260, 1
	case 20652:
		return 335, 1
	case 20657:
		return 630, 1
	case 20658:
		return 570, 1
	case 20808:
		return 50, 1
	case 20809:
		return 865, 1
	case 20832:
		return 700, 1
	case 20979:
		return 980, 1
	case 20991:
		return 665, 1
	case 20994:
		return 590, 1
	case 21261:
		return 170, 1
	case 21508:
		return 100, 1
	case 21510:
		return 280, 1
	case 21511:
		return 995, 1
	case 21512:
		return 995, 1
	case 21514:
		return 185, 1
	case 21516:
		return 495, 1
	case 21517:
		return 495, 1
	case 21518:
		return 255, 1
	case 21636:
		return 950, 1
	case 20655:
		return 110, 2
	case 20656:
		return 150, 2
	case 20772:
		return 105, 2
	case 20810:
		return 50, 2
	case 20812:
		return 490, 2
	case 20814:
		return 775, 2
	case 20816:
		return 875, 2
	case 20819:
		return 280, 2
	case 20955:
		return 670, 2
	case 20978:
		return 555, 2
	case 21058:
		return 355, 2
	case 21060:
		return 45, 2
	case 21075:
		return 110, 2
	case 21078:
		return 610, 2
	case 21081:
		return 955, 2
	case 21264:
		return 920, 2
	case 20815:
		return 205, 3
	case 20822:
		return 100, 3
	case 20824:
		return 665, 3
	case 20825:
		return 620, 3
	case 20983:
		return 205, 3
	case 21314:
		return 145, 3
	case 21316:
		return 235, 3
	case 21318:
		return 280, 3
	case 21320:
		return 355, 3
	case 21322:
		return 430, 3
	case 21376:
		return 280, 3
	case 21378:
		return 375, 3
	case 21380:
		return 375, 3
	case 21387:
		return 640, 3
	case 21393:
		return 935, 3
	case 21395:
		return 855, 3
	case 21652:
		return 375, 3
	case 21655:
		return 640, 3
	case 21657:
		return 935, 3
	case 20828:
		return 935, 4
	case 21061:
		return 530, 4
	case 21069:
		return 825, 4
	case 21382:
		return 125, 4
	case 21384:
		return 400, 4
	case 21390:
		return 750, 4
	case 21654:
		return 400, 4
	case 21656:
		return 750, 4
	case 20829:
		return 115, 6
	case 20859:
		return 890, 8
	case 21066:
		return 5, 5
	case 21068:
		return 565, 11
	case 21071:
		return 400, 12
	}
	return 0, 0
}

// New returns the quest.
func New() script.Script {
	guild := []int32{
		31562, 31563, 31564, 31565, 31566, 31567, 31568, 31569, 31570, 31571, 31572, 31573,
		31574, 31575, 31576, 31577, 31578, 31579, 31696, 31697, 31989, 32007,
	}
	return script.Script{
		Title:   "Quest for Fishing Shot",
		QuestID: 426,
		Items:   []int32{sweetFluid},
		Bind: script.Bindings{
			script.EventQuestStart: guild,
			script.EventTalked:     guild,
			script.EventMyDying: {
				20005, 20013, 20016, 20017, 20030, 20132, 20038, 20044, 20046, 20047,
				20050, 20058, 20063, 20066, 20070, 20074, 20077, 20078, 20079, 20080,
				20081, 20083, 20084, 20085, 20087, 20088, 20089, 20100, 20103, 20105,
				20115, 20120, 20131, 20135, 20157, 20162, 20176, 20211, 20225, 20227,
				20230, 20232, 20234, 20241, 20267, 20268, 20269, 20270, 20271, 20286,
				20308, 20312, 20317, 20324, 20333, 20341, 20346, 20349, 20356, 20357,
				20363, 20368, 20371, 20386, 20389, 20403, 20404, 20433, 20436, 20448,
				20456, 20463, 20470, 20471, 20475, 20478, 20487, 20511, 20525, 20528,
				20536, 20537, 20538, 20539, 20544, 20550, 20551, 20552, 20553, 20554,
				20555, 20557, 20559, 20560, 20562, 20573, 20575, 20630, 20632, 20634,
				20636, 20638, 20641, 20643, 20644, 20659, 20661, 20663, 20665, 20667,
				20766, 20781, 20783, 20784, 20786, 20788, 20790, 20792, 20794, 20796,
				20798, 20800, 20802, 20804, 20806, 20833, 20834, 20836, 20837, 20839,
				20841, 20845, 20847, 20849, 20936, 20937, 20939, 20940, 20941, 20943,
				20944, 21100, 21101, 21103, 21105, 21107, 21117, 21023, 21024, 21025,
				21026, 21034, 21125, 21263, 21520, 21526, 21536, 21602, 21603, 21605,
				21606, 21611, 21612, 21617, 21618, 21635, 21638, 21639, 21641, 21644,
				20579, 20639, 20646, 20648, 20650, 20651, 20652, 20657, 20658, 20808,
				20809, 20832, 20979, 20991, 20994, 21261, 21508, 21510, 21511, 21512,
				21514, 21516, 21517, 21518, 21636, 20655, 20656, 20772, 20810, 20812,
				20814, 20816, 20819, 20955, 20978, 21058, 21060, 21075, 21078, 21081,
				21264, 20815, 20822, 20824, 20825, 20983, 21314, 21316, 21318, 21320,
				21322, 21376, 21378, 21380, 21387, 21393, 21395, 21652, 21655, 21657,
				20828, 21061, 21069, 21382, 21384, 21390, 21654, 21656, 20829, 20859,
				21066, 21068, 21071,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or ends it.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "08.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// onTalk offers the quest to a player with no state of it, who gets one.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		st = s.NewQuestState(e.Player)
	}
	switch st.Status() {
	case questlog.StatusCreated:
		htmltext = "01.htm"
	case questlog.StatusStarted:
		if e.Player.HasItem(sweetFluid) {
			htmltext = "05.htm"
		} else {
			htmltext = "04.htm"
		}
	}
	return htmltext
}

// onMyDying gives a started party member the sweet fluid of the monster:
// its sure fluids, and one more at its chance. The sound comes first.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	chance, count := drop(e.NPC.NpcID())
	if int32(s.Rnd(1000)) <= chance {
		count++
	}
	if count != 0 {
		s.PlaySound(st.Player(), script.SoundItemGet)
		s.RewardItems(st.Player(), sweetFluid, count)
	}
}
