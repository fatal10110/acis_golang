// Package q635 is the quest In the Dimensional Rift. A Dimensional Rift
// Keeper takes a character into the waiting hall of the rift; there a party
// pays dimension fragments to a rift post watcher, which sends it into one
// of the eight rooms of the level its post stands on. The border guardians
// of a room watch it: they send the party on to another room every few
// minutes, now and then to the ninth room of Anakazel, and put it out of the
// rift when the party is broken, its leader is gone or its time is up.
package q635

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

const (
	questName = "Q635_InTheDimensionalRift"
	questID   = 635
)

// NPCs: the rift post watchers, the keepers that send characters in, and
// the guardians of the border of each room.
const (
	firstPost     = 31488
	lastPost      = 31493
	firstKeeper   = 31494
	lastKeeper    = 31507
	firstGuardian = 31865
	lastGuardian  = 31918
)

// Items.
const dimensionalFragment = 7079

// Timers: the watch of a room, and the jump to another one.
const (
	timerRoomWatch = "2001"
	timerRoomJump  = "2002"
)

// Script events the guardians are sent: the fight in the room of Anakazel is
// on, and its party is kept there, or it is over, and the party may go on.
const (
	eventKeepParty    = 2
	eventReleaseParty = 3
)

// The rooms of a level: eight, and the ninth of Anakazel.
const (
	regularRooms  = 8
	roomsPerLevel = 9
	anakazelRoom  = 8
)

// teleInLoc is where a character the keeper accepts goes.
func teleInLoc() script.Loc { return script.Loc{X: -114796, Y: -179334, Z: -6752} }

// oracleLoc returns one of the three places in front of the oracle of the
// cabal, picked by pick.
func oracleLoc(cabal sevensigns.Cabal, pick int) script.Loc {
	if cabal == sevensigns.Dusk {
		return []script.Loc{{X: -81328, Y: 86536, Z: -5152}, {X: -81262, Y: 86468, Z: -5152}, {X: -81248, Y: 86582, Z: -5152}}[pick]
	}
	return []script.Loc{{X: -80316, Y: 111356, Z: -4896}, {X: -80226, Y: 111290, Z: -4896}, {X: -80217, Y: 111435, Z: -4896}}[pick]
}

// quest holds the rooms of each level, by the name of the level its rift
// posts name, from the moment a post is created.
type quest struct {
	rooms *script.MapCell[string, []*script.RoomCell]
}

// New returns the quest.
func New() script.Script {
	q := quest{rooms: new(script.MapCell[string, []*script.RoomCell])}
	posts := ids(firstPost, lastPost)
	keepers := ids(firstKeeper, lastKeeper)
	guardians := ids(firstGuardian, lastGuardian)
	return script.Script{
		Title:   "In the Dimensional Rift",
		QuestID: questID,
		Bind: script.Bindings{
			script.EventQuestStart:  keepers,
			script.EventCreated:     slices.Concat(posts, guardians),
			script.EventFirstTalk:   slices.Concat(posts, guardians),
			script.EventScriptEvent: guardians,
			script.EventTalked:      slices.Concat(posts, keepers, guardians),
		},
		Hooks: script.Hooks{
			OnCreated: q.onCreated, OnEvent: q.onAdvEvent, OnFirstTalk: onFirstTalk,
			OnScriptEvent: onScriptEvent, OnTalk: onTalk, OnTimer: q.onTimer,
		},
	}
}

// ids returns lo to hi, both included.
func ids(lo, hi int32) []int32 {
	out := make([]int32, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		out = append(out, id)
	}
	return out
}

func isPost(npcID int32) bool { return npcID >= firstPost && npcID <= lastPost }

func isKeeper(npcID int32) bool { return npcID >= firstKeeper && npcID <= lastKeeper }

func isGuardian(npcID int32) bool { return npcID >= firstGuardian && npcID <= lastGuardian }

// levelRooms returns the rooms of the level the NPC's spawn names, none
// when it names no level or none has rooms yet.
func (q quest) levelRooms(npc *script.NPC) []*script.RoomCell {
	level, ok := npc.StringParam("LevelName")
	if !ok {
		return nil
	}
	rooms, _ := q.rooms.Get(level)
	return rooms
}

