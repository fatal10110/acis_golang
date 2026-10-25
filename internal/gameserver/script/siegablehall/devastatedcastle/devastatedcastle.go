// Package devastatedcastle is the Devastated Castle's hall script. An NPC
// clan under Gustav defends the castle; the siege raises it, and the
// attacking clan that dealt Gustav the most damage takes the castle once
// he falls. Gustav raises his lieutenants Dietrich and Mikhail and his
// knights and archers as he appears; the defenders go back home now and
// then, and flee to the inner hall once Gustav is all but dead.
package devastatedcastle

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// devastatedCastle is the Devastated Castle's hall id.
const devastatedCastle int32 = 34

// The castle's defenders.
const (
	dietrich    = 35408
	mikhail     = 35409
	gustav      = 35410
	doomServant = 35411
	doomGuard   = 35412
	doomArcher  = 35413
	doomTrooper = 35414
	doomWarrior = 35415
	doomKnight  = 35416
)

// The defenders' skills: the escape Gustav and his lieutenants cast on
// themselves, Gustav's and Mikhail's strikes, and the curse Dietrich casts
// on a healer.
const (
	escape     = 4235
	gustavHit  = 4236
	mikhailHit = 4237
	reuseCurse = 4238
)

// The classes whose spells Dietrich follows to their caster: the healers
// and the orc buffers.
const (
	classBishop        = 16
	classProphet       = 17
	classElvenElder    = 30
	classShillienElder = 43
	classOverlord      = 51
	classWarcryer      = 52
)

// level1 is the level 1 of skill id.
func level1(id skill.ID) skill.Ref { return skill.Ref{ID: id, Level: 1} }

// New returns the Devastated Castle's hall script.
func New() script.Script {
	// damage is, per attacking clan, the damage it dealt Gustav this
	// siege, each sum a counter the attacked hook adds to.
	damage := &script.IntMapCell[*script.IntCell]{}
	// speller is the last healer or buffer whose spell Dietrich saw.
	speller := &script.PlayerCell{}
	base := script.Hooks{}.Named("ClanHallSiege")
	return script.Script{
		Dir: "siegablehall",
		Bind: script.Bindings{
			script.EventAttacked:         {gustav},
			script.EventCreated:          {dietrich, mikhail, gustav},
			script.EventMyDying:          {gustav},
			script.EventNoDesire:         {dietrich, mikhail, gustav, doomServant, doomGuard, doomArcher, doomTrooper, doomWarrior, doomKnight},
			script.EventPartyAttacked:    {dietrich, mikhail, gustav, doomArcher, doomKnight},
			script.EventUseSkillFinished: {dietrich, mikhail},
			script.EventSeeSpell:         {dietrich},
		},
		BindHall: func(s *script.Script) {
			s.BindHallSiege(devastatedCastle, script.HallSiegeHooks{
				Winner: func(*script.Hall) int32 { return winner(s, damage) },
				SpawnNPCs: func(*script.Hall) {
					s.Spawner().StartSpawnTime("agit_defend_warfare_start", "34")
				},
				UnspawnNPCs: func(*script.Hall) {
					s.Spawner().StopSpawnTime("agit_defend_warfare_start", "34")
				},
			})
		},
		Hooks: base.With(script.Hooks{
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				if e.Name == "1001" {
					if !n.InMyTerritory() && s.Rnd(3) < 1 {
						n.CleanAllHate()
						n.TeleportTo(n.SpawnLocation())
					}
					if s.Rnd(5) < 1 {
						n.RandomizeAttack()
					}
				} else if e.Name == "1002" {
					n.DoDie(n)
				}
				return base.Timer(s, e)
			},
			OnAttacked: func(s *script.Script, e script.Attacked) {
				h := s.SiegableHall(devastatedCastle)
				if h == nil || !h.InSiege() || !e.Attacker.Playable() {
					return
				}
				if p := script.ActingPlayer(e.Attacker); p != nil {
					if clanID := p.ClanID(); clanID != 0 && slices.Contains(h.Attackers(), clanID) {
						counter, _ := damage.SetIfAbsent(clanID, &script.IntCell{})
						counter.Add(int(e.Damage))
					}
				}
				base.Attacked(s, e)
			},
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				switch n.NpcID() {
				case dietrich:
					n.Shout(script.NpcString(1000277))
				case mikhail:
					n.Shout(script.NpcString(1000276))
				case gustav:
					n.Shout(script.NpcString(1000275))
					for _, p := range gustavPrivates() {
						s.Spawner().CreateOnePrivateEx(n, p.id, script.Loc{X: p.x, Y: p.y, Z: p.z, Heading: 32768}, 0, script.Params{})
					}
				}
				// Dietrich, Mikhail and Gustav each check where they stand
				// once, a minute after they appear.
				s.StartTimer("1001", n, nil, 60000*time.Millisecond)
				base.Created(s, e)
			},
			OnMyDying: func(s *script.Script, e script.MyDying) {
				if h := s.SiegableHall(devastatedCastle); h != nil && h.InSiege() {
					h.SetMissionAccomplished(true)
					h.CancelSiegeTask()
					h.EndSiege()
				}
				base.MyDying(s, e)
			},
			OnNoDesire: func(*script.Script, script.NoDesire) {},
			OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
				called := e.Called
				switch called.NpcID() {
				case dietrich:
					if p := speller.Get(); p != nil && s.Rnd(3) < 1 && called.InSpawnTerritory(p) {
						called.AddCastDesire(p, level1(reuseCurse), 1000000)
					}
					if master := called.Master(); master != nil {
						escapeOnce(called, master, script.NpcString(1000280))
					}
				case mikhail:
					if s.Rnd(3) < 1 {
						called.AddCastDesire(e.Target, level1(mikhailHit), 1000000)
					}
					if master := called.Master(); master != nil {
						escapeOnce(called, master, script.NpcString(1000279))
					}
				case gustav:
					if s.Rnd(3) < 1 {
						called.AddCastDesire(e.Target, level1(gustavHit), 1000000)
					}
					escapeOnce(called, called, script.NpcString(1000278))
				}
				base.PartyAttacked(s, e)
			},
			OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
				switch e.Caster.ClassID() {
				case classBishop, classProphet, classElvenElder, classShillienElder, classOverlord, classWarcryer:
					speller.Set(e.Caster)
				}
				base.SeeSpell(s, e)
			},
			OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
				if e.Skill.ID == escape {
					n := e.NPC
					// The inner hall.
					n.CleanAllHate()
					n.TeleportTo(script.Loc{X: 177134, Y: -18807, Z: -2263})
					if n.NpcID() == gustav {
						s.StartTimer("1002", n, nil, 3000*time.Millisecond)
					}
				}
				base.UseSkillFinished(s, e)
			},
		}),
	}
}

