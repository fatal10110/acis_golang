// Package rainbowspringschateau is the Rainbow Springs Chateau's hall
// script. Clan leaders register their clan with the messenger by handing
// in every Rainbow Springs Clan Hall War Decree they hold; at the siege
// date the owner and the clans with the most decrees, four at most, are
// chosen, and an hour later each chosen clan's party enters its own arena
// through the caretaker to play the watering mini game.
//
// The mini game never gives the hall away: every end of the game, its
// winner found or its time run out, is a draw that leaves the hall free.
package rainbowspringschateau

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	minPartyCount      = 5
	minClanMemberCount = 5
	// waitTime is how long the chosen clans wait for the game to start.
	waitTime = 3600000 * time.Millisecond

	rainbowSprings int32 = 62

	warDecrees = 8034

	messenger = 35604
	caretaker = 35603
)

// The system messages the script sends.
const (
	msgSiegeBegun               = 844
	msgSiegeDraw                = 856
	msgRegistrationPeriodEnded  = 1823
	msgRegisteredForClanHallWar = 1824
	msgRegistrationFailed       = 1825
	msgClanHallWarBeginsMinutes = 1826
	msgClanHallWarBeginsSeconds = 1828
	msgClanHallWarCancelled     = 1841
)

// The hall's maker, which places the arena's cleaners as the game ends, and
// the spawn time of the arenas' NPCs.
const (
	rainbowMakerName   = "godard06_npc2414_02m1"
	makerEventSiegeEnd = "onSiegeEnd"
	defendSpawnTime    = "agit_defend_warfare_start"
)

// The script's delayed steps: the choice of the clans, the game's start
// and end, the end the hall's siege or the mini game asks for, the players'
// trip to town after it, and the countdown to the start.
const (
	timerFinalAttackers    = "rainbow-final-attackers"
	timerSiegeStart        = "rainbow-siege-start"
	timerSiegeEnd          = "rainbow-siege-end"
	timerEndNow            = "rainbow-end-now"
	timerTeleportOut       = "rainbow-teleport-out"
	timerRegistrationEnded = "rainbow-registration-ended"
	timerMinutes           = "rainbow-minutes-"
	timerSeconds           = "rainbow-seconds-"

	// siegeEndMargin is how long before the siege's length runs out the
	// game ends.
	siegeEndMargin = 120000 * time.Millisecond
	// teleportOutDelay is how long after the game's end the players left
	// on the battlefield go to town.
	teleportOutDelay = 120000 * time.Millisecond
)

// The links the messenger and the caretaker answer.
const (
	eventRegister    = "register"
	eventCancel      = "cancel"
	eventPortToArena = "portToArena"
)

// gourds are the gourds of the four arenas, the gourd of the n-th chosen
// clan's arena at n.
func gourds() [4]int32 { return [4]int32{35588, 35589, 35590, 35591} }

// arenas are where the four chosen clans' parties enter the game.
func arenas() [4]script.Loc {
	return [4]script.Loc{
		{X: 151562, Y: -127080, Z: -2214}, // Arena 1
		{X: 153141, Y: -125335, Z: -2214}, // Arena 2
		{X: 153892, Y: -127530, Z: -2214}, // Arena 3
		{X: 155657, Y: -125752, Z: -2214}, // Arena 4
	}
}

// chateau is the script's state. The clans' decrees are the hall
// runtime's.
type chateau struct {
	// accepted are the clans chosen to play, in their arenas' order.
	accepted *script.ListCell[int32]
	// caretaker is the arena's caretaker once it appeared.
	caretaker *script.NPCCell
	// registrationEnds is the registrations' deadline the messenger names,
	// in Unix milliseconds; 0 until set.
	registrationEnds *script.IntCell
}

// New returns the Rainbow Springs Chateau's hall script.
func New() script.Script {
	c := &chateau{accepted: &script.ListCell[int32]{}, caretaker: &script.NPCCell{}, registrationEnds: &script.IntCell{}}
	base := script.Hooks{}.Named("ClanHallSiege")
	return script.Script{
		Dir: "siegablehall",
		Bind: script.Bindings{
			script.EventCreated:   {caretaker},
			script.EventFirstTalk: {messenger, caretaker},
			script.EventTalked:    {messenger, caretaker},
		},
		BindHall: c.bindHall,
		Hooks: base.With(script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) {
				c.caretaker.Set(e.NPC)
			},
			OnFirstTalk: c.firstTalk,
			OnEvent:     c.event,
			OnTimer:     c.timer,
		}),
	}
}