// onCreated gives the rooms of a level when its post appears, and starts the
// watch of a guardian after it put out whoever stands in its territory.
func (q quest) onCreated(s *script.Script, e script.Created) {
	npc := e.NPC
	if isPost(npc.NpcID()) {
		if level, ok := npc.StringParam("LevelName"); ok {
			rooms := make([]*script.RoomCell, roomsPerLevel)
			for i := range rooms {
				rooms[i] = s.NewRoom(i)
			}
			q.rooms.Set(level, rooms)
		}
		return
	}
	npc.SetInt(script.SlotIntAI1, 3)
	npc.SetInt(script.SlotIntAI2, 0)
	oustPlayers(s, npc)
	s.StartTimerAtFixedRate(timerRoomWatch, npc, nil, 1000*time.Millisecond, 10000*time.Millisecond)
}

func onFirstTalk(_ *script.Script, e script.FirstTalk) string {
	npcID := e.NPC.NpcID()
	if isPost(npcID) {
		return "rift_watcher_1001.htm"
	}
	if isGuardian(npcID) {
		room, ok := e.NPC.StringParam("RoomIndex")
		if !ok {
			room = "null"
		}
		return "tel_dungeon_npc_hi" + room + ".htm"
	}
	return ""
}

func onTalk(_ *script.Script, e script.Talk) string {
	if !isKeeper(e.NPC.NpcID()) {
		return ""
	}
	p := e.Player
	switch {
	case p.Level() < 20:
		return "dimension_keeper_01.htm"
	case p.StartedQuests() >= 25:
		return "dimension_keeper_02.htm"
	case !p.HasItem(dimensionalFragment):
		return "dimension_keeper_03.htm"
	}
	return "dimension_keeper_04.htm"
}

func onScriptEvent(_ *script.Script, e script.ScriptEvent) {
	if e.EventID == eventKeepParty || e.EventID == eventReleaseParty {
		e.NPC.SetInt(script.SlotIntAI1, e.EventID)
	}
}

// oustPlayers sends everyone in the territory of the NPC's spawn out of the
// rift, to the place its spawn names.
func oustPlayers(s *script.Script, npc *script.NPC) {
	at := script.Loc{X: int(npc.IntParam("escape_x")), Y: int(npc.IntParam("escape_y")), Z: int(npc.IntParam("escape_z"))}
	s.TeleportPlayersInTerritory(npc, at, 100)
}

// makerEvent sends the maker of the NPC's spawn the script event name.
func makerEvent(s *script.Script, npc *script.NPC, name string) {
	s.Spawner().SendMakerEvent(npc.MakerName(), name, 0, 0)
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	switch {
	case strings.EqualFold(e.Name, "dimension_keeper_07.htm"):
		return enterRift(s, e)
	case strings.EqualFold(e.Name, "rift_watcher_05.htm"):
		return q.enterRoom(s, e)
	case strings.EqualFold(e.Name, "rift_watcher_06.htm"):
		leaveRift(s, e.Player)
	case strings.EqualFold(e.Name, "rift_watcher_10.htm"):
		return goToOracle(s, e.Player)
	case strings.EqualFold(e.Name, "GiveUpRift"):
		return q.giveUp(s, e)
	case strings.EqualFold(e.Name, "ChangeRiftRoom"):
		return q.changeRoom(s, e)
	}
	return e.Name
}

// enterRift starts the quest for a character that has none and takes it to
// the waiting hall, from the keeper that remembers where it came from.
func enterRift(s *script.Script, e script.Event) string {
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return e.Name
	}
	page := e.Name
	if st.Status() == questlog.StatusStarted {
		page = "dimension_keeper_07a.htm"
	} else {
		e.Player.SetMemo("rift", strconv.Itoa(int(e.NPC.NpcID()-lastPost)*10000))
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	}
	e.Player.TeleportTo(teleInLoc(), 0)
	return page
}

// fillName puts name in the page file.
func fillName(s *script.Script, file, name string) string {
	return strings.ReplaceAll(s.HTMLText(file), "<?name?>", name)
}

