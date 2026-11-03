// Package q619 is the quest Relics of the Old Empire: the Ghost of an
// Adventurer, for a character of level 74 or more, asks for broken relic
// parts from the Four Sepulchers and the Imperial Tomb, where the
// monsters may drop several at once. A thousand of them pay a random
// recipe of an S grade weapon; the Imperial Tomb's monsters also drop the
// entrance pass to the sepulchers.
package q619

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q619_RelicsOfTheOldEmpire"

// NPC.
const ghostOfAdventurer = 31538

// Items.
const (
	brokenRelicPart            = 7254
	entrancePassToTheSepulcher = 7075
)

// rewards returns the recipes the ghost hands out.
func rewards() []int32 {
	return []int32{6881, 6883, 6885, 6887, 6891, 6893, 6895, 6897, 6899, 7580}
}

// sepulchersChance returns the Four Sepulchers monster npcID's drop chance
// of a relic part. Above a million it drops more than one.
func sepulchersChance(npcID int32) int32 {
	switch npcID {
	case 18120:
		return 1280000
	case 18121:
		return 1210000
	case 18122:
		return 930000
	case 18123:
		return 1280000
	case 18124:
		return 1210000
	case 18125:
		return 930000
	case 18126:
		return 1280000
	case 18127:
		return 1210000
	case 18128:
		return 930000
	case 18129:
		return 1280000
	case 18130:
		return 1210000
	case 18131:
		return 930000
	case 18132:
		return 1300000
	case 18133:
		return 1200000
	case 18134:
		return 900000
	case 18135:
		return 1200000
	case 18136:
		return 1200000
	case 18137:
		return 890000
	case 18138:
		return 1190000
	case 18139:
		return 1170000
	case 18140:
		return 1190000
	case 18141:
		return 760000
	case 18142:
		return 760000
	case 18143:
		return 760000
	case 18144:
		return 760000
	case 18145:
		return 650000
	case 18146:
		return 660000
	case 18147:
		return 620000
	case 18148:
		return 720000
	case 18149:
		return 630000
	case 18166:
		return 920000
	case 18167:
		return 900000
	case 18168:
		return 930000
	case 18169:
		return 900000
	case 18170:
		return 900000
	case 18171:
		return 940000
	case 18172:
		return 890000
	case 18173:
		return 990000
	case 18174:
		return 1220000
	case 18175:
		return 930000
	case 18176:
		return 990000
	case 18177:
		return 1220000
	case 18178:
		return 930000
	case 18179:
		return 990000
	case 18180:
		return 1220000
	case 18181:
		return 930000
	case 18182:
		return 990000
	case 18183:
		return 1220000
	case 18184:
		return 930000
	case 18185:
		return 1230000
	case 18186:
		return 1240000
	case 18187:
		return 1200000
	case 18188:
		return 900000
	case 18189:
		return 1200000
	case 18190:
		return 1200000
	case 18191:
		return 890000
	case 18192:
		return 1190000
	case 18193:
		return 1170000
	case 18194:
		return 1200000
	case 18195:
		return 910000
	case 18212:
		return 3790000
	case 18213:
		return 3790000
	case 18214:
		return 3790000
	case 18215:
		return 3790000
	case 18216:
		return 3790000
	case 18217:
		return 3790000
	case 18218:
		return 3790000
	case 18219:
		return 3790000
	case 18220:
		return 1240000
	case 18221:
		return 1270000
	case 18222:
		return 1210000
	case 18223:
		return 900000
	case 18224:
		return 1220000
	case 18225:
		return 1210000
	case 18226:
		return 890000
	case 18227:
		return 1530000
	case 18228:
		return 1150000
	case 18229:
		return 1190000
	case 18230:
		return 1490000
	}
	return 0
}