// bindHall runs the hall's siege and arms the choice of its clans at its
// siege date. Its start does nothing and its end runs the game's end; the
// registrations' close an hour before the date stays the base one.
func (c *chateau) bindHall(s *script.Script) {
	h := s.BindHallSiege(rainbowSprings, script.HallSiegeHooks{
		Start: func(*script.Hall, script.HallBase) {},
		End: func(*script.Hall, script.HallBase) {
			s.CancelTimer(timerSiegeEnd, nil, nil)
			s.StartTimer(timerEndNow, nil, nil, 0)
		},
		SpawnNPCs:      func(*script.Hall) { c.spawnNPCs(s) },
		UnspawnNPCs:    func(*script.Hall) {},
		OwnAttackers:   true,
		MiniGameWinner: func(h *script.Hall, gourd *script.NPC, winner *script.Player) { c.miniGameWinner(s, h, gourd, winner) },
	})
	if h == nil {
		return
	}
	// The halls set a passed siege date anew from now as they are
	// restored, so the date lies ahead.
	if delay := h.SiegeDate().Sub(s.Now()); delay >= 0 {
		c.registrationEnds.Set(int(h.SiegeDate().UnixMilli()))
		s.StartTimer(timerFinalAttackers, nil, nil, delay)
	}
}

// firstTalk answers the messenger, naming the registrations' deadline and
// the clan that owns the hall, and the caretaker, open while the chosen
// clans wait for the game.
func (c *chateau) firstTalk(s *script.Script, e script.FirstTalk) string {
	h := s.SiegableHall(rainbowSprings)
	if h == nil {
		return ""
	}
	html := ""
	switch e.NPC.NpcID() {
	case messenger:
		main := "messenger_yetti001a.htm"
		if h.OwnerID() > 0 {
			main = "messenger_yetti001.htm"
		}
		html, _ = s.Page("data/html/script/siegablehall/RainbowSpringsChateau/" + main)
		html = strings.ReplaceAll(html, "%time%", registrationEndString(c.registrationEnds.Get()))
		if h.OwnerID() > 0 {
			if owner := s.Clan(h.OwnerID()); owner != nil {
				html = strings.ReplaceAll(html, "%owner%", owner.Name())
			}
		}
	case caretaker:
		html = "game_manager003.htm"
		if h.WaitingBattle() {
			html = "game_manager001.htm"
		}
	}
	return html
}

// event answers the messenger's registration links and the caretaker's
// arena entry. Any other link shows its own name.
func (c *chateau) event(s *script.Script, e script.Event) string {
	html := e.Name
	h := s.SiegableHall(rainbowSprings)
	if h == nil {
		return html
	}
	p := e.Player
	switch e.NPC.NpcID() {
	case messenger:
		switch e.Name {
		case eventRegister:
			html = register(s, h, p)
		case eventCancel:
			html = c.cancel(s, h, p)
		}
	case caretaker:
		if e.Name == eventPortToArena {
			html = c.portToArena(s, h, p, html)
		}
	}
	return html
}

// register registers the clan of p, its leader, handing in every war
// decree p holds.
func register(s *script.Script, h *script.Hall, p *script.Player) string {
	if !p.IsClanLeader() {
		return "messenger_yetti010.htm"
	}
	clanID := p.ClanID()
	switch {
	case p.ClanLevel() < 3 || s.Clan(clanID).MembersCount() < minClanMemberCount:
		return "messenger_yetti011.htm"
	case p.ClanHallID() > 0:
		return "messenger_yetti012.htm"
	case !h.Registering():
		return "messenger_yetti014.htm"
	}
	if _, registered := h.Decrees(clanID); registered {
		return "messenger_yetti013.htm"
	}
	if p.ItemCount(warDecrees) == 0 {
		return "messenger_yetti008.htm"
	}
	count := p.FirstItemCount(warDecrees)
	// The decrees go before the clan registers, so that no registration
	// stands on decrees the player still holds.
	if !s.DestroyItems(p, warDecrees, count, true) {
		return "messenger_yetti008.htm"
	}
	h.SetDecrees(clanID, int(count))
	return "messenger_yetti009.htm"
}