// enterRoom takes the party of a watcher's visitor into a free room of the
// level, when it is big enough, led by the visitor, every member has the
// quest and pays the fragments.
func (q quest) enterRoom(s *script.Script, e script.Event) string {
	p, npc := e.Player, e.NPC
	rift := s.EventRules().Rift
	members := p.PartyMembers()
	if members == nil || len(members) < rift.MinParty {
		return "rift_watcher_01.htm"
	}
	leader := p.PartyLeader()
	if leader == nil || leader.ObjectID() != p.ObjectID() {
		name := ""
		if leader != nil {
			name = leader.Name()
		}
		return fillName(s, "rift_watcher_02.htm", name)
	}

	postID := npc.NpcID()
	if !isPost(postID) {
		return ""
	}
	cost := int32(rift.Costs[postID-firstPost])
	tick := s.CurrentTick()
	for _, member := range members {
		mst := s.QuestState(member, questName)
		if mst == nil || mst.Status() != questlog.StatusStarted {
			return fillName(s, "rift_watcher_03a.htm", member.Name())
		}
		if member.ItemCount(dimensionalFragment) < cost {
			// A member put out by the rift (memo state 2) within the hour
			// goes in again without the fragments.
			since := s.MemoStateEx(member, questID, 1)
			if s.MemoState(member, questID) != 2 || tick-since > 3600 {
				return fillName(s, "rift_watcher_03.htm", member.Name())
			}
		}
	}

	if _, ok := npc.StringParam("LevelName"); !ok {
		return ""
	}
	room := -1
	if rooms := q.levelRooms(npc); rooms != nil {
		room = takeRoom(s, rooms, p)
	}
	switch room {
	case -1:
		return "rift_watcher_04.htm"
	case -2:
		return "rift_watcher_08.htm"
	}

	at := script.Loc{
		X: int(npc.IntParamOr("PosX"+strconv.Itoa(room), 0)),
		Y: int(npc.IntParamOr("PosY"+strconv.Itoa(room), 0)),
		Z: script.LocOf(npc).Z,
	}
	now := s.CurrentTick()
	for _, member := range members {
		if npc.Distance2D(member) > 1500 {
			continue
		}
		s.DestroyItems(member, dimensionalFragment, cost, true)
		s.SetMemoState(member, questID, now)
		leads := int32(0)
		if member.ObjectID() == leader.ObjectID() {
			leads = 1
		}
		s.SetMemoStateEx(member, questID, 1, leads)
		member.TeleportTo(at, 0)
	}
	return e.Name
}

// takeRoom gives the party of p one of the eight rooms and returns its
// index: -2 when a room already holds that party, -1 when fewer than three
// rooms are free or none could be taken.
func takeRoom(s *script.Script, rooms []*script.RoomCell, p *script.Player) int {
	free := 0
	for _, room := range rooms[:regularRooms] {
		if !room.Locked() {
			free++
		} else if room.HoldsPartyOf(p) {
			return -2
		}
	}
	if free < 3 {
		return -1
	}

	for range regularRooms {
		i := s.Rnd(regularRooms)
		if rooms[i].TryLock() {
			rooms[i].SetParty(p)
			return i
		}
	}
	for i, room := range rooms[:regularRooms] {
		if room.TryLock() {
			room.SetParty(p)
			return i
		}
	}
	return -1
}

// riftExit is where a character of a cabal goes to from the rift post of a
// town, by the number its memo holds.
type riftExit struct {
	from, to   int
	byMemo     bool
	dawn, dusk script.Loc
}

func riftExits() []riftExit {
	return []riftExit{
		{95, 195, false, script.Loc{X: -80542, Y: 150315, Z: -3040}, script.Loc{X: -82340, Y: 151575, Z: -3120}},
		{195, 295, false, script.Loc{X: -13996, Y: 121413, Z: -2984}, script.Loc{X: -14727, Y: 124002, Z: -3112}},
		{295, 395, false, script.Loc{X: 16320, Y: 142915, Z: -2696}, script.Loc{X: 18501, Y: 144673, Z: -3056}},
		{395, 495, false, script.Loc{X: 83312, Y: 149236, Z: -3400}, script.Loc{X: 81572, Y: 148580, Z: -3464}},
		{495, 595, false, script.Loc{X: 111359, Y: 220959, Z: -3544}, script.Loc{X: 112441, Y: 220149, Z: -3544}},
		{595, 695, false, script.Loc{X: 83057, Y: 53983, Z: -1488}, script.Loc{X: 82842, Y: 54613, Z: -1520}},
		{695, 795, false, script.Loc{X: 146955, Y: 26690, Z: -2200}, script.Loc{X: 147528, Y: 28899, Z: -2264}},
		{795, 895, false, script.Loc{X: 115206, Y: 74775, Z: -2600}, script.Loc{X: 116651, Y: 77512, Z: -2688}},
		{995, 1095, false, script.Loc{X: 148326, Y: -55533, Z: -2776}, script.Loc{X: 149968, Y: -56645, Z: -2976}},
		{1095, 1195, true, script.Loc{X: 45605, Y: -50360, Z: -792}, script.Loc{X: 44505, Y: -48331, Z: -792}},
		{1195, 1295, true, script.Loc{X: 86730, Y: -143148, Z: -1336}, script.Loc{X: 85048, Y: -142046, Z: -1536}},
	}
}

