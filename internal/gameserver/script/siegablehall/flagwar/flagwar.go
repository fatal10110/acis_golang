// Package flagwar is the flag war base of the siegable clan halls fought
// as a flag war: the Bandit Stronghold and the Wild Beast Reserve. Up to
// five clans register, each under a flag and with the NPC its leader picks;
// the clan owning the hall defends under the royal flag. The siege opens
// with tryouts, in which every challenger's warrior fights over its flag
// until one stands, then a final between the last challenger and the
// owner. The clan whose warrior stands last takes the hall.
package flagwar

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// The system messages a flag war sends.
const (
	msgTryoutsBegun        = 789
	msgFinalsBegun         = 790
	msgFinalMatchBegin     = 791
	msgTryoutsAboutToBegin = 815
	msgSiegeBegun          = 844
	msgClanWonMatch        = 857 // clan name, hall name
	msgMatchDraw           = 858 // hall name
	msgWarBeginsInMinutes  = 1826
	msgWarBeginsInSeconds  = 1828
)

// The timers a flag war runs its delayed steps on; seconds names one per
// countdown second.
const (
	timerClearTeleport = "clear-tp-delay"
	timerMoveToCenter  = "flagwar-move-to-center"
	timerOneMinute     = "flagwar-one-minute"
	timerSeconds       = "flagwar-seconds-"
	timerTryouts       = "flagwar-tryouts"
	timerFinal         = "flagwar-final"
)

// Config is what a flag war hall sets: its hall, the maker of its siege
// NPCs, its flags (royal, then red to purple), its five warriors, its first
// teleporter (the royal flag's; the flags' follow it in order), its
// messenger, its outer and inner doors, the points the flags and warriors
// stand on (the five challengers' first, then the owner's and the
// challenger's of the final) and the centre its warriors walk to.
type Config struct {
	HallID    int32
	MakerName string

	RoyalFlag, FlagRed, FlagYellow, FlagGreen, FlagBlue, FlagPurple int32

	Ally1, Ally2, Ally3, Ally4, Ally5 int32

	Teleport1 int32
	Messenger int32

	OuterDoors, InnerDoors []int32
	FlagCoords             []script.Loc
	Center                 script.Loc
}

// ClanData is a clan's registration: its flag, the NPC its leader picked
// (0 for none), the members it registered and, once the siege started,
// those of them who were in the world.
type ClanData struct {
	Flag, NPC int32
	Players   *script.ListCell[int32]
	Online    *script.ListCell[*script.Player]
}

func newClanData(flag, npc int32) ClanData {
	return ClanData{Flag: flag, NPC: npc, Players: &script.ListCell[int32]{}, Online: &script.ListCell[*script.Player]{}}
}

// War is a flag war hall's state, shared by its hooks and its siege steps.
type War struct {
	Config
	data         *script.IntMapCell[ClanData]
	messenger    *script.NPCCell
	winner       *script.IntCell
	ownersNPC    *script.IntCell
	firstPhase   *script.BoolCell
	accomplished *script.BoolCell

	npcSpawns, tentSpawns, clanlessNPCSpawns, clanlessTentSpawns *script.ListCell[*script.NPC]
}

// New returns the flag war of the hall cfg describes, in its first phase
// with no clan registered.
func New(cfg Config) *War {
	w := &War{
		Config:             cfg,
		data:               script.NewIntMapCell[ClanData](6),
		messenger:          &script.NPCCell{},
		winner:             &script.IntCell{},
		ownersNPC:          &script.IntCell{},
		firstPhase:         &script.BoolCell{},
		accomplished:       &script.BoolCell{},
		npcSpawns:          &script.ListCell[*script.NPC]{},
		tentSpawns:         &script.ListCell[*script.NPC]{},
		clanlessNPCSpawns:  &script.ListCell[*script.NPC]{},
		clanlessTentSpawns: &script.ListCell[*script.NPC]{},
	}
	w.firstPhase.Set(true)
	return w
}

// Bind is what a flag war hall's script registers for: the first talk of
// its messenger, flags and teleporters, the talk of its messenger, and the
// appearance of its messenger and warriors and the death of its warriors.
func (w *War) Bind() script.Bindings {
	firstTalk := []int32{w.Messenger, w.RoyalFlag, w.FlagRed, w.FlagYellow, w.FlagGreen, w.FlagBlue, w.FlagPurple}
	for i := range int32(6) {
		firstTalk = append(firstTalk, w.Teleport1+i)
	}
	return script.Bindings{
		script.EventFirstTalk: firstTalk,
		script.EventTalked:    {w.Messenger},
		script.EventCreated:   {w.Messenger, w.Ally1, w.Ally2, w.Ally3, w.Ally4, w.Ally5},
		script.EventMyDying:   {w.Ally1, w.Ally2, w.Ally3, w.Ally4, w.Ally5},
	}
}