// cancel cancels the registration of the clan of p, its leader, while the
// registrations are open, giving back half its decrees.
func (c *chateau) cancel(s *script.Script, h *script.Hall, p *script.Player) string {
	html := eventCancel
	if !p.IsClanLeader() {
		return "messenger_yetti010.htm"
	}
	clanID := p.ClanID()
	if _, registered := h.Decrees(clanID); !registered {
		return "messenger_yetti016.htm"
	}
	if !h.Registering() || h.WaitingBattle() {
		return "messenger_yetti017.htm"
	}
	if decrees, ok := h.RemoveDecrees(clanID); ok {
		html = "messenger_yetti019.htm"
		s.GiveItems(p, warDecrees, int32(decrees/2))
	}
	c.accepted.Remove(clanID)
	return html
}

// portToArena sends the party of p, the leader of a chosen clan leading a
// party of at least five of its members, into its clan's arena: each member
// loses its effects and its summon first. A party sent in sees html, the
// link's own name.
func (c *chateau) portToArena(s *script.Script, h *script.Hall, p *script.Player, html string) string {
	clanID := p.ClanID()
	switch {
	case clanID == 0:
		return "game_manager009.htm"
	case !p.IsClanLeader():
		return "game_manager004.htm"
	case !p.InParty():
		return "game_manager005.htm"
	case p.PartyLeader().ObjectID() != p.ObjectID():
		return "game_manager006.htm"
	}
	members := p.PartyMembers()
	for _, m := range members {
		if m.ClanID() != clanID {
			return "game_manager007.htm"
		}
	}
	switch {
	case len(members) < minPartyCount:
		return "game_manager008.htm"
	case p.ClanHallID() > 0:
		return "game_manager010.htm"
	case p.ClanLevel() < h.MinClanLevel():
		return "game_manager011.htm"
	case s.Clan(clanID).MembersCount() < minClanMemberCount:
		return "game_manager012.htm"
	}
	index := c.accepted.Index(clanID)
	if index < 0 {
		if _, registered := h.Decrees(clanID); !registered {
			return "game_manager014.htm"
		}
		return "game_manager015.htm"
	}
	if index > 3 {
		return html
	}
	at := arenas()[index]
	for _, m := range members {
		m.StopAllEffects()
		if summon := m.ActiveSummon(); summon != nil {
			script.Unsummon(summon)
		}
		m.TeleportTo(at, 0)
	}
	return html
}

// timer runs the script's delayed steps.
func (c *chateau) timer(s *script.Script, e script.Timer) string {
	h := s.SiegableHall(rainbowSprings)
	if h == nil {
		return ""
	}
	switch {
	case e.Name == timerFinalAttackers:
		c.setFinalAttackers(s, h)
	case e.Name == timerSiegeStart:
		c.siegeStart(s, h)
	case e.Name == timerSiegeEnd, e.Name == timerEndNow:
		c.siegeEnd(s, h)
	case e.Name == timerTeleportOut:
		for _, p := range h.PlayersOnBattlefield() {
			p.TeleportToTown()
		}
	case e.Name == timerRegistrationEnded:
		c.tell(h, false, script.SiegeMessage{ID: msgRegistrationPeriodEnded})
		c.tell(h, false, script.SiegeMessage{ID: msgRegisteredForClanHallWar})
	case strings.HasPrefix(e.Name, timerMinutes):
		mins, _ := strconv.ParseInt(strings.TrimPrefix(e.Name, timerMinutes), 10, 32)
		c.tell(h, false, script.SiegeMessage{ID: msgClanHallWarBeginsMinutes, Number: int32(mins), HasNumber: true})
	case strings.HasPrefix(e.Name, timerSeconds):
		secs, _ := strconv.ParseInt(strings.TrimPrefix(e.Name, timerSeconds), 10, 32)
		c.tell(h, false, script.SiegeMessage{ID: msgClanHallWarBeginsSeconds, Number: int32(secs), HasNumber: true})
	}
	return ""
}