// escapeOnce makes called, once, cast its escape on itself and say line,
// when watched is below 5% of its hit points.
func escapeOnce(called, watched *script.NPC, line string) {
	if called.IsScriptValue(0) && watched.HPRatio() < 0.05 {
		called.SetScriptValue(1)
		called.AddCastDesire(called, level1(escape), 1000000)
		called.Say(line)
	}
}

// winner names the clan that dealt Gustav the most damage, the first in
// the map's walk order among equals, and forgets every clan's damage; it
// is 0 when no clan dealt him any, or when that clan is gone. The sums
// wrap as 32-bit ints.
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

// private is one private Gustav raises: its NPC id and where it stands.
type private struct {
	id      int32
	x, y, z int
}

// gustavPrivates are the privates Gustav raises, each facing 32768:
// Mikhail, Dietrich, 15 doom knights and 15 doom archers.
func gustavPrivates() []private {
	return []private{
		{mikhail, 178304, -17712, -2194},
		{dietrich, 178306, -17535, -2195},
		{doomKnight, 178178, -17346, -2226},
		{doomKnight, 178110, -17768, -2226},
		{doomKnight, 178132, -17683, -2226},
		{doomKnight, 178180, -17953, -2226},
		{doomKnight, 178147, -17172, -2226},
		{doomKnight, 178056, -17424, -2226},
		{doomKnight, 178120, -17331, -2226},
		{doomKnight, 178124, -17577, -2226},
		{doomKnight, 178159, -18138, -2226},
		{doomKnight, 178396, -18050, -2226},
		{doomKnight, 178078, -17917, -2226},
		{doomKnight, 178261, -17269, -2226},
		{doomKnight, 178152, -18063, -2226},
		{doomKnight, 178025, -17541, -2226},
		{doomKnight, 178285, -18013, -2226},
		{doomArcher, 178112, -17968, -2221},
		{doomArcher, 178079, -18092, -2221},
		{doomArcher, 178039, -17732, -2221},
		{doomArcher, 178224, -18025, -2221},
		{doomArcher, 178111, -17510, -2221},
		{doomArcher, 178088, -17620, -2221},
		{doomArcher, 178059, -17223, -2221},
		{doomArcher, 178043, -17325, -2221},
		{doomArcher, 178311, -17946, -2221},
		{doomArcher, 178170, -17885, -2221},
		{doomArcher, 178317, -18097, -2221},
		{doomArcher, 178099, -17823, -2221},
		{doomArcher, 178290, -17335, -2221},
		{doomArcher, 178116, -17386, -2221},
		{doomArcher, 178169, -17266, -2221},
	}
}