// Hall returns the flag war's hall, nil when it is not a siegable hall.
func (w *War) Hall(s *script.Script) *script.Hall { return s.SiegableHall(w.HallID) }

// Clan returns clanID's registration.
func (w *War) Clan(clanID int32) (ClanData, bool) { return w.data.Get(clanID) }

// Clans returns the registrations, in the flag war's walk order.
func (w *War) Clans() []script.IntMapEntry[ClanData] { return w.data.Entries() }

// Registered returns how many clans are registered, the owner included.
func (w *War) Registered() int { return w.data.Len() }

// SetNPC records npcID as the NPC clanID's leader picked, and stores it.
func (w *War) SetNPC(h *script.Hall, clanID, npcID int32) {
	cd, ok := w.data.Get(clanID)
	if !ok {
		return
	}
	cd.NPC = npcID
	w.data.Set(clanID, cd)
	h.StoreFlagWarNPC(clanID, npcID)
}

// AddMember registers the player objectID for clanID, and stores it.
func (w *War) AddMember(h *script.Hall, clanID, objectID int32) {
	cd, ok := w.data.Get(clanID)
	if !ok {
		return
	}
	cd.Players.Add(objectID)
	h.StoreFlagWarMember(clanID, objectID)
}

// RegisterClan registers clanID, its leader as its first member, and
// stores it: the owner under the royal flag with the owner's NPC, any other
// clan under the next free flag. A clan owning another hall, or no clan,
// is refused (false).
func (w *War) RegisterClan(s *script.Script, h *script.Hall, clanID int32) (ClanData, bool) {
	clan := s.Clan(clanID)
	if clan == nil {
		return ClanData{}, false
	}
	if clan.ClanHallID() > 0 && clan.ClanHallID() != h.ID() {
		return ClanData{}, false
	}
	isOwner := clan.ID() == h.OwnerID()
	h.AddAttacker(clan.ID())
	flag := w.RoyalFlag
	if !isOwner {
		flag = w.RoyalFlag + int32(w.data.Len()) + 1
	}
	cd := newClanData(flag, 0)
	cd.Players.Add(clan.LeaderID())
	if isOwner {
		cd.NPC = int32(w.ownersNPC.Get())
	}
	w.data.Set(clan.ID(), cd)
	h.StoreFlagWarClan(clan.ID(), cd.Flag)
	h.StoreFlagWarMember(clan.ID(), clan.LeaderID())
	return cd, true
}

// BindHall binds the hall's siege to the flag war and loads its stored
// registrations; the hall's owner is the winner until a siege names one.
func (w *War) BindHall(s *script.Script) {
	h := s.BindHallSiege(w.HallID, script.HallSiegeHooks{
		Prepare:           func(h *script.Hall, _ script.HallBase, delay time.Duration) { w.prepare(s, h, delay) },
		Start:             func(h *script.Hall, _ script.HallBase) { w.start(s, h) },
		End:               func(h *script.Hall, _ script.HallBase) { w.end(s, h) },
		OwnAttackers:      true,
		DoorsUnattackable: true,
	})
	if h == nil {
		return
	}
	w.loadAttackers(h)
	w.winner.Set(int(h.OwnerID()))
}

// loadAttackers loads the stored registrations and the owner's NPC. The
// reference loads them before its hall sets its warriors, so an owner with
// no NPC stored keeps none.
func (w *War) loadAttackers(h *script.Hall) {
	for _, c := range h.FlagWarClans() {
		cd := newClanData(c.Flag, c.NPC)
		cd.Players.Add(c.Members...)
		w.data.Set(c.ClanID, cd)
	}
	if h.OwnerID() != 0 {
		if clanID, npcID, ok := h.FlagWarOwnerNPC(); ok && h.OwnerID() == clanID {
			w.ownersNPC.Set(int(npcID))
		}
	}
}