// dimensionExit returns where the character that went in through the
// keeper numbered i2 comes out.
func dimensionExit(i2 int) (script.Loc, bool) {
	switch i2 {
	case 1:
		return script.Loc{X: -41443, Y: 210030, Z: -5080}, true
	case 2:
		return script.Loc{X: -53034, Y: -250421, Z: -7935}, true
	case 3:
		return script.Loc{X: 45160, Y: 123605, Z: -5408}, true
	case 4:
		return script.Loc{X: 46488, Y: 170184, Z: -4976}, true
	case 5:
		return script.Loc{X: 111521, Y: 173905, Z: -5432}, true
	case 6:
		return script.Loc{X: -20395, Y: -250930, Z: -8191}, true
	case 7:
		return script.Loc{X: -21482, Y: 77253, Z: -5168}, true
	case 8:
		return script.Loc{X: 140688, Y: 79565, Z: -5424}, true
	case 9:
		return script.Loc{X: -52007, Y: 78986, Z: -4736}, true
	case 10:
		return script.Loc{X: 118547, Y: 132669, Z: -4824}, true
	case 11:
		return script.Loc{X: 172562, Y: -17730, Z: -4896}, true
	case 12:
		return script.Loc{X: 83344, Y: 209110, Z: -5432}, true
	case 13:
		return script.Loc{X: -19154, Y: 13415, Z: -4896}, true
	case 14:
		return script.Loc{X: 12747, Y: -248614, Z: -9607}, true
	case 21:
		return script.Loc{X: -41559, Y: 209140, Z: -5080}, true
	case 22:
		return script.Loc{X: 42448, Y: 143943, Z: -5376}, true
	case 23:
		return script.Loc{X: 45239, Y: 124522, Z: -5408}, true
	case 24:
		return script.Loc{X: 45680, Y: 170299, Z: -4976}, true
	case 25:
		return script.Loc{X: 110659, Y: 174008, Z: -5432}, true
	case 26:
		return script.Loc{X: 77132, Y: 78399, Z: -5120}, true
	case 27:
		return script.Loc{X: -22408, Y: 77375, Z: -5168}, true
	case 28:
		return script.Loc{X: 139807, Y: 79675, Z: -5424}, true
	case 29:
		return script.Loc{X: -53177, Y: 79100, Z: -4736}, true
	case 30:
		return script.Loc{X: 117647, Y: 132801, Z: -4824}, true
	case 31:
		return script.Loc{X: 171684, Y: -17602, Z: -4896}, true
	case 32:
		return script.Loc{X: 82456, Y: 209218, Z: -5432}, true
	case 33:
		return script.Loc{X: -20105, Y: 13505, Z: -4896}, true
	case 34:
		return script.Loc{X: 113299, Y: 84547, Z: -6536}, true
	}
	return script.Loc{}, false
}

// leaveRift takes the character out of the rift to where its memo of the
// way in says, by its cabal for a town.
func leaveRift(s *script.Script, p *script.Player) {
	i0 := 0
	if v, ok := p.Memo("rift"); ok {
		i0, _ = strconv.Atoi(v)
	}
	i1 := i0 % 10000
	i2 := ((i0 - i1) + 5) / 10000
	cabal := s.PlayerCabal(p.ObjectID())

	for _, exit := range riftExits() {
		at := i1
		if exit.byMemo {
			at = i0
		}
		if i1 < exit.from || at >= exit.to {
			continue
		}
		switch cabal {
		case sevensigns.Dawn:
			p.TeleportTo(exit.dawn, 0)
		case sevensigns.Dusk:
			p.TeleportTo(exit.dusk, 0)
		}
		return
	}
	if at, ok := dimensionExit(i2); ok {
		p.TeleportTo(at, 0)
	}
}

// goToOracle takes a character of a cabal to its oracle while the
// competition is on.
func goToOracle(s *script.Script, p *script.Player) string {
	if s.SevenSignsPeriod() != sevensigns.Competition {
		return "rift_watcher_12.htm"
	}
	cabal := s.PlayerCabal(p.ObjectID())
	if cabal != sevensigns.Dusk && cabal != sevensigns.Dawn {
		return "rift_watcher_11.htm"
	}
	p.TeleportTo(oracleLoc(cabal, s.Rnd(3)), 0)
	return "rift_watcher_10.htm"
}

