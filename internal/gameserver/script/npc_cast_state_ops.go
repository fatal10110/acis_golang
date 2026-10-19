package script

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// The cast and attack state of a hostile NPC its behaviors read, and the
// distances they measure (#3748). The ones that go through (*NPC).hostile
// exist for hostile NPCs only (kindOps).

// PhysicalMuted reports whether the NPC is kept from its physical skills.
func (n *NPC) PhysicalMuted() bool {
	return n.hostile("check a physical mute").EffectList().IsAffected(effect.FlagPhysicalMuted)
}

// AttackingNow reports whether the NPC's attack is in flight: its swing
// started and its hit has not ended.
func (n *NPC) AttackingNow() bool { return n.hostile("check its attack").AttackingNow() }

// StopAttack aborts the NPC's attack in flight, its pending hit with it,
// and clears its bow's reuse wait. Nothing is shown to anyone.
func (n *NPC) StopAttack() { n.hostile("stop its attack").StopAttack() }

// MaxMP returns the NPC's maximum MP.
func (n *NPC) MaxMP() int32 { return int32(n.hostile("read its mana").MaxMPValue()) }

// MeetsHPMPConditions reports whether the NPC has the MP and HP its cast of
// ref at target takes, whatever mutes it. No target, or a skill the NPC's
// table lacks, does not.
func (n *NPC) MeetsHPMPConditions(target Creature, ref skill.Ref) bool {
	return n.hostile("check a cast's cost").MeetsHPMP(combatantOf(target), ref)
}

// MeetsHPMPDisabledConditions is MeetsHPMPConditions that also refuses a
// skill a mute keeps the NPC from: a magic one while it is muted, any other
// while it is physically muted.
func (n *NPC) MeetsHPMPDisabledConditions(target Creature, ref skill.Ref) bool {
	return n.hostile("check a cast's cost").MeetsHPMPDisabled(combatantOf(target), ref)
}

// CanAttemptCast reports whether the NPC may try its cast of ref at target
// as far as its skill reuse goes: with any skill of its waiting for its
// reuse, a stun, meditation, sleep, paralysis or fear locks every skill;
// otherwise ref's own reuse wait decides. No target, or a skill the NPC's
// table lacks, may not.
func (n *NPC) CanAttemptCast(target Creature, ref skill.Ref) bool {
	return n.hostile("check a skill's reuse").CanAttemptCast(combatantOf(target), ref)
}

// ActualSpoiler reports whether p is the player who spoiled the NPC.
func (n *NPC) ActualSpoiler(p *Player) bool {
	h := n.hostile("check its spoiler")
	return p != nil && p.combatant() != nil && h.SpoilerID() == p.ObjectID()
}

// SetRunningQuietly puts the NPC in its run stance when run is true, in its
// walk stance otherwise, without showing the change to the players around
// it: its moves take the stance's speed. A handle on nothing does nothing.
func (n *NPC) SetRunningQuietly(run bool) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.SetRunning(run)
	case *npc.Folk:
		o.SetRunning(run)
	}
}

// Distance3D returns the NPC's distance to at, a creature or a point.
func (n *NPC) Distance3D(at Place) float64 { return distance3D(locOf(n), at.loc()) }

// InRadius3D reports whether a and b, creatures or points, stand strictly
// within radius of each other.
func InRadius3D(a, b Place, radius int) bool {
	return distance3D(a.loc(), b.loc()) < float64(radius)
}

func distance3D(a, b Loc) float64 {
	dx, dy, dz := float64(a.X)-float64(b.X), float64(a.Y)-float64(b.Y), float64(a.Z)-float64(b.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// CastUnchecked starts the NPC's cast of ref at target at once, outside its
// desires and past every check a cast's start makes: the skill's reuse, its
// MP and HP, mutes, weapon and conditions. A cast the NPC already has in
// flight is replaced, its pending steps dropped and its end reported to no
// one. The cast then runs as any cast of the NPC: it is shown to the
// players around, charges its reuse and MP, and lands at its hit unless the
// MP or HP it takes there is short. No target, or a skill the NPC's table
// lacks, starts nothing.
func (n *NPC) CastUnchecked(target Creature, ref skill.Ref) {
	n.hostile("start a cast").CastUnchecked(combatantOf(target), ref)
}