// Hooks returns the flag war's hooks: the first talk of its flags and
// teleporters, the appearance of its messenger and warriors, the death of
// its warriors and the timers of its delayed steps.
func (w *War) Hooks() script.Hooks {
	base := script.Hooks{}.Named("ClanHallSiege")
	return base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			var html string
			id := e.NPC.NpcID()
			if id >= w.RoyalFlag && id <= w.FlagPurple {
				return ""
			}
			if id != w.Messenger {
				index := id - w.Teleport1
				if (index == 0 && w.firstPhase.Get()) || e.NPC.Int(script.SlotIntAI1) != 0 {
					html = "teleporter_notyet.htm"
				} else {
					e.NPC.SetInt(script.SlotIntAI1, 1)
					teleportPlayers(s, e.NPC)
					s.StartTimer(timerClearTeleport, e.NPC, e.Player, 60000*time.Millisecond)
				}
			}
			return html
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			if h := w.Hall(s); h != nil && h.InSiege() {
				w.dying(s, h, e.NPC)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			if e.NPC.NpcID() == w.Messenger {
				w.messenger.Set(e.NPC)
			} else {
				s.StartTimer(timerMoveToCenter, e.NPC, nil, time.Duration((s.Rnd(6)+5)*1000)*time.Millisecond)
			}
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			switch {
			case strings.EqualFold(e.Name, timerClearTeleport):
				e.NPC.SetInt(script.SlotIntAI1, 0)
			case e.Name == timerMoveToCenter:
				e.NPC.AddMoveToDesire(w.Center, 5)
			case e.Name == timerOneMinute:
				w.announce(script.SiegeMessage{ID: msgWarBeginsInMinutes, Number: 1, HasNumber: true})
			case strings.HasPrefix(e.Name, timerSeconds):
				secs, _ := strconv.ParseInt(strings.TrimPrefix(e.Name, timerSeconds), 10, 32)
				w.announce(script.SiegeMessage{ID: msgWarBeginsInSeconds, Number: int32(secs), HasNumber: true})
			case e.Name == timerTryouts:
				if h := w.Hall(s); h != nil {
					w.tryouts(s, h)
				}
			case e.Name == timerFinal:
				if h := w.Hall(s); h != nil {
					w.final(s, h)
				}
			}
			return ""
		},
	}).Named("FlagWar")
}

// dying runs a warrior's death during the siege: its clan, when another
// remains, leaves the war. In the tryouts a lone challenger of an unowned
// hall wins it, the last challenger of an owned one goes to the final and
// any other outcome ends the siege; in the final the last clan standing
// wins.
func (w *War) dying(s *script.Script, h *script.Hall, n *script.NPC) {
	owner := h.OwnerID()
	keep := 0
	if owner != 0 {
		keep = 1
	}
	if w.data.Len() > keep && n.ClanID() > 0 {
		w.removeParticipant(s, h, n.ClanID(), true)
	}
	w.clanlessNPCSpawns.Remove(n)
	clanIDs := w.data.Keys()
	if !w.firstPhase.Get() {
		w.doUnSpawns()
		w.accomplished.Set(true)
		w.winner.Set(int(clanIDs[0]))
		h.EndSiege()
		return
	}
	// Warriors standing for no clan are still to be beaten.
	if w.clanlessNPCSpawns.Len() > 0 && len(clanIDs) > keepOwned(owner) {
		return
	}
	switch {
	case len(clanIDs) == 1 && owner <= 0:
		// The hall was free: the last flag standing wins it.
		w.doUnSpawns()
		w.accomplished.Set(true)
		w.winner.Set(int(clanIDs[0]))
		h.CancelSiegeTask()
		h.EndSiege()
	case w.data.Len() == 2 && owner > 0:
		w.startFinal(s, h, owner)
	default:
		w.doUnSpawns()
		h.CancelSiegeTask()
		h.EndSiege()
	}
}

// keepOwned is how many clans an owned hall keeps besides its challengers.
func keepOwned(owner int32) int {
	if owner > 0 {
		return 1
	}
	return 0
}

// startFinal ends the tryouts of an owned hall with one challenger left:
// everyone is told it won its match and the final begins, the final lasts
// 40 minutes, the battlefield turns off and the inner doors open while the
// two flags stand on their final points; the warriors follow 5 minutes
// later.
func (w *War) startFinal(s *script.Script, h *script.Hall, owner int32) {
	var challenger int32
	for _, id := range w.data.Keys() {
		if id != owner {
			challenger = id
			break
		}
	}
	// A challenger that is no clan panics here, as the reference throws.
	w.announce(script.SiegeMessage{ID: msgClanWonMatch, Text: s.Clan(challenger).Name(), CastleID: int(h.ID())})
	w.announce(script.SiegeMessage{ID: msgFinalMatchBegin})
	h.CancelSiegeTask()
	h.ArmEnd(40 * 60 * 1000 * time.Millisecond)
	w.firstPhase.Set(false)
	h.SetBattlefieldActive(false)
	for _, door := range w.InnerDoors {
		h.OpenDoor(door)
	}
	w.doUnSpawns()
	for _, e := range w.data.Entries() {
		w.doSpawns(s, h, e.Key, e.Value, true, false)
	}
	s.StartTimer(timerFinal, nil, nil, 300000*time.Millisecond)
}