// setFinalAttackers chooses the clans that play: the owner, then the clans
// that handed in the most decrees, four clans in all; the leader of each
// clan left over is told it failed. With two clans or more the game starts
// after the wait, counted down; otherwise the siege is called off, every
// registration dropped and the next siege date armed.
func (c *chateau) setFinalAttackers(s *script.Script, h *script.Hall) {
	if owner := h.OwnerID(); owner > 0 {
		if clan := s.Clan(owner); clan != nil {
			c.accepted.Add(clan.ID())
		}
	}
	for _, clanID := range byDecrees(h) {
		clan := s.Clan(clanID)
		if c.accepted.Len() > 3 {
			// A clan no longer existing, or whose leader is away, is
			// passed over without a word.
			if clan != nil {
				if leader := clan.ClanLeader(); leader != nil {
					leader.SystemMessage(msgRegistrationFailed)
				}
			}
			continue
		}
		if clan == nil || clan.DissolvingExpiry() > 0 {
			h.RemoveDecrees(clanID)
		} else {
			c.accepted.Add(clan.ID())
		}
	}
	if c.accepted.Len() >= 2 {
		scheduleAnnouncements(s, waitTime)
		cancelNextSiege(s)
		s.StartTimer(timerSiegeStart, nil, nil, waitTime)
		h.SetSiegeStatus(script.SiegeRegistrationOver)
		h.ClearAttackers()
		for _, clanID := range c.accepted.Items() {
			h.AddAttacker(clanID)
		}
		return
	}
	h.UpdateNextSiege()
	cancelNextSiege(s)
	s.StartTimer(timerFinalAttackers, nil, nil, h.SiegeDate().Sub(s.Now()))
	h.SetSiegeStatus(script.SiegeRegistrationOpened)
	h.ClearAttackers()
	c.tell(h, true, script.SiegeMessage{ID: msgRegistrationPeriodEnded})
	c.tell(h, false, script.SiegeMessage{ID: msgClanHallWarCancelled})
	c.tell(h, false, script.SiegeMessage{ID: msgSiegeDraw, CastleID: int(rainbowSprings)})
	h.ClearDecrees()
	c.accepted.Clear()
	if owner := h.OwnerID(); owner > 0 && s.Clan(owner) != nil {
		h.Free()
		h.SetOwner(owner)
	}
}

// siegeStart starts the game: the hall is freed, the chosen clans are told,
// the siege is under way, its end is armed two minutes before its length
// runs out and the arenas fill.
func (c *chateau) siegeStart(s *script.Script, h *script.Hall) {
	if owner := h.OwnerID(); owner > 0 && s.Clan(owner) != nil {
		h.Free()
	}
	c.tell(h, false, script.SiegeMessage{ID: msgSiegeBegun, CastleID: int(rainbowSprings)})
	h.SetSiegeStatus(script.SiegeInProgress)
	s.StartTimer(timerSiegeEnd, nil, nil, h.SiegeLength()-siegeEndMargin)
	c.spawnNPCs(s)
}

// spawnNPCs fills the arenas and the caretaker tells the players around it
// the game began.
func (c *chateau) spawnNPCs(s *script.Script) {
	s.Spawner().StartSpawnTime(defendSpawnTime, "62")
	if n := c.caretaker.Get(); n != nil {
		n.SystemMessageInRadius(script.SiegeMessage{ID: msgSiegeBegun, CastleID: int(rainbowSprings)}, 8000)
	}
}

