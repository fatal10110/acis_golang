package skill

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const summonFriendConfirmTimeout = 30 * time.Second

// item is cast.Item, a genuinely heterogeneous payload forwarded untouched
// here (see Cast.Item's own doc comment for the other consumer); left
// untyped deliberately.
type creatureSummonRuntime interface {
	SummonCreature(skill modelskill.Definition, item any)
}

type summonCreatureHandler struct{}

func (summonCreatureHandler) Types() []string { return []string{"SUMMON_CREATURE"} }

func (summonCreatureHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(creatureSummonRuntime)
	if !ok {
		return
	}
	caster.SummonCreature(cast.Skill, cast.Item)
}

type summonFriendActorState interface {
	Mounted() bool
	OlympiadMode() bool
	ObserverMode() bool
	NoSummonFriendZone() bool
}

type summonFriendCaster interface {
	summonFriendActorState
	Position() (x, y, z int)
}

type summonFriendTargetState interface {
	summonFriendActorState
	AlikeDead() bool
	Operating() bool
	Rooted() bool
	InCombat() bool
	FestivalParticipant() bool
}

// summonFriendRequester is *player.Character's teleport-request/confirm-summon
// surface (see actor_test.go's compile-time assertion). caster stays untyped
// (any) rather than a Character-typed parameter: the plain
// SUMMON_FRIEND/SUMMON_PARTY path also calls TeleportRequest/ConfirmSummon
// with the caster forwarded opaquely, matching #1519/#1497's precedent for a
// value used only for identity/state, not behavior, at this layer.
type summonFriendRequester interface {
	TeleportRequest(caster any, skill modelskill.Definition) bool
	ClearTeleportRequest()
	ConfirmSummon(caster any, skill modelskill.Definition, timeout time.Duration)
}

type summonFriendTraveler interface {
	TeleportTo(x, y, z, radius int)
}

type summonFriendItemConsumer interface {
	ItemCount(itemID int) int
	ConsumeItem(itemID, count int) bool
}

type summonPartyProvider interface {
	PartyMembers() []creature.DeathActor
}

type summonFriendHandler struct{}

func (summonFriendHandler) Types() []string { return []string{"SUMMON_FRIEND", "SUMMON_PARTY"} }

func (summonFriendHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(summonFriendCaster)
	if !ok || !canSummonFriend(caster) {
		return
	}

	if skillTypeKey(cast.Skill.SkillType) == "SUMMON_PARTY" {
		party, ok := cast.Caster.(summonPartyProvider)
		if !ok {
			return
		}
		for _, target := range party.PartyMembers() {
			if !canBeSummoned(cast.Caster, target) {
				continue
			}
			teleportSummonedFriend(caster, target, cast.Skill)
		}
		return
	}

	for _, target := range cast.Targets {
		if !canBeSummoned(cast.Caster, target) {
			continue
		}
		requester, ok := target.(summonFriendRequester)
		if !ok || !requester.TeleportRequest(cast.Caster, cast.Skill) {
			continue
		}
		if cast.Skill.ID == 1403 {
			requester.ConfirmSummon(cast.Caster, cast.Skill, summonFriendConfirmTimeout)
			continue
		}
		teleportSummonedFriend(caster, target, cast.Skill)
		requester.ClearTeleportRequest()
	}
}

func canSummonFriend(actor summonFriendActorState) bool {
	return !actor.Mounted() && !actor.OlympiadMode() && !actor.ObserverMode() && !actor.NoSummonFriendZone()
}

// canBeSummoned takes target as creature.DeathActor because a SUMMON_PARTY
// cast walks the caster's own party list rather than a resolved target set;
// a party member that isn't actor-shaped fails the self-exclusion check the
// same way an unrelated value did before.
func canBeSummoned(caster Actor, target creature.DeathActor) bool {
	other, _ := target.(Actor)
	if sameObject(caster, other) {
		return false
	}
	state, ok := target.(summonFriendTargetState)
	if !ok {
		return false
	}
	if state.AlikeDead() || state.Operating() || state.Rooted() || state.InCombat() {
		return false
	}
	if state.OlympiadMode() || state.FestivalParticipant() || state.Mounted() {
		return false
	}
	return !state.ObserverMode() && !state.NoSummonFriendZone()
}

func teleportSummonedFriend(caster summonFriendCaster, target creature.DeathActor, skill modelskill.Definition) {
	if skill.TargetConsumeID > 0 && skill.TargetConsumeCount > 0 {
		consumer, ok := target.(summonFriendItemConsumer)
		if !ok || consumer.ItemCount(skill.TargetConsumeID) < skill.TargetConsumeCount {
			return
		}
		if !consumer.ConsumeItem(skill.TargetConsumeID, skill.TargetConsumeCount) {
			return
		}
	}
	traveler, ok := target.(summonFriendTraveler)
	if !ok {
		return
	}
	x, y, z := caster.Position()
	traveler.TeleportTo(x, y, z, 20)
}
