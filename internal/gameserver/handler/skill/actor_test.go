package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ---- from actor_test.go ----
// The handlers in this package reach every capability they need by asserting
// a cast participant into one of the focused interfaces below. Those
// assertions fail silently by design — a target that doesn't implement the
// surface is skipped rather than rejected — so a real actor that stops
// satisfying one of them disables a skill path without failing any test that
// uses a double. These assertions pin the real production actors against the
// surfaces they are expected to reach, so that regression is a build failure.
//
// Every participant surface embeds Actor, so this also pins the claim Actor
// rests on: the actors that report Dead() all report ObjectID() too.
var (
	_ Actor = (*player.Character)(nil)
	_ Actor = (*npc.Hostile)(nil)
	_ Actor = (*summon.Actor)(nil)
	_ Actor = (*npc.EffectPoint)(nil)

	// Effect-carrying targets: the destination of any effect-applying,
	// effect-cancelling, or continuous (buff/debuff/over-time) skill.
	_ effect.Actor = (*player.Character)(nil)
	_ effect.Actor = (*npc.Hostile)(nil)
	_ effect.Actor = (*summon.Actor)(nil)
	_ effect.Actor = (*npc.EffectPoint)(nil)

	// Damage and resource targets: PDAM/CHARGEDAM, MDAM/DEATHLINK, BLOW and
	// MANADAM all reach HP and MP through Creature, and the player-only
	// resources and notifications through Player.
	_ Creature = (*player.Character)(nil)
	_ Creature = (*npc.Hostile)(nil)
	_ Creature = (*summon.Actor)(nil)
	_ Player   = (*player.Character)(nil)
	_ NPC      = (*npc.Hostile)(nil)

	// DRAIN rolls a player target's cast break ahead of its effects and
	// HP loss.
	_ earlyCastBreaker = (*player.Character)(nil)

	// Caster-side surfaces resolved from Cast.Caster. cancelTarget above
	// and these three share a Level() int requirement that *player.Character
	// could not meet until its persisted level field was renamed off of
	// Level to make room for the method (see player.Character.CharLevel).
	_ magicCaster   = (*player.Character)(nil)
	_ sowCaster     = (*player.Character)(nil)
	_ harvestCaster = (*player.Character)(nil)

	// Signet: the radius scan hands each found object to the tick as an
	// Actor, and an anti-summon signet narrows that to a dismissable summon.
	_ signetUnsummonable = (*summon.Actor)(nil)

	// Erase: the servitor surface disableErase reaches through, and the
	// owner-facing notification it fires once erased.

	// SummonFriend/SummonParty: the caster-side gate, the target-side gate,
	// the pending teleport-request/confirm-summon surface, the required-item
	// check, and the teleport itself.
	_ summonFriendCaster       = (*player.Character)(nil)
	_ summonFriendRequester    = (*player.Character)(nil)
	_ summonFriendItemConsumer = (*player.Character)(nil)
	_ summonFriendTraveler     = (*player.Character)(nil)
)