// tombChance returns the Imperial Tomb monster npcID's drop chance of a
// relic part, and whether npcID is a monster of the Imperial Tomb.
func tombChance(npcID int32) (int32, bool) {
	switch npcID {
	case 21396:
		return 1510000, true
	case 21397:
		return 500000, true
	case 21398:
		return 950000, true
	case 21399:
		return 840000, true
	case 21400:
		return 760000, true
	case 21401:
		return 670000, true
	case 21402:
		return 690000, true
	case 21403:
		return 800000, true
	case 21404:
		return 900000, true
	case 21405:
		return 640000, true
	case 21406:
		return 870000, true
	case 21407:
		return 560000, true
	case 21408:
		return 820000, true
	case 21409:
		return 920000, true
	case 21410:
		return 810000, true
	case 21411:
		return 660000, true
	case 21412:
		return 1060000, true
	case 21413:
		return 810000, true
	case 21414:
		return 1790000, true
	case 21415:
		return 800000, true
	case 21416:
		return 820000, true
	case 21417:
		return 1270000, true
	case 21418:
		return 660000, true
	case 21419:
		return 670000, true
	case 21420:
		return 820000, true
	case 21421:
		return 770000, true
	case 21422:
		return 880000, true
	case 21423:
		return 940000, true
	case 21424:
		return 1190000, true
	case 21425:
		return 1210000, true
	case 21426:
		return 1080000, true
	case 21427:
		return 740000, true
	case 21428:
		return 760000, true
	case 21429:
		return 800000, true
	case 21430:
		return 1100000, true
	case 21431:
		return 940000, true
	case 21432:
		return 1340000, true
	case 21433:
		return 1340000, true
	case 21434:
		return 1900000, true
	case 21435:
		return 1600000, true
	case 21436:
		return 1660000, true
	case 21437:
		return 690000, true
	case 21798:
		return 330000, true
	case 21799:
		return 610000, true
	case 21800:
		return 310000, true
	}
	return 0, false
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Relics of the Old Empire",
		QuestID: 619,
		Items:   []int32{brokenRelicPart},
		Bind: script.Bindings{
			script.EventQuestStart: {ghostOfAdventurer},
			script.EventTalked:     {ghostOfAdventurer},
			script.EventMyDying: {
				18120, 18121, 18122, 18123, 18124, 18125, 18126, 18127, 18128, 18129, 18130, 18131, 18132,
				18133, 18134, 18135, 18136, 18137, 18138, 18139, 18140, 18141, 18142, 18143, 18144, 18145,
				18146, 18147, 18148, 18149, 18166, 18167, 18168, 18169, 18170, 18171, 18172, 18173, 18174,
				18175, 18176, 18177, 18178, 18179, 18180, 18181, 18182, 18183, 18184, 18185, 18186, 18187,
				18188, 18189, 18190, 18191, 18192, 18193, 18194, 18195, 18212, 18213, 18214, 18215, 18216,
				18217, 18218, 18219, 18220, 18221, 18222, 18223, 18224, 18225, 18226, 18227, 18228, 18229,
				18230, 21396, 21397, 21398, 21399, 21400, 21401, 21402, 21403, 21404, 21405, 21406, 21407,
				21408, 21409, 21410, 21411, 21412, 21413, 21414, 21415, 21416, 21417, 21418, 21419, 21420,
				21421, 21422, 21423, 21424, 21425, 21426, 21427, 21428, 21429, 21430, 21431, 21432, 21433,
				21434, 21435, 21436, 21437, 21798, 21799, 21800,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, hands a thousand relic parts in for a random
// recipe, or quits the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31538-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31538-09.htm"):
		if e.Player.ItemCount(brokenRelicPart) >= 1000 {
			htmltext = "31538-09.htm"
			s.TakeItems(e.Player, brokenRelicPart, 1000)
			rs := rewards()
			s.GiveItems(e.Player, rs[s.Rnd(len(rs))], 1)
		} else {
			htmltext = "31538-06.htm"
		}
	case strings.EqualFold(e.Name, "31538-10.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		if e.Player.Level() < 74 {
			htmltext = "31538-02.htm"
		} else {
			htmltext = "31538-01.htm"
		}
	case questlog.StatusStarted:
		switch {
		case e.Player.ItemCount(brokenRelicPart) >= 1000:
			htmltext = "31538-04.htm"
		case e.Player.HasItem(entrancePassToTheSepulcher):
			htmltext = "31538-06.htm"
		default:
			htmltext = "31538-07.htm"
		}
	}
	return htmltext
}

// onMyDying drops relic parts to a party member who has the quest, and for
// an Imperial Tomb monster an entrance pass to the sepulchers after them.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	if chance, ok := tombChance(npcID); ok {
		s.DropItems(st.Player(), brokenRelicPart, 1, 0, chance)
		s.DropItems(st.Player(), entrancePassToTheSepulcher, 1, 0, 33333)
	} else {
		s.DropItems(st.Player(), brokenRelicPart, 1, 0, sepulchersChance(npcID))
	}
}
