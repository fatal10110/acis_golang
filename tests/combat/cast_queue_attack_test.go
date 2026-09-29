package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The mid-cast queue fixture: a long self cast to be mid-cast with and a
// second one to queue behind it, both long enough that the harness's reads,
// which move the driven clock while they wait, never reach their launch.
const (
	queueLongSkillID      = 3
	queueNextSkillID      = 4
	queueOneSkillID       = 5
	queueFollowUpSkillID  = 6
	queueHitTime          = 5000
	queueScrollTemplateID = 736  // Scroll of Escape: ItemSkills, item_skill 2013-1
	queueScrollSkillID    = 2013 // re-typed as a harmless self cast here
)

// queueScrollSkill is the skill the fixture scroll carries: a short self
// cast through the item AI-cast path.
func queueScrollSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: queueScrollSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, ReuseDelay: 5000, StaticReuse: true, SkillType: "DUMMY",
	}
}

// queueFollowUpSkill is a long offensive cast on the selected monster
// carrying nextActionAttack: with nothing queued behind it, its end attacks
// the monster. The fixture monster is parked and never swings back.
func queueFollowUpSkill() modelskill.Definition {
	def := queueCastSkill(queueFollowUpSkillID)
	def.Target = modelskill.TargetOne
	def.CastRange = 600
	def.Offensive = true
	def.NextActionIsAttack = true
	return def
}

func queueCastSkill(id int) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: queueHitTime, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
	}
}

// queueOneSkill is a ONE-target skill to queue on the selected monster.
func queueOneSkill() modelskill.Definition {
	def := queueCastSkill(queueOneSkillID)
	def.Target = modelskill.TargetOne
	def.CastRange = 600
	def.MPConsume = 5
	return def
}

// bootMidCastBesideHostile selects the fixture monster, then starts the long
// self cast, leaving the caster mid-cast with the monster still selected.
func bootMidCastBesideHostile(t *testing.T) (*gameservertest.Server, *player.Character, int32) {
	t.Helper()
	srv, pc, hostileID, _ := bootMidCastWith(t, queueLongSkillID)
	return srv, pc, hostileID
}

// bootMidCastWith selects the fixture monster, then starts the long cast
// longID, leaving the caster mid-cast with the monster still selected. It
// also returns the object ID of a fixture scroll whose skill runs through
// the item AI-cast path.
func bootMidCastWith(t *testing.T, longID int32) (*gameservertest.Server, *player.Character, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{
			queueCastSkill(queueLongSkillID), queueCastSkill(queueNextSkillID), queueOneSkill(), queueFollowUpSkill(), queueScrollSkill(),
		})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, queueLongSkillID, 1)
	seedKnownSkill(t, srv, objID, queueNextSkillID, 1)
	seedKnownSkill(t, srv, objID, queueOneSkillID, 1)
	seedKnownSkill(t, srv, objID, queueFollowUpSkillID, 1)
	scroll := srv.GiveItem(t, objID, queueScrollTemplateID, 3)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeRequestMagicSkillUse(longID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "long cast MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "long cast MagicSkillUse")
	drainUntilQuiet(t, c)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("long cast not in flight")
	}
	return srv, pc, hostile.ObjectID(), scroll
}

// requestAttackMidCast clicks the selected monster mid-cast: the attack
// waits for the cast and is answered with ActionFailed.
func requestAttackMidCast(t *testing.T, c *scriptedClient, hostileID int32) {
	t.Helper()
	c.Send(encodeAction(hostileID, hostileX, hostileY, hostileZ, false))
	assertFrameOpcode(t, mustRead(t, c, "mid-cast attack ActionFailed"), serverpackets.OpcodeActionFailed, "mid-cast attack ActionFailed")
}

// assertNoAttackFor reads frames while d passes and fails on any Attack.
func assertNoAttackFor(t *testing.T, c *scriptedClient, d time.Duration, what string) {
	t.Helper()
	for end := c.Now().Add(d); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame != nil && frame[0] == serverpackets.OpcodeAttack {
			t.Fatalf("%s: an attack started", what)
		}
	}
}

// TestMidCastSkillReplacesQueuedAttack pins the single next-intention slot
// from the attack side: an attack requested mid-cast, then a skill, leaves
// only the skill queued. It runs when the cast ends, and no attack follows
// once it has run too.
func TestMidCastSkillReplacesQueuedAttack(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	key := cast.ReuseKey(queueCastSkill(queueNextSkillID))

	requestAttackMidCast(t, c, hostileID)
	c.Send(encodeRequestMagicSkillUse(queueNextSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued skill ActionFailed"), serverpackets.OpcodeActionFailed, "queued skill ActionFailed")

	srv.AdvanceUntil(t, "queued skill start", func() bool { return pc.SkillDisabled(key) })
	srv.AdvanceUntil(t, "queued skill end", func() bool { return !pc.CastingNow() })
	assertNoAttackFor(t, c, 2*time.Second, "after the queued skill")
}

// TestMidCastAttackReplacesQueuedSkill is the other order: a skill queued
// mid-cast, then an attack, leaves only the attack, which starts when the
// cast ends; the skill never runs.
func TestMidCastAttackReplacesQueuedSkill(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	key := cast.ReuseKey(queueCastSkill(queueNextSkillID))

	c.Send(encodeRequestMagicSkillUse(queueNextSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued skill ActionFailed"), serverpackets.OpcodeActionFailed, "queued skill ActionFailed")
	requestAttackMidCast(t, c, hostileID)

	srv.AdvanceUntil(t, "cast end", func() bool { return !pc.CastingNow() })
	readUntil(t, c, serverpackets.OpcodeAttack, "queued attack")
	if pc.SkillDisabled(key) || pc.CastingNow() {
		t.Fatal("replaced queued skill ran")
	}
}

// TestQueuedSkillWhoseTargetLeftSendsNothing pins the resume's lost-target
// check: a ONE-target skill queued on the selected monster, which then
// leaves the world before the cast ends, is dropped with no packet at all —
// no ActionFailed, no reason message, no cast — and costs nothing.
func TestQueuedSkillWhoseTargetLeftSendsNothing(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	key := cast.ReuseKey(queueOneSkill())
	mpBefore := srv.PlayerCurrentMP(t, pc.ObjectID())

	c.Send(encodeRequestMagicSkillUse(queueOneSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued skill ActionFailed"), serverpackets.OpcodeActionFailed, "queued skill ActionFailed")

	hostile, ok := srv.State.Object(hostileID)
	if !ok {
		t.Fatal("fixture monster missing from world state")
	}
	srv.State.Despawn(hostile)
	drainUntilQuiet(t, c)
	if !pc.CastingNow() {
		t.Fatal("long cast ended before the queued skill's target left")
	}

	srv.AdvanceUntil(t, "cast end", func() bool { return !pc.CastingNow() })
	for end := c.Now().Add(2 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeMagicSkillUse:
			t.Fatalf("resume with its target gone sent opcode %#x, want no packet", frame[0])
		}
	}
	if pc.SkillDisabled(key) || pc.CastingNow() {
		t.Fatal("queued skill ran with its target gone")
	}
	if got := srv.PlayerCurrentMP(t, pc.ObjectID()); got != mpBefore {
		t.Fatalf("MP after the dropped queued skill = %d, want unchanged %d", got, mpBefore)
	}
}
