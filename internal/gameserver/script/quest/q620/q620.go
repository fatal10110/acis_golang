// Package q620 is the quest Four Goblets (Nameless Spirit): the Four
// Sepulchers. A party with entrance passes opens a sepulcher through its
// keeper, the hall gatekeepers open the doors of its rooms against chapel keys,
// the monsters inside drop sealed boxes and the raid bosses the four goblets,
// and the ghosts of Wigoth and the Elmoreden chamberlain turn boxes and relic
// parts into rewards.
package q620

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q620_FourGoblets"

// NPCs: the ghosts and the nameless spirit, the first and last sepulcher
// keeper and hall gatekeeper, the three runs of room monsters that drop
// sealed boxes, the raid bosses that drop goblets and the privates a hall
// calls.
const (
	ghostOfWigoth1   = 31452
	namelessSpirit   = 31453
	ghostOfWigoth2   = 31454
	ghostChamberlain = 31919
	firstManager     = 31921
	lastManager      = 31924
	firstGatekeeper  = 31925
	lastGatekeeper   = 31944

	firstDropperA = 18120
	lastDropperA  = 18149
	firstDropperB = 18166
	lastDropperB  = 18195
	firstDropperC = 18212
	lastDropperC  = 18230

	halishaAlectia = 25339
	halishaTishas  = 25342
	halishaMekara  = 25346
	halishaMorigul = 25349

	privateStrong = 18244
	privateMiddle = 18245
	privateWeak   = 18246
)

// Timers and chat lines.
const (
	timerGateWatch       = "3000"
	timerGateClose       = "3001"
	timerWigothWatch     = "62001"
	sayGatekeeperCalling = 1000502
)

// Items.
const (
	brokenRelicPart    = 7254
	sealedBox          = 7255
	gobletOfAlectia    = 7256
	gobletOfTishas     = 7257
	gobletOfMekara     = 7258
	gobletOfMorigul    = 7259
	chapelKey          = 7260
	entrancePass       = 7075
	usedGravePass      = 7261
	antiqueBrooch      = 7262
	brokenRelicsNeeded = 1000
	adena              = 57
)

// sepulcherLoc returns where the keeper npcID takes a party to.
func sepulcherLoc(npcID int32) script.Loc {
	switch npcID {
	case 31921:
		return script.Loc{X: 181528, Y: -85583, Z: -7216}
	case 31922:
		return script.Loc{X: 179849, Y: -88990, Z: -7216}
	case 31923:
		return script.Loc{X: 173216, Y: -86195, Z: -7216}
	default:
		return script.Loc{X: 175615, Y: -82365, Z: -7216}
	}
}

// hallDoorID returns the door the hall gatekeeper npcID opens.
func hallDoorID(npcID int32) int32 {
	switch npcID {
	case 31925:
		return 25150012
	case 31926:
		return 25150013
	case 31927:
		return 25150014
	case 31928:
		return 25150015
	case 31929:
		return 25150016
	case 31930:
		return 25150002
	case 31931:
		return 25150003
	case 31932:
		return 25150004
	case 31933:
		return 25150005
	case 31934:
		return 25150006
	case 31935:
		return 25150032
	case 31936:
		return 25150033
	case 31937:
		return 25150034
	case 31938:
		return 25150035
	case 31939:
		return 25150036
	case 31940:
		return 25150022
	case 31941:
		return 25150023
	case 31942:
		return 25150024
	case 31943:
		return 25150025
	case 31944:
		return 25150026
	}
	return 0
}