// siegeEnd ends the game as a draw: the chosen clans are told, the hall's
// maker places its cleaners, the arenas empty, the registrations open for
// the next siege date, every registration is dropped and the players left
// on the battlefield go to town two minutes later.
func (c *chateau) siegeEnd(s *script.Script, h *script.Hall) {
	c.tell(h, false, script.SiegeMessage{ID: msgSiegeDraw, CastleID: int(rainbowSprings)})
	if !s.Spawner().SendMakerEvent(rainbowMakerName, makerEventSiegeEnd, 0, 0) {
		panic("rainbowspringschateau: no npcmaker " + rainbowMakerName)
	}
	s.Spawner().StopSpawnTime(defendSpawnTime, "62")
	h.UpdateNextSiege()
	cancelNextSiege(s)
	s.StartTimer(timerFinalAttackers, nil, nil, h.SiegeDate().Sub(s.Now()))
	h.SetSiegeStatus(script.SiegeRegistrationOpened)
	h.ClearAttackers()
	c.registrationEnds.Set(int(h.SiegeDate().UnixMilli()))
	h.ClearDecrees()
	c.accepted.Clear()
	s.StartTimer(timerTeleportOut, nil, nil, teleportOutDelay)
}

// miniGameWinner ends the game once a player of a chosen clan broke the
// gourd of its clan's arena.
func (c *chateau) miniGameWinner(s *script.Script, h *script.Hall, gourd *script.NPC, winner *script.Player) {
	if !h.InSiege() || winner == nil || gourd == nil {
		return
	}
	clanID := winner.ClanID()
	if clanID == 0 {
		return
	}
	index, arenaGourds := c.accepted.Index(clanID), gourds()
	if index < 0 || index >= len(arenaGourds) {
		return
	}
	if gourd.NpcID() == arenaGourds[index] {
		s.CancelTimer(timerSiegeEnd, nil, nil)
		s.StartTimer(timerEndNow, nil, nil, 0)
	}
}

// scheduleAnnouncements counts down the game's start, due after delay, to
// the chosen clans.
func scheduleAnnouncements(s *script.Script, delay time.Duration) {
	s.StartTimer(timerRegistrationEnded, nil, nil, delay-time.Duration(3600*1000)*time.Millisecond)
	for _, mins := range []int{50, 40, 30, 20, 10, 5, 3, 2, 1} {
		s.StartTimer(timerMinutes+strconv.Itoa(mins), nil, nil, delay-time.Duration(mins*60*1000)*time.Millisecond)
	}
	for _, secs := range []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1} {
		s.StartTimer(timerSeconds+strconv.Itoa(secs), nil, nil, delay-time.Duration(secs*1000)*time.Millisecond)
	}
}

// cancelNextSiege cancels the next step of the siege's cycle: the choice of
// the clans or the game's start.
func cancelNextSiege(s *script.Script) {
	s.CancelTimer(timerFinalAttackers, nil, nil)
	s.CancelTimer(timerSiegeStart, nil, nil)
}

// tell sends m to the members in the world of the chosen clans, the owner
// left out when exceptOwner is set.
func (c *chateau) tell(h *script.Hall, exceptOwner bool, m script.SiegeMessage) {
	var clans []int32
	for _, clanID := range c.accepted.Items() {
		if exceptOwner && h.OwnerID() > 0 && h.OwnerID() == clanID {
			continue
		}
		clans = append(clans, clanID)
	}
	h.TellClans(clans, m)
}

// byDecrees returns the clans that handed in decrees for the hall, most
// decrees first, those with as many in the decrees' walk order.
func byDecrees(h *script.Hall) []int32 {
	entries := h.DecreeEntries()
	slices.SortStableFunc(entries, func(a, b script.IntMapEntry[int]) int { return cmp.Compare(b.Value, a.Value) })
	out := make([]int32, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Key)
	}
	return out
}

// registrationEndString writes ms, a date in Unix milliseconds, as the
// messenger names the registrations' deadline: year-month-day, the hour on
// a 12-hour clock and the minutes, with no zero padding but the minutes';
// empty for none.
func registrationEndString(ms int) string {
	if ms == 0 {
		return ""
	}
	t := time.UnixMilli(int64(ms))
	sep := ":"
	if t.Minute() < 10 {
		sep = ":0"
	}
	return strconv.Itoa(t.Year()) + "-" + strconv.Itoa(int(t.Month())) + "-" + strconv.Itoa(t.Day()) + " " + strconv.Itoa(t.Hour()%12) + sep + strconv.Itoa(t.Minute())
}