// final starts the final's fight: the inner doors close, the warriors
// stand by their flags, the hall's maker hears the final's event and the
// battlefield turns on.
func (w *War) final(s *script.Script, h *script.Hall) {
	w.announce(script.SiegeMessage{ID: msgFinalsBegun})
	for _, door := range w.InnerDoors {
		h.CloseDoor(door)
	}
	for _, e := range w.data.Entries() {
		w.doSpawns(s, h, e.Key, e.Value, false, true)
	}
	w.sendMakerEvent(s, h, "onFlagWarFinalEvent")
	h.SetBattlefieldActive(true)
}

// prepare closes the registrations: the owner registers when it has not,
// the players of other clans are thrown out of the grounds and the siege
// starts after delay.
func (w *War) prepare(s *script.Script, h *script.Hall, delay time.Duration) {
	w.registerOwnerIfAbsent(s, h)
	h.BanishForeigners()
	h.SetSiegeStatus(script.SiegeRegistrationOver)
	h.ArmStart(delay)
}

// start starts the siege. With too few clans it is called off: every clan
// leaves, the registrations open for the next date and no siege is armed.
// Otherwise the outer doors open for the challengers, the flags stand and
// the tryouts begin 5 minutes later, counted down.
func (w *War) start(s *script.Script, h *script.Hall) {
	w.removeInvalidRegistrations(h)
	w.winner.Set(0)
	w.firstPhase.Set(true)
	w.accomplished.Set(false)
	owner := h.OwnerID()
	needed := 1
	if owner > 0 {
		needed = 2
	}
	if len(w.data.Keys()) < needed {
		for _, clanID := range w.data.Keys() {
			w.removeParticipant(s, h, clanID, owner != clanID)
		}
		h.ClearFlagWar()
		h.ClearAttackers()
		h.UpdateNextSiege()
		h.SetSiegeStatus(script.SiegeRegistrationOpened)
		w.announce(script.SiegeMessage{ID: siege.MsgSiegeCanceledNoInterest, CastleID: int(h.ID())})
		return
	}
	w.announce(script.SiegeMessage{ID: msgSiegeBegun, CastleID: int(h.ID())})
	w.announce(script.SiegeMessage{ID: msgTryoutsAboutToBegin})
	s.StartTimer(timerOneMinute, nil, nil, 240000*time.Millisecond)
	for _, secs := range []int{20, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1} {
		s.StartTimer(timerSeconds+strconv.Itoa(secs), nil, nil, time.Duration((300-secs)*1000)*time.Millisecond)
	}
	for _, door := range w.OuterDoors {
		h.OpenDoor(door)
	}
	s.StartTimer(timerTryouts, nil, nil, 300000*time.Millisecond)
	for _, e := range w.data.Entries() {
		w.doSpawns(s, h, e.Key, e.Value, true, false)
		for _, objID := range e.Value.Players.Items() {
			if p := s.WorldPlayer(objID); p != nil {
				e.Value.Online.Add(p)
			}
		}
	}
	w.doClanlessNPCSpawns(s, true, false)
}