// sealedBoxChance returns the chance, in millionths of a sealed box, the
// monster npcID drops; a chance above 1000000 drops more boxes.
func sealedBoxChance(npcID int32) (int32, bool) {
	switch npcID {
	case 18147:
		return 730000, true
	case 18149:
		return 750000, true
	case 18145:
		return 760000, true
	case 18146:
		return 780000, true
	case 18148:
		return 850000, true
	case 18141, 18142, 18143, 18144:
		return 900000, true
	case 18137, 18169, 18172, 18191, 18226:
		return 1060000, true
	case 18134, 18167, 18170, 18188, 18223:
		return 1070000, true
	case 18166, 18195:
		return 1080000, true
	case 18122, 18125, 18128, 18131, 18168, 18175, 18178, 18181, 18184:
		return 1100000, true
	case 18171:
		return 1110000, true
	case 18173, 18176, 18179, 18182:
		return 1170000, true
	case 18228:
		return 1360000, true
	case 18139, 18193:
		return 1390000, true
	case 18138, 18140, 18192, 18229:
		return 1410000, true
	case 18133, 18135, 18136, 18187, 18189, 18190, 18194:
		return 1420000, true
	case 18222, 18225:
		return 1430000, true
	case 18121, 18124, 18127, 18130, 18224:
		return 1440000, true
	case 18174, 18177, 18180, 18183:
		return 1450000, true
	case 18185:
		return 1460000, true
	case 18186, 18220:
		return 1470000, true
	case 18120, 18123, 18126, 18129, 18221:
		return 1510000, true
	case 18132:
		return 1540000, true
	case 18230:
		return 1580000, true
	case 18227:
		return 1820000, true
	case 18212, 18213, 18214, 18215, 18216, 18217, 18218, 18219:
		return 4500000, true
	}
	return 0, false
}

// halishaGoblet returns the goblet the raid boss npcID drops.
func halishaGoblet(npcID int32) (int32, bool) {
	switch npcID {
	case halishaAlectia:
		return gobletOfAlectia, true
	case halishaTishas:
		return gobletOfTishas, true
	case halishaMekara:
		return gobletOfMekara, true
	case halishaMorigul:
		return gobletOfMorigul, true
	}
	return 0, false
}

// isRecipeReward reports whether id is a recipe the relic parts buy.
func isRecipeReward(id int32) bool {
	return slices.Contains([]int32{6881, 6883, 6885, 6887, 6891, 6893, 6895, 6897, 6899, 7580}, id)
}

// ids returns lo to hi, both included.
func ids(lo, hi int32) []int32 {
	out := make([]int32, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		out = append(out, id)
	}
	return out
}

// New returns the quest.
func New() script.Script {
	droppers := slices.Concat(ids(firstDropperA, lastDropperA), ids(firstDropperB, lastDropperB), ids(firstDropperC, lastDropperC))
	managers := ids(firstManager, lastManager)
	gatekeepers := ids(firstGatekeeper, lastGatekeeper)
	return script.Script{
		Title:   "Four Goblets",
		QuestID: 620,
		Items:   []int32{sealedBox, chapelKey, usedGravePass},
		Bind: script.Bindings{
			script.EventQuestStart: {namelessSpirit},
			script.EventTalked: slices.Concat(
				[]int32{namelessSpirit, ghostOfWigoth1, ghostOfWigoth2, ghostChamberlain}, managers, gatekeepers,
			),
			script.EventCreated: slices.Concat(gatekeepers, managers, []int32{ghostOfWigoth1}),
			script.EventMyDying: slices.Concat(droppers, []int32{halishaAlectia, halishaTishas, halishaMekara, halishaMorigul}),
		},
		Hooks: script.Hooks{
			OnCreated: onCreated, OnEvent: onAdvEvent, OnMyDying: onMyDying, OnTalk: onTalk, OnTimer: onTimer,
		},
	}
}

func isGatekeeper(npcID int32) bool { return npcID >= firstGatekeeper && npcID <= lastGatekeeper }

func isManager(npcID int32) bool { return npcID >= firstManager && npcID <= lastManager }

// closeHallDoor closes the door of the gatekeeper npcID.
func closeHallDoor(s *script.Script, npcID int32) {
	if d := s.DoorByID(hallDoorID(npcID)); d != nil {
		d.Close()
	}
}

