// Package defaultnpc is the root of the NPC behaviors: DefaultNpc, which
// reacts to nothing itself, and the helpers every behavior below it calls.
package defaultnpc

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// New is DefaultNpc. It sets no hook, so its own NPCs are bound to no
// event; its hooks are named, as every behavior derives from them.
func New() script.Script {
	return script.Script{Dir: "ai", Behavior: true, NPCs: npcIDs(), Hooks: script.Hooks{}.Named("DefaultNpc")}
}

// npcIDs are DefaultNpc's own NPCs.
func npcIDs() []int32 {
	return []int32{
		18004, 18257, 18258, 18259, 18260, 18261, 18262, 18263, 18264, 27195, 31003, 31004, 31005, 31006,
		31007, 31008, 31009, 31010, 31011, 31012, 31013, 31014, 31015, 31016, 31017, 31018, 31019, 31020,
		31021, 31022, 31023, 31024, 31025, 31026,
	}
}

// HateRatio returns the share, in percent, by which the NPC hates attacker
// more for what it is: the hate ratios of the class group, the class and
// the race the NPC's AI parameters name, each counted when attacker, a
// player, matches it. Anything but a player gives 0.
func HateRatio(n *script.NPC, attacker script.Creature) float64 {
	p, ok := attacker.(*script.Player)
	if !ok {
		return 0
	}
	ratio := 0.0
	if group, ok := n.StringParam("SetHateGroup"); ok && p.InClassGroup(group) {
		ratio += float64(n.IntParam("SetHateGroupRatio"))
	}
	if occupation, ok := n.StringParam("SetHateOccupation"); ok && p.SameOccupation(occupation) {
		ratio += float64(n.IntParam("SetHateOccupationRatio"))
	}
	if race, ok := n.StringParam("SetHateRace"); ok && p.SameRace(race) {
		ratio += float64(n.IntParam("SetHateRaceRatio"))
	}
	return ratio
}

// SkillByTypeOrDefault returns the skill the NPC's template lists under t,
// def when it lists none.
func SkillByTypeOrDefault(n *script.NPC, t npc.SkillType, def skill.Ref) skill.Ref {
	if ref, ok := n.SkillByType(t); ok {
		return ref
	}
	return def
}

// MaybeCastPetrify makes the NPC petrify attacker's player when that player
// is allied with the NPC's faction, Varka or Ketra, and stop hating
// attacker; it reports whether it did.
func MaybeCastPetrify(n *script.NPC, attacker script.Creature) bool {
	p := script.ActingPlayer(attacker)
	if p == nil {
		return false
	}
	cast := false
	if p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan") {
		cast = true
	}
	if p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan") {
		cast = true
	}
	if cast {
		n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
		n.StopHate(attacker)
	}
	return cast
}

// TryToAttack makes the NPC attack c when it stands in its territory. Its
// SetAggressiveTime AI parameter sets how long a newly spawned NPC waits:
// -1 waits 3 to 7 AI cycles, anything else does not wait.
func TryToAttack(s *script.Script, n *script.NPC, c script.Creature) {
	if !n.InMyTerritory() {
		return
	}
	switch aggressiveTime := n.IntParam("SetAggressiveTime"); {
	case aggressiveTime == -1:
		if n.LifeTime() >= int32(s.Rnd(5)+3) {
			n.AddAttackDesire(c, 200)
		}
	case aggressiveTime == 0:
		n.AddAttackDesire(c, 200)
	case n.LifeTime() > aggressiveTime+int32(s.Rnd(4)):
		n.AddAttackDesire(c, 200)
	default:
		n.AddAttackDesire(c, 200)
	}
}

// CreatePrivates spawns the NPC's privates, the ones its spawn declares or
// else its template's, when it has any.
func CreatePrivates(s *script.Script, n *script.NPC) {
	s.Spawner().CreatePrivates(n)
}

// MakeAttackEvent runs the script's own attacked hook as if attacker had
// attacked the NPC for damage, with no skill. When generatePartyAttacked is
// set and the NPC is in a party, it then calls the party-attacked hooks of
// its master, when alive and not the NPC itself, and of every live private
// of its party but the NPC: the NPC attacked by attacker called each.
func MakeAttackEvent(s *script.Script, n *script.NPC, attacker script.Creature, damage int32, generatePartyAttacked bool) {
	s.RunAttacked(n, attacker, damage)
	if !generatePartyAttacked || (!n.IsMaster() && !n.HasMaster()) {
		return
	}
	if master := n.Master(); master != nil && !master.Dead() && !script.Same(n, master) {
		s.RaisePartyAttacked(n, master, attacker, damage)
	}
	for _, minion := range n.Minions() {
		if script.Same(minion, n) || minion.Dead() {
			continue
		}
		s.RaisePartyAttacked(n, minion, attacker, damage)
	}
}

// OpenCloseDoor opens the door named doorName when state is 0 and closes it
// otherwise; no such door, nothing happens.
func OpenCloseDoor(s *script.Script, doorName string, state int32) {
	d := s.Door(doorName)
	if d == nil {
		return
	}
	if state == 0 {
		d.Open()
	} else {
		d.Close()
	}
}

// PledgeCastleState returns where c's player stands towards the castle the
// NPC belongs to: 2 when its clan defends or owns the castle in the
// castle's siege under way, or owns the castle; 1 when its clan attacks
// the castle in that siege; 0 otherwise, and for a creature no player
// controls, a player in no clan or an NPC of no castle.
func PledgeCastleState(s *script.Script, n *script.NPC, c script.Creature) int32 {
	if !c.Playable() {
		return 0
	}
	p := script.ActingPlayer(c)
	if p == nil {
		return 0
	}
	castle := s.Castle(n)
	if !p.InClan() || castle == nil {
		return 0
	}
	if castle.SiegeInProgress() {
		if castle.OnSiegeSide(p, script.SiegeDefender, script.SiegeOwner) {
			return 2
		}
		if castle.OnSiegeSide(p, script.SiegeAttacker) {
			return 1
		}
	}
	if castle.ID() == p.ClanCastleID() {
		return 2
	}
	return 0
}
