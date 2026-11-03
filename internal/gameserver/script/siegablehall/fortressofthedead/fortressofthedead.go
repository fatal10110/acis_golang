// Package fortressofthedead is the Fortress of the Dead's hall script.
// Lidia von Hellmann and her vampires hold the fortress; the siege raises
// her, and the attacking clan that dealt her the most damage takes the
// fortress once she falls. Lidia raises Alfred and Giselle von Hellmann and
// her vampire leaders as she appears, and casts on herself every half
// minute until both her kin are dead. The siege is only fought at night:
// a start that comes by day waits for the next in-game midnight.
package fortressofthedead

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// fortressOfDead is the Fortress of the Dead's hall id.
const fortressOfDead int32 = 64

// Lidia, her kin and her vampires.
const (
	lidia           = 35629
	alfred          = 35630
	giselle         = 35631
	vampireSoldier  = 35633
	vampireCaster   = 35634
	vampireMagister = 35635
	vampireWarlord  = 35636
	vampireLeader1  = 35637
	vampireLeader2  = 35647
)

// lidiaSelfCast is the skill Lidia casts on herself every half minute.
const lidiaSelfCast = 4997

// New returns the Fortress of the Dead's hall script.
func New() script.Script {
	// damage is, per attacking clan, the damage it dealt Lidia this siege,
	// each sum a counter the attacked hook adds to.
	damage := &script.IntMapCell[*script.IntCell]{}
	base := script.Hooks{}.Named("ClanHallSiege")
	return script.Script{
		Dir: "siegablehall",
		Bind: script.Bindings{
			script.EventAttacked:  {lidia},
			script.EventCreated:   {lidia, alfred, giselle},
			script.EventMyDying:   {lidia, alfred, giselle},
			script.EventNoDesire:  {lidia, alfred, giselle, vampireSoldier, vampireCaster, vampireMagister, vampireWarlord, vampireLeader1, vampireLeader2},
			script.EventPartyDied: {lidia, alfred, giselle},
		},
		BindHall: func(s *script.Script) {
			s.BindHallSiege(fortressOfDead, script.HallSiegeHooks{
				Winner: func(*script.Hall) int32 { return winner(s, damage) },
				Start: func(h *script.Hall, b script.HallBase) {
					// The siege must start at night.
					if hoursLeft := s.GameHour(); hoursLeft < 0 || hoursLeft > 6 {
						h.CancelSiegeTask()
						h.ArmStart(time.Duration(24-hoursLeft) * 600000 * time.Millisecond)
						return
					}
					b.Start()
				},
				SpawnNPCs: func(*script.Hall) {
					s.Spawner().StartSpawnTime("agit_defend_warfare_start", "64")
				},
				UnspawnNPCs: func(*script.Hall) {
					s.Spawner().StopSpawnTime("agit_defend_warfare_start", "64")
				},
			})
		},
		Hooks: base.With(script.Hooks{
			OnTimer: func(s *script.Script, e script.Timer) string {
				if e.Name == "1001" {
					n := e.NPC
					n.AddCastDesire(n, skill.Ref{ID: lidiaSelfCast, Level: 1}, 1000000)
					if n.ScriptValue() > 1 {
						s.CancelTimer("1001", n, nil)
					}
				}
				return base.Timer(s, e)
			},
			OnAttacked: func(s *script.Script, e script.Attacked) {
				h := s.SiegableHall(fortressOfDead)
				if h == nil || !h.InSiege() || !e.Attacker.Playable() {
					return
				}
				if p := script.ActingPlayer(e.Attacker); p != nil {
					if clanID := p.ClanID(); clanID != 0 && slices.Contains(h.Attackers(), clanID) {
						counter, _ := damage.SetIfAbsent(clanID, &script.IntCell{})
						counter.Add(int(e.Damage))
					}
				}
				if n := e.NPC; !n.InMyTerritory() {
					n.CleanAllHate()
					n.TeleportTo(n.SpawnLocation())
				}
				base.Attacked(s, e)
			},
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				switch n.NpcID() {
				case lidia:
					n.Shout(script.NpcString(1010624))
					for _, p := range lidiaPrivates() {
						s.Spawner().CreateOnePrivateEx(n, p.id, script.Loc{X: p.x, Y: p.y, Z: p.z, Heading: p.heading}, 0, script.Params{})
					}
					s.StartTimerAtFixedRate("1001", n, nil, 0, 30000*time.Millisecond)
				case alfred:
					n.Shout(script.NpcString(1010636))
				case giselle:
					n.Shout(script.NpcString(1010637))
				}
				base.Created(s, e)
			},
			OnMyDying: func(s *script.Script, e script.MyDying) {
				n := e.NPC
				switch n.NpcID() {
				case lidia:
					n.Shout(script.NpcString(1010638))
					if h := s.SiegableHall(fortressOfDead); h != nil && h.InSiege() {
						h.SetMissionAccomplished(true)
						h.CancelSiegeTask()
						h.EndSiege()
					}
				case alfred:
					n.Shout(script.NpcString(1010625))
				case giselle:
					n.Shout(script.NpcString(1010625))
				}
				base.MyDying(s, e)
			},
			OnNoDesire: func(*script.Script, script.NoDesire) {},
			OnPartyDied: func(s *script.Script, e script.PartyDied) {
				if e.Called.NpcID() == lidia {
					switch e.Caller.NpcID() {
					case alfred, giselle:
						e.Called.SetScriptValue(e.Called.ScriptValue() + 1)
					}
				}
				base.PartyDied(s, e)
			},
		}),
	}
}

// winner names the attacking clan that dealt Lidia the most damage, the
// first in the map's walk order among equals, and forgets every clan's
// damage; it is 0 when no clan dealt her any, or when that clan is gone.
// The sums wrap as 32-bit ints.
func winner(s *script.Script, damage *script.IntMapCell[*script.IntCell]) int32 {
	entries := damage.Entries()
	if len(entries) == 0 {
		return 0
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if int32(e.Value.Get()) > int32(best.Value.Get()) {
			best = e
		}
	}
	damage.Clear()
	if clan := s.Clan(best.Key); clan != nil {
		return clan.ID()
	}
	return 0
}

// private is one private Lidia raises: its NPC id, where it stands and the
// way it faces.
type private struct {
	id               int32
	x, y, z, heading int
}

// lidiaPrivates are the privates Lidia raises: Giselle, Alfred and 15
// vampire leaders.
func lidiaPrivates() []private {
	return []private{
		{giselle, 56619, -27866, 569, 54000},
		{alfred, 59282, -26496, 569, 48000},
		{vampireLeader2, 57905, -27648, 608, 33540},
		{vampireLeader2, 57905, -27712, 608, 33540},
		{vampireLeader2, 58233, -27182, 608, 33540},
		{vampireLeader2, 58233, -27232, 608, 33540},
		{vampireLeader2, 58233, -27282, 608, 33540},
		{vampireLeader2, 58233, -27332, 608, 33540},
		{vampireLeader2, 58233, -27382, 608, 33540},
		{vampireLeader2, 58233, -27432, 608, 33540},
		{vampireLeader2, 58233, -27482, 608, 33540},
		{vampireLeader2, 58233, -27532, 608, 33540},
		{vampireLeader2, 58233, -27582, 608, 33540},
		{vampireLeader2, 58233, -27632, 608, 33540},
		{vampireLeader2, 58233, -27682, 608, 33540},
		{vampireLeader2, 58233, -27732, 608, 33540},
		{vampireLeader2, 58233, -27782, 608, 33540},
	}
}
