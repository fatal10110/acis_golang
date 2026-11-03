package script

import (
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// The siegable clan halls a hall script, behavior or quest reads and runs
// (#3502): the hall and its siege, the hall siege script's binding and its
// step overrides.

// Hall is a script's handle on one siegable clan hall and its siege.
type Hall struct {
	s   *clanhall.HallSiege
	env *Env
	// sc is the script the handle was made for.
	sc *Script
}

// HallSpawn names one of a hall's spawn lists.
type HallSpawn = residence.SpawnType

// The spawn lists of a hall.
const (
	HallSpawnOwner   = residence.SpawnOwner
	HallSpawnOther   = residence.SpawnOther
	HallSpawnChaotic = residence.SpawnChaotic
	HallSpawnBanish  = residence.SpawnBanish
)

// HallBase is a hall siege's base steps, which a step override runs as part
// of its own: Prepare(delay), Start() and End(). It is valid only while the
// override runs.
type HallBase = clanhall.SiegeBase

// HallSiegeHooks are a siegable hall's siege script. Each hook gets the
// hall. Its hooks run with no lock of the halls held; they must not take a
// siege step themselves (Hall.EndSiege): an override runs the base steps
// through its HallBase instead.
type HallSiegeHooks struct {
	// Winner names the clan that won the siege, 0 for none. The siege's
	// end asks it first.
	Winner func(h *Hall) int32
	// SpawnNPCs puts the hall's siege NPCs in the world as the siege
	// starts; UnspawnNPCs takes them out as it ends.
	SpawnNPCs, UnspawnNPCs func(h *Hall)
	// Prepare, Start and End, when set, run in place of the base steps:
	// the registrations' close (after delay, the siege's start is due),
	// the start and the end. The siege's timers and EndSiege run them.
	Prepare func(h *Hall, base HallBase, delay time.Duration)
	Start   func(h *Hall, base HallBase)
	End     func(h *Hall, base HallBase)
	// OwnAttackers keeps the stored attackers out as the script binds the
	// siege: the script loads its attackers itself.
	OwnAttackers bool
	// DoorsUnattackable keeps the hall's doors out of everyone's attacks,
	// even while the siege is fought.
	DoorsUnattackable bool
	// MiniGameWinner hears that winner broke gourd in the hall's mini game
	// (Hall.SetMiniGameWinner); gourd may be nil.
	MiniGameWinner func(h *Hall, gourd *NPC, winner *Player)
}

// SiegableHall returns the siegable clan hall id, nil when there is none.
func (s *Script) SiegableHall(id int32) *Hall {
	if s.env == nil || s.env.Halls == nil {
		return nil
	}
	hs, ok := s.env.Halls.Siege(id)
	if !ok {
		return nil
	}
	return &Hall{s: hs, env: s.env, sc: s}
}

// HallOf returns the siegable clan hall the NPC serves, nil for none: the
// one its maker names as its residence, else the one whose NPCs list its
// template. A handle on nothing serves none.
func (s *Script) HallOf(n *NPC) *Hall {
	if s.env == nil || s.env.Halls == nil || n.combatant() == nil {
		return nil
	}
	inst := n.instance()
	if id, ok := inst.Maker.ResidenceParam(); ok {
		return s.SiegableHall(int32(id))
	}
	id, ok := s.env.Halls.SiegableHallOfNPC(inst.Template.ID)
	if !ok {
		return nil
	}
	return s.SiegableHall(id)
}

// BindHallSiege makes hooks run siegable hall id's siege and returns the
// hall, nil for a hall that is not siegable, has no siege date or whose
// siege a script runs already. The hall takes its stored attackers, but
// with OwnAttackers, and its registrations close an hour before its siege
// date.
func (s *Script) BindHallSiege(id int32, hooks HallSiegeHooks) *Hall {
	h := s.SiegableHall(id)
	if h == nil {
		return nil
	}
	sc := clanhall.SiegeScript{OwnAttackers: hooks.OwnAttackers, DoorsUnattackable: hooks.DoorsUnattackable}
	if f := hooks.Winner; f != nil {
		sc.Winner = func() (winner int32) {
			s.runHall("winner", func() { winner = f(h) })
			return winner
		}
	}
	if f := hooks.SpawnNPCs; f != nil {
		sc.SpawnNPCs = func() { s.runHall("spawn npcs", func() { f(h) }) }
	}
	if f := hooks.UnspawnNPCs; f != nil {
		sc.UnspawnNPCs = func() { s.runHall("unspawn npcs", func() { f(h) }) }
	}
	if f := hooks.Prepare; f != nil {
		sc.Prepare = func(b clanhall.SiegeBase, delay time.Duration) { s.runHall("prepare", func() { f(h, b, delay) }) }
	}
	if f := hooks.Start; f != nil {
		sc.Start = func(b clanhall.SiegeBase) { s.runHall("start", func() { f(h, b) }) }
	}
	if f := hooks.End; f != nil {
		sc.End = func(b clanhall.SiegeBase) { s.runHall("end", func() { f(h, b) }) }
	}
	if f := hooks.MiniGameWinner; f != nil {
		sc.MiniGameWinner = func(gourd, winner attackable.Combatant) {
			s.runHall("mini game winner", func() { f(h, NPCOf(gourd), PlayerOf(winner)) })
		}
	}
	if _, ok := s.env.Halls.BindSiege(id, sc); !ok {
		return nil
	}
	return h
}

// SetMiniGameWinner tells the script running the hall's siege that winner
// broke gourd in the hall's mini game; gourd may be nil. A hall whose
// script keeps no mini game does nothing.
func (h *Hall) SetMiniGameWinner(gourd *NPC, winner *Player) {
	h.s.SetMiniGameWinner(gourd.combatant(), winner.combatant())
}

// ID returns the hall's id.
func (h *Hall) ID() int32 { return h.s.ID() }

// Name returns the hall's name.
func (h *Hall) Name() string { return h.s.Name() }

// OwnerID returns the clan owning the hall, 0 for none.
func (h *Hall) OwnerID() int32 { return h.s.OwnerID() }

// Unowned reports whether no clan owns the hall.
func (h *Hall) Unowned() bool { return h.s.OwnerID() == 0 }

// SiegeDate returns the next siege's date, on the local clock.
func (h *Hall) SiegeDate() time.Time { return time.UnixMilli(h.s.Date()) }

// SiegeLength returns how long the siege is fought.
func (h *Hall) SiegeLength() time.Duration { return h.s.SiegeLength() }

// Registering reports whether the siege takes registrations.
func (h *Hall) Registering() bool { return h.s.Registering() }

// WaitingBattle reports whether the registrations closed and the siege has
// not started.
func (h *Hall) WaitingBattle() bool { return h.s.WaitingBattle() }

// InSiege reports whether the siege is being fought.
func (h *Hall) InSiege() bool { return h.s.InProgress() }

// SetSiegeStatus sets where the siege stands in its cycle.
func (h *Hall) SetSiegeStatus(st SiegeStatus) { h.s.SetStatus(st) }

// UpdateNextSiege sets the next siege's date from now, by the hall's
// schedule, and stores it.
func (h *Hall) UpdateNextSiege() { h.s.UpdateNextSiege() }

// Attackers returns the attacking clans, in the order they joined.
func (h *Hall) Attackers() []int32 { return h.s.Attackers() }

// AddAttacker makes clanID an attacker of the siege, and stores it,
// reporting false when the clan attacks it already.
func (h *Hall) AddAttacker(clanID int32) bool { return h.s.AddAttacker(clanID) }

// RemoveAttacker drops clanID from the attackers, and stores it.
func (h *Hall) RemoveAttacker(clanID int32) { h.s.RemoveAttacker(clanID) }

// ClearAttackers drops every attacker, and stores it.
func (h *Hall) ClearAttackers() { h.s.ClearAttackers() }

// ReleaseAttackers lets the attackers go, as a siege's end does: each
// loses its headquarters and its members in the world their siege state,
// the players still on the battlefield are flagged and the attackers are
// dropped, and stored.
func (h *Hall) ReleaseAttackers() { h.s.ReleaseAttackers() }

// CancelSiegeTask stops the siege step armed last.
func (h *Hall) CancelSiegeTask() { h.s.CancelTask() }

// EndSiege ends the siege now, as its time running out does: the script's
// End when it has one.
func (h *Hall) EndSiege() { h.s.End() }

// ArmPrepare arms the registrations' close an hour before the next siege's
// date, as the siege step armed last.
func (h *Hall) ArmPrepare() { h.s.ArmPrepare() }

// ArmStart arms the siege's start after d, as the siege step armed last.
func (h *Hall) ArmStart(d time.Duration) { h.s.ArmStart(d) }

// ArmEnd arms the siege's end after d, as the siege step armed last.
func (h *Hall) ArmEnd(d time.Duration) { h.s.ArmEnd(d) }

// PreviouslyOwned reports whether a clan owned the hall as the
// registrations of its siege closed.
func (h *Hall) PreviouslyOwned() bool { return h.s.PreviouslyOwned() }

// SetMissionAccomplished records whether a clan won the siege; the base
// end then gives the hall to the winner.
func (h *Hall) SetMissionAccomplished(v bool) { h.s.SetMissionAccomplished(v) }

// RespawnDoors stands the hall's doors again at full HP, closed.
func (h *Hall) RespawnDoors() { h.s.RespawnDoors() }

// BanishForeigners throws the players of other clans than the owner's out
// of the hall's grounds.
func (h *Hall) BanishForeigners() { h.s.BanishForeigners() }

// Free takes the hall from its owner: its lease stops, the owner loses it
// and the hall its functions, and its doors close.
func (h *Hall) Free() { h.s.Free() }

// SetOwner gives the hall to clanID, reporting false for no clan.
func (h *Hall) SetOwner(clanID int32) bool { return h.s.SetOwner(clanID) }

// SetBattlefieldActive turns the siege's battlefield on or off.
func (h *Hall) SetBattlefieldActive(on bool) { h.s.SetBattlefieldActive(on) }

// Announce sends every player in the world system message msgID, with
// text as its first parameter when set and the hall's name after it.
func (h *Hall) Announce(msgID int, text string) {
	h.s.Announce(siege.Message{ID: msgID, Text: text, CastleID: int(h.s.ID())})
}

// SetClanRestartPoint makes at where clanID's players restart to town while
// the siege's battlefield is on and holds them.
func (h *Hall) SetClanRestartPoint(clanID int32, at Place) {
	l := at.loc()
	h.s.SetClanRestartPoint(clanID, location.Location{X: l.X, Y: l.Y, Z: l.Z})
}

// DropClanRestartPoint drops clanID's restart point.
func (h *Hall) DropClanRestartPoint(clanID int32) { h.s.DropClanRestartPoint(clanID) }

// RandomSpawn returns one of the hall's spawn points of kind, drawn at
// random; false when it lists none.
func (h *Hall) RandomSpawn(kind HallSpawn) (Loc, bool) {
	list := h.s.Spawns(kind)
	if len(list) == 0 || h.env.Rand == nil {
		return Loc{}, false
	}
	at := list[h.env.Rand(len(list))]
	return Loc{X: at.X, Y: at.Y, Z: at.Z}, true
}

// OpenDoor opens the hall's door doorID; a door that is not one of the
// hall's is left alone.
func (h *Hall) OpenDoor(doorID int32) { h.setDoorOpen(doorID, true) }

// CloseDoor closes the hall's door doorID; a door that is not one of the
// hall's is left alone.
func (h *Hall) CloseDoor(doorID int32) { h.setDoorOpen(doorID, false) }

func (h *Hall) setDoorOpen(doorID int32, open bool) {
	doors := h.env.Doors
	if doors == nil {
		return
	}
	obj, ok := doors.Door(int(doorID))
	if !ok || !slices.ContainsFunc(h.s.Gates(), func(g string) bool { return strings.EqualFold(g, obj.Template.Name) }) {
		return
	}
	doors.SetDoorOpen(int(doorID), open)
}

// ClanID returns the clan the NPC stands for: the one its spawn gave it,
// else the owner of the residence it serves; 0 for none, and for a handle
// on nothing.
func (n *NPC) ClanID() int32 {
	if n.combatant() == nil {
		return 0
	}
	return n.instance().NPCClanID()
}

// SpawnForClan spawns an NPC of npcID at at, on the ground below it,
// standing for the clan clanID before any residence it serves, and returns
// it; nil when npcID has no template or the NPC cannot be placed. A
// negative heading faces a random way. The NPC never respawns.
func (s *Spawner) SpawnForClan(npcID int32, at Place, clanID int32) *NPC {
	loc := at.loc()
	live, err := s.population().SpawnForClan(npcID, loc.X, loc.Y, loc.Z, loc.Heading, clanID)
	if err != nil {
		s.logRefused(npcID, err)
		return nil
	}
	return NPCOf(live)
}

// runHall runs fn, a hall siege hook of the script, as one invocation: a
// panic aborts it and is logged with its stack, as a hook's is.
func (s *Script) runHall(what string, fn func()) {
	r := s.registry
	if r == nil {
		fn()
		return
	}
	epoch := r.runs.Begin()
	defer r.runs.End(epoch)
	defer func() {
		if p := recover(); p != nil {
			r.log.Error().Str("script", s.path).Str("hook", "hall siege "+what).Interface("panic", p).Str("stack", string(debug.Stack())).Msg("script: hook panicked")
		}
	}()
	fn()
}

// BindHallSieges runs the hall siege binding (Script.BindHall) of every
// registered script that has one, in list order, each as one invocation.
// Run it once, after the halls are restored and before their sieges run.
func (r *Registry) BindHallSieges() {
	for _, e := range r.entries {
		if s := e.script; s != nil && s.BindHall != nil {
			s.runHall("binding", func() { s.BindHall(s) })
		}
	}
}