// tryouts begins the tryouts: the outer doors close, the players in the
// grounds of no clan or not registered are thrown out, the owner's are
// gathered on its spawn points, the battlefield turns on, the warriors
// stand by their flags and by the free ones, and the first phase lasts 20
// minutes.
func (w *War) tryouts(s *script.Script, h *script.Hall) {
	w.announce(script.SiegeMessage{ID: msgTryoutsBegun})
	for _, door := range w.OuterDoors {
		h.CloseDoor(door)
	}
	allowed := map[int32]bool{}
	for _, cd := range w.data.Values() {
		for _, objID := range cd.Players.Items() {
			allowed[objID] = true
		}
	}
	// The reference teleports at once, so a player thrown out is no longer
	// in the grounds as the owner's are gathered, and every registered
	// owner's member left is; here a teleport lands after this step, so the
	// thrown out are left out by hand.
	banished := map[int32]bool{}
	for _, p := range h.PlayersInside() {
		if p.ClanID() == 0 || !allowed[p.ObjectID()] {
			banished[p.ObjectID()] = true
			if at, ok := h.RandomSpawn(script.HallSpawnBanish); ok {
				p.TeleportTo(at, 20)
			}
		}
	}
	if owner := h.OwnerID(); owner > 0 && s.Clan(owner) != nil {
		for _, p := range h.PlayersInside() {
			if banished[p.ObjectID()] || p.ClanID() != owner {
				continue
			}
			if at, ok := h.RandomSpawn(script.HallSpawnOwner); ok {
				p.TeleportTo(at, 0)
			}
		}
	}
	h.SetBattlefieldActive(true)
	for _, e := range w.data.Entries() {
		w.doSpawns(s, h, e.Key, e.Value, false, true)
	}
	w.sendMakerEvent(s, h, "onSiegeStart")
	w.doClanlessNPCSpawns(s, false, true)
	h.SetSiegeStatus(script.SiegeInProgress)
	h.CancelSiegeTask()
	h.ArmEnd(20 * 60 * 1000 * time.Millisecond)
}

// end ends the siege: the hall's maker hears the end, the war's NPCs go,
// the hall goes to the winner, back to its owner after a drawn first phase,
// or stays free; everyone is told, the grounds and doors are reset, the
// attackers are let go, the next siege is armed and every clan leaves the
// war.
func (w *War) end(s *script.Script, h *script.Hall) {
	h.CancelSiegeTask()
	w.sendMakerEvent(s, h, "onSiegeEnd")
	w.doUnSpawns()
	prevOwner := h.OwnerID()
	// An owner that is no clan panics here, as the reference throws.
	if prevOwner > 0 {
		_ = s.Clan(prevOwner).ID()
		h.Free()
	}
	winner := s.Clan(int32(w.winner.Get()))
	switch {
	case w.accomplished.Get() && winner != nil:
		h.SetOwner(winner.ID())
		h.Announce(clanhall.MsgClanDefeated, winner.Name())
	case w.firstPhase.Get():
		if prevOwner > 0 {
			h.SetOwner(s.Clan(prevOwner).ID())
		}
		h.Announce(msgMatchDraw, "")
	default:
		h.Announce(siege.MsgSiegeDraw, "")
	}
	h.Announce(clanhall.MsgSiegeFinished, "")
	w.accomplished.Set(false)
	h.SetBattlefieldActive(false)
	h.UpdateNextSiege()
	h.RespawnDoors()
	h.BanishForeigners()
	h.ReleaseAttackers()
	h.ArmPrepare()
	h.SetSiegeStatus(script.SiegeRegistrationOpened)
	owner := h.OwnerID()
	if owner > 0 && prevOwner != owner {
		if prevOwner > 0 {
			h.DeleteFlagWarOwnerNPC()
		}
		cd, ok := w.data.Get(owner)
		if !ok {
			panic(fmt.Sprintf("flagwar: hall %d's new owner %d is not registered", h.ID(), owner))
		}
		h.StoreFlagWarOwnerNPC(owner, cd.NPC)
	}
	if prevOwner > 0 && owner <= 0 {
		h.DeleteFlagWarOwnerNPC()
	}
	for _, clanID := range w.data.Keys() {
		w.removeParticipant(s, h, clanID, owner != clanID)
	}
	h.ClearFlagWar()
}

// doSpawns stands clanID's flag (tents) and warrior (npcs): in the first
// phase on the point of its flag, the owner standing nowhere; in the final
// on the owner's or the challenger's point. A warrior stands for its clan,
// the clan's picked NPC or the second warrior when it picked none. A point
// out of the list, or a template missing, stands nothing more.
func (w *War) doSpawns(s *script.Script, h *script.Hall, clanID int32, cd ClanData, tents, npcs bool) {
	first := w.firstPhase.Get()
	if first && clanID == h.OwnerID() {
		return
	}
	var index int32
	switch {
	case first:
		index = cd.Flag - w.FlagRed
	case clanID == h.OwnerID():
		index = 5
	default:
		index = 6
	}
	if index < 0 || int(index) >= len(w.FlagCoords) {
		return
	}
	loc := w.FlagCoords[index]
	if cd.Flag != 0 && tents {
		tent := s.Spawner().SpawnForClan(cd.Flag, loc, 0)
		if tent == nil {
			return
		}
		w.tentSpawns.Add(tent)
	}
	if npcs {
		npcID := cd.NPC
		if npcID == 0 {
			npcID = w.Ally2
		}
		if warrior := s.Spawner().SpawnForClan(npcID, loc, clanID); warrior != nil {
			w.npcSpawns.Add(warrior)
		}
	}
}