// roomOf returns the room of the guardian's spawn and its rooms.
func (q quest) roomOf(npc *script.NPC) (*script.RoomCell, []*script.RoomCell) {
	rooms := q.levelRooms(npc)
	index := int(npc.IntParam("RoomIndex"))
	if index < 0 || index >= len(rooms) {
		return nil, nil
	}
	return rooms[index], rooms
}

// eachMember runs fn for the members of the room as it took them that are
// in the world.
func eachMember(s *script.Script, room *script.RoomCell, fn func(p *script.Player)) {
	for _, id := range room.MemberIDs() {
		if p := s.WorldPlayer(id); p != nil {
			fn(p)
		}
	}
}

// giveUp puts the party of a room out of the rift on its leader's word.
func (q quest) giveUp(s *script.Script, e script.Event) string {
	room, _ := q.roomOf(e.NPC)
	if room == nil {
		return ""
	}
	leader := room.PartyLeader()
	if leader == nil || leader.ObjectID() != e.Player.ObjectID() {
		return "tel_dungeon_npc_notleader.htm"
	}

	oustPlayers(s, e.NPC)
	makerEvent(s, e.NPC, "0")
	eachMember(s, room, func(p *script.Player) { s.SetMemoStateEx(p, questID, 1, -1) })
	room.Clear()
	return ""
}

// changeRoom sends the party of a room to another free room, once, on its
// leader's word.
func (q quest) changeRoom(s *script.Script, e script.Event) string {
	p, npc := e.Player, e.NPC
	if p.PartyMembers() == nil {
		return ""
	}
	room0, rooms := q.roomOf(npc)
	if room0 == nil {
		return ""
	}
	// The word of a leader of some other party moves no one.
	if !p.IsPartyLeader() || !room0.HoldsPartyOf(p) {
		return "tel_dungeon_npc_notleader.htm"
	}
	if s.MemoStateEx(p, questID, 1) != 1 {
		return "tel_dungeon_npc_nomorechance.htm"
	}
	s.SetMemoStateEx(p, questID, 1, 0)

	var free []*script.RoomCell
	for _, room := range rooms {
		if !room.Locked() && room.Index() != anakazelRoom {
			free = append(free, room)
		}
	}
	var room1 *script.RoomCell
	for room1 == nil && len(free) > 0 {
		i := s.Rnd(len(free))
		if free[i].TryLock() {
			room1 = free[i]
		}
		free = append(free[:i], free[i+1:]...)
	}
	if room1 == nil {
		npc.SetInt(script.SlotIntAI4, -1)
		return ""
	}

	room1.SetPartyFrom(room0)
	makerEvent(s, npc, "0")
	moveParty(npc, room0, room1.Index())
	room0.Clear()
	return ""
}

// moveParty sends the members of the room that stand within 2000 of the
// NPC to the room numbered index of its level.
func moveParty(npc *script.NPC, from *script.RoomCell, index int) {
	at := script.Loc{
		X: int(npc.IntParamOr("PosX"+strconv.Itoa(index), 0)),
		Y: int(npc.IntParamOr("PosY"+strconv.Itoa(index), 0)),
		Z: script.LocOf(npc).Z,
	}
	for _, member := range from.PartyMembers() {
		if npc.Distance2D(member) <= 2000 {
			member.TeleportTo(at, 0)
		}
	}
}

// outside reports whether the leader is gone or out of the territory of the
// NPC's spawn.
func outside(npc *script.NPC, leader *script.Player) bool {
	return leader == nil || !npc.InSpawnTerritory(leader)
}

// quake shakes the screen of the members of the room.
func quake(room *script.RoomCell, intensity int32) {
	for _, member := range room.PartyMembers() {
		member.Earthquake(intensity, 10)
	}
}

func (q quest) onTimer(s *script.Script, e script.Timer) string {
	npc := e.NPC
	rooms := q.levelRooms(npc)
	if rooms == nil {
		s.StartTimer(e.Name, npc, e.Player, 10000*time.Millisecond)
		return ""
	}
	tick := s.CurrentTick()
	roomIndex := int(npc.IntParam("RoomIndex"))
	room0 := rooms[roomIndex]

	switch {
	case strings.EqualFold(e.Name, timerRoomWatch):
		watchRoom(s, e, room0, roomIndex, tick)
	case strings.EqualFold(e.Name, timerRoomJump):
		jump(s, e, rooms, room0, roomIndex)
	}
	return ""
}

