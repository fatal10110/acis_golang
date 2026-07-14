package skill

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const summonFriendConfirmTimeout = 30 * time.Second

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
	PartyMembers() []any
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
			if sameObject(cast.Caster, target) || !canBeSummoned(cast.Caster, target) {
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

func canBeSummoned(caster, target any) bool {
	if sameObject(caster, target) {
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

func teleportSummonedFriend(caster summonFriendCaster, target any, skill modelskill.Definition) {
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
