// Package fortressofresistance is the Fortress of Resistance's hall
// script. Bloody Lord Nurka and his partisans hold the fortress; the siege
// raises him, in his first form for a hall nobody owned and in his second
// for one a clan held, and the clan that dealt him the most damage takes
// the fortress once he falls. The fortress keeps no attacker list: its
// siege is fought even with nobody registered.
package fortressofresistance

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// fortressOfResistance is the Fortress of Resistance's hall id.
const fortressOfResistance int32 = 21

// Brakel, the partisans' herald, who names the next siege's date.
const brakel = 35382

// Nurka's two forms and his partisans.
const (
	bloodyLordNurka1    = 35368
	bloodyLordNurka2    = 35375
	partisanHealer      = 35369
	partisanCourtGuard1 = 35370
	partisanCourtGuard2 = 35371
	partisanSoldier     = 35372
	partisanSorcerer    = 35373
	partisanArcher      = 35374
)

// ddMagic is the magic strike Nurka casts at his attackers.
const ddMagic = 4042

// ddMagicLevel1 returns the level 1 of ddMagic.
func ddMagicLevel1() skill.Ref { return skill.Ref{ID: ddMagic, Level: 1} }

// New returns the Fortress of Resistance's hall script.
func New() script.Script {
	// damage is, per clan, the damage it dealt Nurka this siege, each sum a
	// counter the attacked hook adds to.
	damage := &script.IntMapCell[*script.IntCell]{}
	base := script.Hooks{}.Named("ClanHallSiege")
	return script.Script{
		Dir: "siegablehall",
		Bind: script.Bindings{
			script.EventFirstTalk:    {brakel},
			script.EventAttacked:     {bloodyLordNurka1, bloodyLordNurka2},
			script.EventClanAttacked: {bloodyLordNurka1, bloodyLordNurka2, partisanHealer, partisanCourtGuard1, partisanCourtGuard2, partisanSoldier, partisanSorcerer, partisanArcher},
			script.EventCreated:      {bloodyLordNurka1, bloodyLordNurka2},
			script.EventMyDying:      {bloodyLordNurka1, bloodyLordNurka2},
			script.EventNoDesire:     {partisanSoldier},
			script.EventSeeSpell:     {bloodyLordNurka1, bloodyLordNurka2},
		},
		BindHall: func(s *script.Script) {
			s.BindHallSiege(fortressOfResistance, script.HallSiegeHooks{
				Winner: func(*script.Hall) int32 { return winner(s, damage) },
				SpawnNPCs: func(h *script.Hall) {
					s.Spawner().StartSpawnTime(spawnTime(h), "21")
				},
				UnspawnNPCs: func(h *script.Hall) {
					s.Spawner().StopSpawnTime(spawnTime(h), "21")
				},
				OwnAttackers: true,
			})
		},
		Hooks: base.With(script.Hooks{
			OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
				h := s.SiegableHall(fortressOfResistance)
				if h == nil {
					return ""
				}
				return strings.ReplaceAll(s.HTMLText("partisan_ordery_brakel001.htm"), "%nextSiege%", h.SiegeDate().Format("2006-01-02 15:04:05"))
			},
			OnAttacked: func(s *script.Script, e script.Attacked) {
				h := s.SiegableHall(fortressOfResistance)
				if h == nil || !h.InSiege() || !e.Attacker.Playable() {
					return
				}
				if s.Rnd(100) < 10 {
					e.NPC.AddCastDesire(e.Attacker, ddMagicLevel1(), 1000000)
				}
				if p := script.ActingPlayer(e.Attacker); p != nil {
					if clanID := p.ClanID(); clanID != 0 {
						counter, _ := damage.SetIfAbsent(clanID, &script.IntCell{})
						counter.Add(int(e.Damage))
					}
				}
				base.Attacked(s, e)
			},
			OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
				switch e.Called.NpcID() {
				case bloodyLordNurka1, bloodyLordNurka2:
					if s.Rnd(100) < 2 {
						e.Called.AddCastDesire(e.Attacker, ddMagicLevel1(), 1000000)
					}
				}
				base.ClanAttacked(s, e)
			},
			OnCreated: func(s *script.Script, e script.Created) {
				s.TeleportPlayersInTerritory(e.NPC, script.Loc{X: 51952, Y: 111060, Z: -1970}, 200)
				base.Created(s, e)
			},
			OnMyDying: func(s *script.Script, e script.MyDying) {
				if h := s.SiegableHall(fortressOfResistance); h != nil && h.InSiege() {
					h.SetMissionAccomplished(true)
					h.CancelSiegeTask()
					h.EndSiege()
				}
				base.MyDying(s, e)
			},
			OnNoDesire: func(*script.Script, script.NoDesire) {},
			OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
				if s.Rnd(100) < 15 {
					e.NPC.AddCastDesire(e.Caster, ddMagicLevel1(), 1000000)
				}
				base.SeeSpell(s, e)
			},
		}),
	}
}

// spawnTime names the makers the siege of h runs: the attack makers when a
// clan held the fortress as the registrations closed, else the defence
// makers.
func spawnTime(h *script.Hall) string {
	if h.PreviouslyOwned() {
		return "agit_attack_warfare_start"
	}
	return "agit_defend_warfare_start"
}

// winner names the clan that dealt Nurka the most damage, the first in the
// map's walk order among equals, and forgets every clan's damage; it is 0
// when no clan dealt him any, or when that clan is gone. The sums wrap as
// 32-bit ints.
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