func hasAllGoblets(p *script.Player) bool {
	return p.HasItems(gobletOfAlectia, gobletOfTishas, gobletOfMekara, gobletOfMorigul)
}

// onCreated closes a gatekeeper's door and starts the watches of the
// gatekeepers and of the ghost of Wigoth.
func onCreated(s *script.Script, e script.Created) {
	npcID := e.NPC.NpcID()
	if isGatekeeper(npcID) {
		e.NPC.SetInt(script.SlotIntAI0, 0)
		closeHallDoor(s, npcID)
		s.StartTimerAtFixedRate(timerGateWatch, e.NPC, nil, 1000*time.Millisecond, 1000*time.Millisecond)
	}
	if npcID == ghostOfWigoth1 {
		s.StartTimerAtFixedRate(timerWigothWatch, e.NPC, nil, 1000*time.Millisecond, 1000*time.Millisecond)
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	npcID := e.NPC.NpcID()

	switch {
	// Nameless Spirit
	case strings.EqualFold(e.Name, "31453-13.htm"):
		st.SetStatus(questlog.StatusStarted)
		if p.HasItem(antiqueBrooch) {
			st.SetCond(2)
			htmltext = "31453-19.htm"
		} else {
			st.SetCond(1)
		}
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31453-16.htm"):
		if !hasAllGoblets(p) {
			return ""
		}
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gobletOfAlectia, -1)
		s.TakeItems(p, gobletOfTishas, -1)
		s.TakeItems(p, gobletOfMekara, -1)
		s.TakeItems(p, gobletOfMorigul, -1)
		s.GiveItems(p, antiqueBrooch, 1)
	case strings.EqualFold(e.Name, "31453-18.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	// Ghost of Wigoth 1
	case strings.EqualFold(e.Name, "31452-06.htm"):
		s.TakeItems(p, chapelKey, -1)
		p.TeleportTo(script.Loc{X: 169592, Y: -91006, Z: -2912}, 0)
	// Ghost of Wigoth 2
	case strings.EqualFold(e.Name, "31454-14.htm"):
		if !p.HasItem(sealedBox) {
			return ""
		}
		s.TakeItems(p, sealedBox, 1)
		if openSealedBox(s, p) {
			htmltext = "31454-13.htm"
		}
	case script.IsDigit(e.Name):
		// A simple digit is a recipe asked for.
		id, err := strconv.Atoi(e.Name)
		if err != nil || id < math.MinInt32 || id > math.MaxInt32 || !isRecipeReward(int32(id)) || p.ItemCount(brokenRelicPart) < brokenRelicsNeeded {
			return ""
		}
		s.TakeItems(p, brokenRelicPart, brokenRelicsNeeded)
		s.GiveItems(p, int32(id), 1)
		htmltext = "31454-17.htm"
	// Ghost Chamberlain of Elmoreden
	case strings.EqualFold(e.Name, "31919-06.htm"):
		if p.HasItem(sealedBox) {
			s.TakeItems(p, sealedBox, 1)

			// The chamberlain succeeds half as often as the ghost of Wigoth.
			switch {
			case s.RndBool():
				htmltext = "31919-05.htm"
			case openSealedBox(s, p):
				htmltext = "31919-03.htm"
			default:
				htmltext = "31919-04.htm"
			}
		}
	// Sepulcher Managers
	case isManager(npcID) && strings.EqualFold(e.Name, "Enter"):
		return enter(s, e.NPC, p)
	}
	return htmltext
}

// enter takes the party of p into the sepulcher of the keeper npc, and
// answers with the page for the entry or for what refuses it.
func enter(s *script.Script, keeper *script.NPC, p *script.Player) string {
	npcID := keeper.NpcID()
	page := func(suffix string) string { return strconv.Itoa(int(npcID)) + suffix }

	// The sepulchers are open to entry only the last minutes of the hour.
	if minute := s.Now().Minute(); minute >= 0 && minute < 55 {
		return page("-02.htm")
	}

	members := p.PartyMembers()
	if members == nil || len(members) < s.EventRules().FourGobletsParty {
		return page("-04.htm")
	}
	if !p.IsPartyLeader() {
		return page("-03.htm")
	}

	// Every member needs an entrance pass and the quest.
	for _, member := range members {
		if !member.HasItem(entrancePass) {
			return strings.ReplaceAll(s.HTMLText(page("-05.htm")), "%member%", member.Name())
		}
		mst := s.QuestState(member, questName)
		if mst == nil || mst.Status() != questlog.StatusStarted {
			return strings.ReplaceAll(s.HTMLText(page("-06.htm")), "%member%", member.Name())
		}
	}

	// A sepulcher takes one party at a time.
	if keeper.CompareAndExchangeInt(script.SlotAVQuest0, 0, 1) == 1 {
		return page("-07.htm")
	}

	at := sepulcherLoc(npcID)
	for _, member := range members {
		if !script.InRange(1000, keeper, member, true) {
			continue
		}
		mst := s.QuestState(member, questName)
		if mst == nil {
			continue
		}
		mst.Set("completed", strconv.FormatBool(false))

		s.TakeItems(member, entrancePass, 1)
		if !member.HasItem(antiqueBrooch) {
			s.GiveItems(member, usedGravePass, 1)
		}
		s.TakeItems(member, chapelKey, -1)

		if z := s.BossZoneAt(at); z != nil {
			z.AllowPlayerEntry(member, 30)
		}
		s.InstantTeleportNear(member, at, 80)
	}
	return page("-08.htm")
}

// onMyDying has a dying room monster drop a sealed box to one questing member
// of its killer's party, and a dying raid boss pay its goblet to every one
// and call the ghost of Wigoth.
func onMyDying(s *script.Script, e script.MyDying) {
	npcID := e.NPC.NpcID()
	p := script.ActingPlayer(e.Killer)

	if chance, ok := sealedBoxChance(npcID); ok {
		if st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted); st != nil {
			s.DropItems(st.Player(), sealedBox, 1, 0, chance)
		}
	}

	if goblet, ok := halishaGoblet(npcID); ok {
		for _, mst := range s.PartyMembersState(p, e.NPC, questlog.StatusStarted) {
			mst.Set("completed", strconv.FormatBool(true))

			member := mst.Player()
			if mst.Cond() == 2 || member.HasItem(goblet) {
				continue
			}
			s.GiveItems(member, goblet, 1)
			s.PlaySound(member, script.SoundItemGet)
		}

		// The ghost of Wigoth stays until the sepulchers close.
		if wigoth := s.Spawner().AddSpawn(ghostOfWigoth1, e.NPC, false, 0); wigoth != nil {
			s.Spawner().ScheduleDespawn(wigoth, time.Duration(max(54-s.Now().Minute(), 1)*60*1000)*time.Millisecond)
		}
	}
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}

	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() >= 74 {
			htmltext = "31453-01.htm"
		} else {
			htmltext = "31453-12.htm"
		}
	case questlog.StatusStarted:
		npcID := e.NPC.NpcID()
		cond := st.Cond()
		switch {
		case npcID == namelessSpirit:
			if cond == 1 {
				if hasAllGoblets(p) {
					htmltext = "31453-15.htm"
				} else {
					htmltext = "31453-14.htm"
				}
			} else if cond == 2 {
				htmltext = "31453-17.htm"
			}
		case npcID == ghostOfWigoth1:
			if cond == 1 {
				switch {
				case hasAllGoblets(p):
					htmltext = "31452-04.htm"
				case gobletsHeld(p) == 3:
					htmltext = "31452-02.htm"
				default:
					htmltext = "31452-01.htm"
				}
			}
			if cond == 2 {
				htmltext = "31452-05.htm"
			}
		case npcID == ghostOfWigoth2:
			if st.GetBool("completed") {
				// The base option follows the brooch and the goblets, the rest
				// the sealed boxes and the relic parts.
				index := 5
				if cond == 2 {
					index = 9
				} else if hasAllGoblets(p) {
					index = 1
				}
				if p.HasItem(sealedBox) {
					index++
				}
				if p.ItemCount(brokenRelicPart) >= brokenRelicsNeeded {
					index += 2
				}
				htmltext = fmt.Sprintf("31454-%02d.htm", index)
			}
		case npcID == ghostChamberlain:
			htmltext = "31919-01.htm"
		case isManager(npcID):
			htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
		case isGatekeeper(npcID):
			htmltext = openHall(s, e.NPC, p)
		}
	}
	return htmltext
}