// doClanlessNPCSpawns stands, on each challenger's point no clan's flag
// takes, its flag (tents) and a second warrior standing for no clan
// (npcs). A template missing stands nothing more.
func (w *War) doClanlessNPCSpawns(s *script.Script, tents, npcs bool) {
	var taken []int32
	for _, cd := range w.data.Values() {
		taken = append(taken, cd.Flag-w.FlagRed)
	}
	for _, flg := range []int32{0, 1, 2, 3, 4} {
		if slices.Contains(taken, flg) {
			continue
		}
		loc := w.FlagCoords[flg]
		if tents {
			tent := s.Spawner().SpawnForClan(w.FlagRed+flg, loc, 0)
			if tent == nil {
				return
			}
			w.clanlessTentSpawns.Add(tent)
		}
		if npcs {
			warrior := s.Spawner().SpawnForClan(w.Ally2, loc, 0)
			if warrior == nil {
				return
			}
			w.clanlessNPCSpawns.Add(warrior)
		}
	}
}

// doUnSpawns takes the flags and warriors out of the world.
func (w *War) doUnSpawns() {
	for _, list := range []*script.ListCell[*script.NPC]{w.tentSpawns, w.npcSpawns, w.clanlessTentSpawns, w.clanlessNPCSpawns} {
		for _, n := range list.Take() {
			n.DeleteMe()
		}
	}
}

// removeParticipant drops clanID from the war, its players sent to town
// when teleport is set; its restart point goes with it.
func (w *War) removeParticipant(s *script.Script, h *script.Hall, clanID int32, teleport bool) {
	cd, ok := w.data.Delete(clanID)
	if !ok {
		return
	}
	h.DropClanRestartPoint(clanID)
	cd.Players.Clear()
	if teleport {
		for _, p := range cd.Online.Items() {
			p.TeleportToTown()
		}
	}
	cd.Online.Clear()
}

// removeInvalidRegistrations drops every challenger left with no flag.
func (w *War) removeInvalidRegistrations(h *script.Hall) {
	for _, e := range w.data.Entries() {
		if e.Key != h.OwnerID() && e.Value.Flag == 0 {
			w.data.Delete(e.Key)
			h.RemoveAttacker(e.Key)
		}
	}
}

// registerOwnerIfAbsent registers the hall's owner when it has not.
func (w *War) registerOwnerIfAbsent(s *script.Script, h *script.Hall) {
	if owner := h.OwnerID(); owner > 0 && !w.data.Has(owner) {
		w.RegisterClan(s, h, owner)
	}
}

// sendMakerEvent runs the hall maker's script event name, then points each
// clan's restart to town at its flag's teleporter while one stands.
func (w *War) sendMakerEvent(s *script.Script, h *script.Hall, name string) {
	if !s.Spawner().SendMakerEvent(w.MakerName, name, 0, 0) {
		panic("flagwar: no npcmaker " + w.MakerName)
	}
	for _, e := range w.data.Entries() {
		if tp := s.WorldNPC(w.Teleport1 + e.Value.Flag - w.RoyalFlag); tp != nil {
			h.SetClanRestartPoint(e.Key, tp.SpawnLocation())
		} else {
			h.DropClanRestartPoint(e.Key)
		}
	}
}

// announce sends m to the players within 8000 of the messenger.
func (w *War) announce(m script.SiegeMessage) {
	w.messenger.Get().SystemMessageInRadius(m, 8000)
}

// teleportPlayers sends the players within 250 of the teleporter n, and no
// more than 100 above or below it, to its first destination.
func teleportPlayers(s *script.Script, n *script.NPC) {
	posIndex := strconv.Itoa(s.Rnd(1) + 1)
	at := script.Loc{
		X: int(n.IntParamOr("Pos_x"+posIndex, 0)),
		Y: int(n.IntParamOr("Pos_y"+posIndex, 0)),
		Z: int(n.IntParamOr("Pos_z"+posIndex, 0)),
	}
	z := script.LocOf(n).Z
	for _, p := range n.PlayersInRadius(250) {
		if dz := script.LocOf(p).Z - z; dz <= 100 && dz >= -100 {
			p.TeleportTo(at, 0)
		}
	}
}