// ousted puts the party of the room out of the rift: its members get -1 in
// the memo state slot, the room is emptied and its monsters leave.
func ousted(s *script.Script, npc *script.NPC, room *script.RoomCell, memo func(p *script.Player)) {
	eachMember(s, room, memo)
	oustPlayers(s, npc)
	makerEvent(s, npc, "0")
	room.Clear()
}

// watchRoom is the watch of a guardian over its room, every ten seconds.
func watchRoom(s *script.Script, e script.Timer, room *script.RoomCell, roomIndex int, tick int32) {
	npc := e.NPC
	if !room.Locked() {
		oustPlayers(s, npc)
		return
	}
	if tick-room.Time() < 15 {
		makerEvent(s, npc, "1")
		return
	}
	if room.PartyChanged() || outside(npc, room.PartyLeader()) {
		if tick-room.Time() > 3 {
			for _, id := range room.MemberIDs() {
				if p := s.WorldPlayer(id); p != nil {
					s.SetMemoState(p, questID, -1)
				}
			}
			oustPlayers(s, npc)
			makerEvent(s, npc, "0")
			room.Clear()
		}
		return
	}

	rift := s.EventRules().Rift
	anakazelWaits := roomIndex == anakazelRoom && npc.Int(script.SlotIntAI1) == 2
	if tick-room.Time() <= int32(60*rift.JumpDelayMin+s.Rnd(rift.JumpDelayRnd)*30) {
		if roomIndex == anakazelRoom && npc.Int(script.SlotIntAI1) == 3 {
			s.StartTimer(timerRoomJump, npc, e.Player, 5000*time.Millisecond)
			quake(room, 10)
		}
		return
	}
	if npc.Int(script.SlotIntAI2) == 1 {
		if !anakazelWaits {
			room.SetLock(false)
			makerEvent(s, npc, "0")
			npc.SetInt(script.SlotIntAI2, 0)
		}
		return
	}

	started := int32(-1)
	for _, id := range room.MemberIDs() {
		if p := s.WorldPlayer(id); p != nil {
			started = s.MemoState(p, questID)
			break
		}
	}
	if anakazelWaits {
		return
	}
	if started+60*40 < tick {
		ousted(s, npc, room, func(p *script.Player) { s.SetMemoStateEx(p, questID, 1, -1) })
		return
	}
	s.StartTimer(timerRoomJump, npc, e.Player, 5000*time.Millisecond)
	quake(room, 20)
}

// jump sends the party of a room on, to Anakazel or to another room.
func jump(s *script.Script, e script.Timer, rooms []*script.RoomCell, room0 *script.RoomCell, roomIndex int) {
	npc := e.NPC
	forget := func(p *script.Player) { s.SetMemoStateEx(p, questID, 1, -1) }
	if !room0.Locked() {
		// The party gave up or changed rooms since the quake: the room was
		// emptied, and the jump has nobody left to move.
		return
	}
	if room0.PartyChanged() {
		ousted(s, npc, room0, forget)
		return
	}
	if leader := room0.PartyLeader(); leader == nil || leader.Dead() || outside(npc, leader) {
		ousted(s, npc, room0, forget)
		return
	}
	if roomIndex == anakazelRoom && npc.Int(script.SlotIntAI1) == 2 {
		return
	}

	rift := s.EventRules().Rift
	anakazel := rooms[anakazelRoom]
	if roomIndex != anakazelRoom && s.Rnd(100) < rift.AnakazelPortChance && anakazel.TryLock() {
		anakazel.SetPartyFrom(room0)
		moveParty(npc, room0, anakazelRoom)
		makerEvent(s, npc, "0")
		room0.Clear()
		return
	}

	index := -1
	for range regularRooms {
		if i := s.Rnd(regularRooms); rooms[i].TryLock() {
			index = i
			break
		}
	}
	for i := 0; i < regularRooms && index == -1; i++ {
		if rooms[i].TryLock() {
			index = i
		}
	}
	if index != -1 {
		rooms[index].SetPartyFrom(room0)
		makerEvent(s, npc, "0")
		moveParty(npc, room0, index)
	}
	room0.Clear()
}
