package skill

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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

func (summonCreatureHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeSummonCreature}
}

func (summonCreatureHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(creatureSummonRuntime)
	if !ok {
		return
	}
	caster.SummonCreature(cast.Skill, cast.Item)
}

type summonFriendCaster interface {
	player.SummonFriendRequester
}

// summonFriendTarget is *player.Character's summoned side (see actor_test.go's
// compile-time assertion): the request a confirm-summon skill leaves pending
// and the teleport toward the caster.
type summonFriendTarget interface {
	player.SummonFriendRequester
	TeleportRequest(caster player.SummonFriendRequester, skill modelskill.Definition) bool
	ClearTeleportRequest()
	ConfirmSummon(caster player.SummonFriendRequester, skill modelskill.Definition, timeout time.Duration)
	SummonTo(caster player.SummonFriendRequester, skill modelskill.Definition) player.SummonRefusal
}

// summonPartyProvider is *player.Character's party roster, the caster
// itself included.
type summonPartyProvider interface {
	PartyMembers() []*player.Character
}

// SummonFriendRefused tells the player RecipientID why a summon-friend skill
// stopped: the caster for its own gates and a refused target, a summoned
// player for its side of the teleport.
type SummonFriendRefused struct {
	RecipientID int32
	Refusal     player.SummonRefusal
}

type summonFriendHandler struct{}

func (summonFriendHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeSummonFriend, modelskill.SkillTypeSummonParty}
}

func (summonFriendHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(summonFriendCaster)
	if !ok {
		return
	}
	if r := player.SummonerRefusal(caster); r.Refused() {
		cast.tellSummonRefusal(caster.ObjectID(), r)
		return
	}

	if cast.Skill.SkillType.Normalized() == modelskill.SkillTypeSummonParty {
		party, ok := cast.Caster.(summonPartyProvider)
		if !ok {
			return
		}
		for _, member := range party.PartyMembers() {
			if member.ObjectID() == caster.ObjectID() {
				continue
			}
			target, r := summonedTarget(caster, member)
			if r.Refused() {
				cast.tellSummonRefusal(caster.ObjectID(), r)
				continue
			}
			cast.tellSummonRefusal(target.ObjectID(), target.SummonTo(caster, cast.Skill))
		}
		return
	}

	for _, t := range cast.Targets {
		target, r := summonedTarget(caster, t)
		if r.Refused() {
			cast.tellSummonRefusal(caster.ObjectID(), r)
			continue
		}
		if !target.TeleportRequest(caster, cast.Skill) {
			cast.tellSummonRefusal(caster.ObjectID(), player.SummonRefusal{Reason: player.SummonRefusedPending, Name: target.CharacterName()})
			continue
		}
		if cast.Skill.ID == 1403 {
			target.ConfirmSummon(caster, cast.Skill, summonFriendConfirmTimeout)
			continue
		}
		cast.tellSummonRefusal(target.ObjectID(), target.SummonTo(caster, cast.Skill))
		target.ClearTeleportRequest()
	}
}

// SummonFriendCastRefusal is the refusal a player's SUMMON_FRIEND cast gets
// before it starts, told to caster: the summoner gate, then the summoned gate
// on the cast's target.
func SummonFriendCastRefusal(caster player.SummonFriendRequester, target Actor) player.SummonRefusal {
	if r := player.SummonerRefusal(caster); r.Refused() {
		return r
	}
	_, r := summonedTarget(caster, target)
	return r
}

// summonedTarget resolves a as a player caster may summon; any other actor
// is refused silently.
func summonedTarget(caster player.SummonFriendRequester, a Actor) (summonFriendTarget, player.SummonRefusal) {
	if _, ok := asPlayer(a); !ok {
		return nil, player.SummonRefusal{Reason: player.SummonRefusedSilently}
	}
	target, ok := a.(summonFriendTarget)
	if !ok {
		return nil, player.SummonRefusal{Reason: player.SummonRefusedSilently}
	}
	return target, player.SummonedRefusal(caster, target)
}

// tellSummonRefusal records r for the player recipient; an allowed summon or
// a silent refusal tells nothing.
func (c Cast) tellSummonRefusal(recipient int32, r player.SummonRefusal) {
	if r.Reason == player.SummonAllowed || r.Reason == player.SummonRefusedSilently {
		return
	}
	c.record(SummonFriendRefused{RecipientID: recipient, Refusal: r})
}