// gobletsHeld counts the kinds of goblet p holds.
func gobletsHeld(p *script.Player) int32 {
	return min(1, p.ItemCount(gobletOfAlectia)) + min(1, p.ItemCount(gobletOfMekara)) +
		min(1, p.ItemCount(gobletOfMorigul)) + min(1, p.ItemCount(gobletOfTishas))
}

// openHall opens the door of the gatekeeper npc against a chapel key, for 15
// seconds, and calls what waits behind it: the privates of a room with their
// spots, else the master's room. It answers the page of a player with no
// key, none otherwise.
func openHall(s *script.Script, gate *script.NPC, p *script.Player) string {
	npcID := gate.NpcID()
	if !p.HasItem(chapelKey) {
		return strconv.Itoa(int(npcID)) + "-01.htm"
	}
	s.TakeItems(p, chapelKey, 1)

	if d := s.DoorByID(hallDoorID(npcID)); d != nil {
		d.Open()
	}
	s.StartTimer(timerGateClose, gate, nil, 15000*time.Millisecond)

	spot := func(x, y, z string) script.Loc {
		return script.Loc{X: int(gate.IntParam(x)), Y: int(gate.IntParam(y)), Z: int(gate.IntParam(z)), Heading: 32768}
	}
	spot1 := spot("SpawnX1", "SpawnY1", "SpawnZ1")
	spot2 := spot("SpawnX2", "SpawnY2", "SpawnZ2")
	spot3 := spot("SpawnX3", "SpawnY3", "SpawnZ3")
	master := gate.Master()

	if spot1.X != 0 {
		if master != nil {
			s.SendScriptEvent(master, 1000+gate.IntParam("RoomIDX"), 0, 0)
		}

		var order [3]script.Loc
		switch s.Rnd(3) {
		case 0:
			// The third one stands at the second spot's y.
			third := script.Loc{X: spot3.X, Y: spot2.Y, Z: spot3.Z, Heading: spot3.Heading}
			order = [3]script.Loc{spot1, spot2, third}
		case 1:
			order = [3]script.Loc{spot2, spot3, spot1}
		default:
			order = [3]script.Loc{spot3, spot1, spot2}
		}
		for i, private := range []int32{privateStrong, privateMiddle, privateWeak} {
			s.Spawner().CreateOnePrivateEx(gate, private, order[i], 0, script.Params{P1: int32(i + 1)})
		}
		gate.Shout(script.NpcString(sayGatekeeperCalling))
	} else if master != nil {
		s.SendScriptEvent(master, 1000+gate.Int(script.SlotParam1), 0, 0)
	}
	return ""
}

func onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, timerWigothWatch) {
		if s.Now().Minute() >= 54 {
			e.NPC.DeleteMe()
		}
	}
	if strings.EqualFold(e.Name, timerGateWatch) {
		now := s.Now()
		if now.Minute() == 55 && now.Second() == 0 {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			closeHallDoor(s, e.NPC.NpcID())
		}
	} else if strings.EqualFold(e.Name, timerGateClose) {
		closeHallDoor(s, e.NPC.NpcID())
		e.NPC.SetInt(script.SlotIntAI0, 0)
	}
	return ""
}

// boxReward is an item and its count, given for a pick below the bound.
type boxReward struct{ below, item, count int32 }

// boxRoll is a chance in a thousand of a pick among rewards.
type boxRoll struct {
	chance  int
	rewards []boxReward
}

// boxRolls returns the rolls of a group of prizes.
func boxRolls(group int) []boxRoll {
	switch group {
	case 1:
		return []boxRoll{
			{chance: 848, rewards: []boxReward{
				{43, 1884, 42},
				{66, 1895, 36},
				{184, 1876, 4},
				{250, 1881, 6},
				{287, 5549, 8},
				{484, 1874, 1},
				{681, 1889, 1},
				{799, 1877, 1},
				{902, 1894, 1},
				{1000, 4043, 1},
			}},
			{chance: 323, rewards: []boxReward{
				{335, 1888, 1},
				{556, 4040, 1},
				{725, 1890, 1},
				{872, 5550, 1},
				{962, 1893, 1},
				{986, 4046, 1},
				{1000, 4048, 1},
			}},
		}
	case 2:
		return []boxRoll{
			{chance: 847, rewards: []boxReward{
				{148, 1878, 8},
				{175, 1882, 24},
				{273, 1879, 4},
				{322, 1880, 6},
				{357, 1885, 6},
				{554, 1875, 1},
				{685, 1883, 1},
				{803, 5220, 1},
				{901, 4039, 1},
				{1000, 4044, 1},
			}},
			{chance: 251, rewards: []boxReward{
				{350, 1887, 1},
				{587, 4042, 1},
				{798, 1886, 1},
				{922, 4041, 1},
				{966, 1892, 1},
				{996, 1891, 1},
				{1000, 4047, 1},
			}},
		}
	case 3:
		return []boxRoll{
			{chance: 31, rewards: []boxReward{
				{223, 730, 1},
				{893, 948, 1},
				{1000, 960, 1},
			}},
			{chance: 5, rewards: []boxReward{
				{202, 729, 1},
				{928, 947, 1},
				{1000, 959, 1},
			}},
		}
	case 4:
		return []boxRoll{
			{chance: 329, rewards: []boxReward{
				{88, 6698, 1},
				{185, 6699, 1},
				{238, 6700, 1},
				{262, 6701, 1},
				{292, 6702, 1},
				{356, 6703, 1},
				{420, 6704, 1},
				{482, 6705, 1},
				{554, 6706, 1},
				{576, 6707, 1},
				{640, 6708, 1},
				{704, 6709, 1},
				{777, 6710, 1},
				{799, 6711, 1},
				{863, 6712, 1},
				{927, 6713, 1},
				{1000, 6714, 1},
			}},
			{chance: 54, rewards: []boxReward{
				{100, 6688, 1},
				{198, 6689, 1},
				{298, 6690, 1},
				{398, 6691, 1},
				{499, 7579, 1},
				{601, 6693, 1},
				{703, 6694, 1},
				{801, 6695, 1},
				{902, 6696, 1},
				{1000, 6697, 1},
			}},
		}
	}
	return nil
}

// openSealedBox gives p the prizes of a sealed box, which hold nothing at
// all some of the time; false when it held nothing.
func openSealedBox(s *script.Script, p *script.Player) bool {
	result := false
	group := s.Rnd(5)
	if group == 0 {
		result = true
		s.GiveItems(p, adena, 10000)
	}

	for _, roll := range boxRolls(group) {
		if s.Rnd(1000) >= roll.chance {
			continue
		}
		result = true
		pick := int32(s.Rnd(1000))
		for _, r := range roll.rewards {
			if pick < r.below {
				s.GiveItems(p, r.item, r.count)
				break
			}
		}
	}
	return result
}
