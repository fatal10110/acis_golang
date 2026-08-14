package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

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
	_ effectListTarget = (*player.Character)(nil)
	_ effectListTarget = (*npc.Hostile)(nil)
	_ effectListTarget = (*summon.Actor)(nil)
	_ effectListTarget = (*npc.EffectPoint)(nil)
	_ continuousTarget = (*player.Character)(nil)
	_ continuousTarget = (*npc.Hostile)(nil)
	_ continuousTarget = (*summon.Actor)(nil)
	_ disablerTarget   = (*player.Character)(nil)
	_ disablerTarget   = (*npc.Hostile)(nil)
	_ disablerTarget   = (*summon.Actor)(nil)
	_ cancelTarget     = (*player.Character)(nil)

	// Damage targets: PDAM/CHARGEDAM, MDAM/DEATHLINK, BLOW, and MANADAM
	// each narrow to one of these before touching HP or MP.
	_ hpDamageTarget      = (*player.Character)(nil)
	_ hpDamageTarget      = (*npc.Hostile)(nil)
	_ hpDamageTarget      = (*summon.Actor)(nil)
	_ physicalSkillTarget = (*player.Character)(nil)
	_ physicalSkillTarget = (*npc.Hostile)(nil)
	_ physicalSkillTarget = (*summon.Actor)(nil)
	_ magicDamageTarget   = (*player.Character)(nil)
	_ magicDamageTarget   = (*npc.Hostile)(nil)
	_ magicDamageTarget   = (*summon.Actor)(nil)
	_ blowDamageTarget    = (*player.Character)(nil)
	_ blowDamageTarget    = (*npc.Hostile)(nil)
	_ manaDamageTarget    = (*player.Character)(nil)
	_ manaDamageTarget    = (*npc.Hostile)(nil)

	// Caster-side surfaces resolved from Cast.Caster. cancelTarget above
	// and these three share a Level() int requirement that *player.Character
	// could not meet until its persisted level field was renamed off of
	// Level to make room for the method (see player.Character.CharLevel).
	_ magicCaster   = (*player.Character)(nil)
	_ sowCaster     = (*player.Character)(nil)
	_ harvestCaster = (*player.Character)(nil)

	// Signet: the radius scan hands each found object to the tick as an
	// Actor, and an anti-summon signet narrows that to a dismissable summon.
	_ signetCastTarget    = (*player.Character)(nil)
	_ signetCastTarget    = (*npc.Hostile)(nil)
	_ signetUnsummonable  = (*summon.Actor)(nil)
	_ spoilableTarget     = (*npc.Hostile)(nil)
	_ effectSuccessSource = (*npc.Hostile)(nil)
)

// fakeActor supplies the Actor surface every cast participant carries, so a
// test double only has to spell out the capability the case under test
// actually exercises. Prefer a real actor (see newDisablerHostile) when the
// case is about the behavior rather than about one narrow surface; a double
// that models death or identity itself declares its own Dead or ObjectID,
// which shadows the one embedded here.
type fakeActor struct {
	objectID int32
}

func (f fakeActor) ObjectID() int32 { return f.objectID }

func (fakeActor) Dead() bool { return false }
