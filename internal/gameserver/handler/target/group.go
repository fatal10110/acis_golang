package target

import (
	"slices"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// This file holds the group-targeting handlers: skills whose affected set is
// the caster's party, clan, alliance, a single member of one of those, or the
// corpses of allied players. The membership and duel/olympiad gating lives in
// the social layer; this package only consumes it through the narrow seams
// declared below. The live party/clan/alliance/duel surfaces (milestone M8)
// will satisfy these interfaces when that layer lands — the same shape the
// summon/owner/corpse seams in target.go follow.

// PartyMate reports whether this creature is in the same player party as
// other. Used by the party-area sweep.
type PartyMate interface {
	IsInSameParty(other Creature) bool
}

// ClanAllyMate reports whether this creature shares a clan or alliance with
// other. Used by the ally-area and corpse-ally sweeps.
type ClanAllyMate interface {
	IsInSameClan(other Creature) bool
	IsInSameAlly(other Creature) bool
}

// PartyState answers the party-membership checks the single-target party
// handlers gate on: whether the caster is in a party at all, and whether a
// specific other creature is a member of it.
type PartyState interface {
	IsInParty() bool
	PartyContains(other Creature) bool
}

// OlympiadParticipant is implemented by actors barred from alliance/corpse
// target sweeps while in the Olympiad.
type OlympiadParticipant interface {
	OlympiadMode() bool
}

// Dueler optionally exposes the duel id and team a creature belongs to, used
// by the ally-area sweeps to keep duel opponents out of allied buffs. A duel
// id of 0 means "not dueling"; only dueling casters restrict the sweep.
type Dueler interface {
	DuelID() int32
	DuelTeam() int
}

// ClanHolder reports whether the actor's clan is set, gating the corpse-ally
// sweep so a clanless caster can't resurrect allied strangers through it.
type ClanHolder interface {
	HasClan() bool
}

// MageClasser reports whether the actor's profession class is a mage, the
// skill-426/427 gate on TargetPartyOther.
type MageClasser interface {
	MageClass() bool
}

// ClanGroupMember exposes the social-group tags a monster template carries,
// used by TargetClan to match a casting mob against its fellow clan members.
type ClanGroupMember interface {
	ClanGroups() []string
}

// actingPlayerOf returns the player driving caster: the owner when caster is
// a player-owned summon, or caster itself otherwise. ok is false only when
// caster is nil.
func actingPlayerOf(caster Creature) (Creature, bool) {
	if caster == nil {
		return nil, false
	}
	if owner, ok := ownerOf(caster); ok {
		return owner, true
	}
	return caster, true
}

// isPlayerLike reports whether creature should be treated as a player for
// single-target gating. The closest available signal in this layer is a
// playable actor with no owner; the social layer (M8) owns the real player
// distinction.
func isPlayerLike(creature Creature) bool {
	if creature == nil || !creature.Category().Has(CategoryPlayable) {
		return false
	}
	_, owned := ownerOf(creature)
	return !owned
}

func inParty(creature Creature) bool {
	state, ok := any(creature).(PartyState)
	return ok && state.IsInParty()
}

func partyContains(partyMember, other Creature) bool {
	state, ok := any(partyMember).(PartyState)
	if !ok {
		return false
	}
	return state.PartyContains(other)
}

func sameParty(a, b Creature) bool {
	mate, ok := any(a).(PartyMate)
	if !ok {
		return false
	}
	return mate.IsInSameParty(b)
}

func sameClan(a, b Creature) bool {
	mate, ok := any(a).(ClanAllyMate)
	if !ok {
		return false
	}
	return mate.IsInSameClan(b)
}

func sameAlly(a, b Creature) bool {
	mate, ok := any(a).(ClanAllyMate)
	if !ok {
		return false
	}
	return mate.IsInSameAlly(b)
}

func olympiadMode(creature Creature) bool {
	o, ok := any(creature).(OlympiadParticipant)
	return ok && o.OlympiadMode()
}

func hasClan(creature Creature) bool {
	holder, ok := any(creature).(ClanHolder)
	return ok && holder.HasClan()
}

func mageClass(creature Creature) bool {
	m, ok := any(creature).(MageClasser)
	return ok && m.MageClass()
}

// sameDuelTeam reports whether a and b may be grouped together by an
// ally-targeting sweep. A non-dueling caster (DuelID == 0) sweeps everyone;
// otherwise both must be in the same duel id and on the same team.
func sameDuelTeam(a, b Creature) bool {
	caster, ok := any(a).(Dueler)
	if !ok || caster.DuelID() == 0 {
		return true
	}
	other, ok := any(b).(Dueler)
	if !ok {
		return false
	}
	return other.DuelID() == caster.DuelID() && other.DuelTeam() == caster.DuelTeam()
}

func clanGroupsOf(creature Creature) []string {
	member, ok := any(creature).(ClanGroupMember)
	if !ok {
		return nil
	}
	return member.ClanGroups()
}

// clanGroupsOverlap reports whether a and b share any clan-group tag. The
// reference uses an any-of-against-any-of check between the two tag lists,
// so empty tag lists never match.
func clanGroupsOverlap(a, b []string) bool {
	for _, x := range a {
		if x != "" && slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// allyPlayableAppends returns true when creature should be appended to an
// ally-area sweep: it bypasses the clan/ally/duel gate for the caster's own
// summon (which is always covered), otherwise requiring the membership and
// duel-team gates to pass.
func allyPlayableAppends(caster, summon, creature Creature) bool {
	if summon != nil && sameCreature(summon, creature) {
		return true
	}
	if !sameClan(caster, creature) && !sameAlly(caster, creature) {
		return false
	}
	return sameDuelTeam(caster, creature)
}

type partyHandler struct{ known Known }

func (partyHandler) Target() modelskill.Target { return modelskill.TargetParty }

// Targets returns the caster's acting player followed by every nearby alive
// playable in the same party, plus the acting player's own summon regardless
// of party membership. The sweep centers on the acting player, so a skill
// cast by a summon resolves its owning player as the sweep's anchor and list
// head, matching the reference's getActingPlayer indirection.
func (h partyHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	player, ok := actingPlayerOf(caster)
	if !ok {
		return []Creature{caster}
	}
	out := []Creature{player}
	if h.known == nil {
		return out
	}
	playerSummon, _ := summonOf(player)
	h.known.ForEachKnownCreatureInRadius(player, skillRadius(skill), func(creature Creature) {
		if creature.Dead() || !creature.Category().Has(CategoryPlayable) {
			return
		}
		if playerSummon != nil && sameCreature(playerSummon, creature) {
			out = append(out, creature)
			return
		}
		if !sameParty(player, creature) {
			return
		}
		out = append(out, creature)
	})
	return out
}

func (partyHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (partyHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool { return true }

type allyHandler struct{ known Known }

func (allyHandler) Target() modelskill.Target { return modelskill.TargetAlly }

// Targets returns the caster's acting player followed by every nearby alive
// playable sharing clan or alliance (and the same duel team when the acting
// player is dueling), plus the acting player's own summon. An Olympiad
// participant draws only the caster, since allies are unavailable there.
func (h allyHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	player, ok := actingPlayerOf(caster)
	if !ok {
		return []Creature{caster}
	}
	if olympiadMode(player) {
		return []Creature{caster}
	}
	out := []Creature{player}
	if h.known == nil {
		return out
	}
	playerSummon, _ := summonOf(player)
	h.known.ForEachKnownCreatureInRadius(player, skillRadius(skill), func(creature Creature) {
		if creature.Dead() || !creature.Category().Has(CategoryPlayable) {
			return
		}
		if allyPlayableAppends(player, playerSummon, creature) {
			out = append(out, creature)
		}
	})
	return out
}

func (allyHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (allyHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool { return true }

type clanHandler struct{ known Known }

func (clanHandler) Target() modelskill.Target { return modelskill.TargetClan }

// Targets returns the caster and every nearby Attackable monster sharing a
// clan-group tag. The reference handler only assembles a list for Attackable
// casters and otherwise returns an empty target array, so this returns nil
// for non-attackable casters.
func (h clanHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	if !caster.Category().Has(CategoryAttackable) || h.known == nil {
		return nil
	}
	baseGroups := clanGroupsOf(caster)
	out := []Creature{caster}
	h.known.ForEachKnownCreatureInRadius(caster, skillRadius(skill), func(creature Creature) {
		if sameCreature(caster, creature) || creature.Dead() || !creature.Category().Has(CategoryAttackable) {
			return
		}
		if !clanGroupsOverlap(baseGroups, clanGroupsOf(creature)) {
			return
		}
		out = append(out, creature)
	})
	return out
}

func (clanHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (clanHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool { return true }

type partyMemberHandler struct{}

func (partyMemberHandler) Target() modelskill.Target { return modelskill.TargetPartyMember }

func (partyMemberHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (partyMemberHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

// CanCast gates a single-target party-member skill. The Summon Friend skill
// (id 1403) only accepts a living player target; other skills also accept the
// caster's own summon and otherwise require a living playable. Every case
// then requires the target to be a member of the caster's party.
//
// The reference sends an "S1 cannot be used" system message on each failed
// branch; that network send belongs to the cast pipeline, not this layer.
func (partyMemberHandler) CanCast(caster, target Creature, skill *modelskill.Definition, _ bool) bool {
	if target == nil {
		return false
	}
	if sameCreature(caster, target) {
		return true
	}
	if skill != nil && skill.ID == summonFriendSkillID {
		if !isPlayerLike(target) || target.Dead() {
			return false
		}
	} else {
		if summon, ok := summonOf(caster); ok && sameCreature(summon, target) {
			return true
		}
		if !target.Category().Has(CategoryPlayable) || target.Dead() {
			return false
		}
	}
	return inParty(caster) && partyContains(caster, target)
}

type partyOtherHandler struct{}

func (partyOtherHandler) Target() modelskill.Target { return modelskill.TargetPartyOther }

func (partyOtherHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (partyOtherHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

// CanCast gates a single-target party-other skill: never on self, only on a
// living player, with the dual-class tracker skills (426 on a mage, 427 on a
// non-mage) rejected, and the target must be in the caster's party.
//
// As with the party-member handler, the reference's system-message sends on
// each failed branch belong to the cast pipeline and are not reproduced here.
func (partyOtherHandler) CanCast(caster, target Creature, skill *modelskill.Definition, _ bool) bool {
	if target == nil || sameCreature(caster, target) {
		return false
	}
	if !isPlayerLike(target) || target.Dead() {
		return false
	}
	if skill != nil {
		if skill.ID == dualcastManaSkillID && mageClass(target) {
			return false
		}
		if skill.ID == dualcastHealSkillID && !mageClass(target) {
			return false
		}
	}
	return inParty(caster) && partyContains(caster, target)
}

type corpseAllyHandler struct{ known Known }

func (corpseAllyHandler) Target() modelskill.Target { return modelskill.TargetCorpseAlly }

// Targets returns every nearby dead player in the caster's clan or alliance
// (and the same duel team when the caster is dueling). With no clan or no
// matching corpses it returns the caster alone, matching the reference's
// empty-list fallback to the caster.
func (h corpseAllyHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	player, ok := actingPlayerOf(caster)
	if !ok || !hasClan(player) || h.known == nil {
		return []Creature{caster}
	}
	var out []Creature
	h.known.ForEachKnownCreatureInRadius(player, skillRadius(skill), func(creature Creature) {
		if !creature.Dead() || !isPlayerLike(creature) {
			return
		}
		if !sameClan(player, creature) && !sameAlly(player, creature) {
			return
		}
		if !sameDuelTeam(player, creature) {
			return
		}
		out = append(out, creature)
	})
	if len(out) == 0 {
		return []Creature{caster}
	}
	return out
}

func (corpseAllyHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

// CanCast blocks corpse-ally skills while the caster participates in the
// Olympiad; the "skill unavailable during the Olympiad" send is the cast
// pipeline's responsibility.
func (corpseAllyHandler) CanCast(caster, _ Creature, _ *modelskill.Definition, _ bool) bool {
	player, ok := actingPlayerOf(caster)
	if !ok {
		return true
	}
	return !olympiadMode(player)
}

// summonFriendSkillID matches the Summon Friend skill that TargetPartyMember
// special-cases.
const summonFriendSkillID = 1403

// dualcastManaSkillID and dualcastHealSkillID are the two dual-class tracker
// skills TargetPartyOther special-cases: 426 may only target a non-mage, 427
// only a mage.
const (
	dualcastManaSkillID = 426
	dualcastHealSkillID = 427
)
