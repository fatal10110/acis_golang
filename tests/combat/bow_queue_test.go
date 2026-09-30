package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bowQueueSelfSkillID is a short self cast to request mid-shot.
const bowQueueSelfSkillID = 10

func bowQueueSelfSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: bowQueueSelfSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, ReuseDelay: 60_000, StaticReuse: true, SkillType: "DUMMY",
	}
}

// bowShooter is a player shooting the fixture monster with a bow. period is
// how long one shot plus the bow's reuse lasts, measured between the first
// two shots; shotAt is when the latest shot's Attack arrived.
type bowShooter struct {
	srv      *gameservertest.Server
	c        *scriptedClient
	objID    int32
	groundID int32
	period   time.Duration
	shotAt   time.Time
}

// bootBowShooter equips a bow and arrows, drops some adena at the player's
// feet, attacks the fixture monster, and returns once its second shot's
// Attack is out, with that shot in flight.
func bootBowShooter(t *testing.T) bowShooter {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{bowQueueSelfSkill()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a bow shot open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, bowQueueSelfSkillID, 1)
	bow := srv.GiveItem(t, objID, bowTemplateID, 1)
	arrows := srv.GiveItem(t, objID, woodenArrowTemplateID, bowArrowStack)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)
	equipAndFlush(t, srv, c, bow)
	equipAndFlush(t, srv, c, arrows)

	groundID := dropAdenaAtFeet(t, c, adena)

	x, y, z := int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAttackRequest(hostile.ObjectID(), x, y, z, false))
	readUntilSlow(t, c, serverpackets.OpcodeAttack, "first bow shot")
	first := c.Now()
	readUntilSlow(t, c, serverpackets.OpcodeAttack, "second bow shot")
	second := c.Now()
	return bowShooter{srv: srv, c: c, objID: objID, groundID: groundID, period: second.Sub(first), shotAt: second}
}

// dropAdenaAtFeet drops 40 of the adena stack adena at the player's
// origin and returns the ground item's object id.
func dropAdenaAtFeet(t *testing.T, c *scriptedClient, adena int32) int32 {
	t.Helper()
	return dropAdenaBehind(t, c, adena, 0)
}

// dropAdenaBehind drops 40 of the adena stack adena back units behind the
// player's origin, away from the fixture monster, and returns the ground
// item's object id.
func dropAdenaBehind(t *testing.T, c *scriptedClient, adena int32, back int) int32 {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(adena)
	w.WriteInt32(40)
	w.WriteInt32(int32(playerOrigin.X - back))
	w.WriteInt32(int32(playerOrigin.Y))
	w.WriteInt32(int32(playerOrigin.Z))
	c.Send(w.Bytes())
	drop := readUntil(t, c, serverpackets.OpcodeDropItem, "DropItem")[0]
	r := wireReader(drop[1:])
	r.ReadInt32() // dropper id
	return r.ReadInt32()
}

// readUntilSlow is readUntil for streams with gaps longer than its one
// second, such as a bow's reuse.
func readUntilSlow(t *testing.T, c *scriptedClient, want byte, what string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived", what)
		}
		if frame[0] == want {
			return
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
}

// assertBeforeReuseEnd fails when at is not inside the shot-plus-reuse
// period that began with the latest shot: what was queued behind the shot
// waited for the reuse.
func (b bowShooter) assertBeforeReuseEnd(t *testing.T, at time.Time, what string) {
	t.Helper()
	if waited := at.Sub(b.shotAt); waited >= b.period {
		t.Fatalf("%s arrived %v after the shot, want before the reuse ends (%v after the shot)", what, waited, b.period)
	}
}

// TestBowMidShotSkillStartsAtShotEnd pins a cast queued behind a bow shot
// starting when the shot ends, while the bow still reloads, not when the
// reuse is over.
func TestBowMidShotSkillStartsAtShotEnd(t *testing.T) {
	t.Parallel()
	b := bootBowShooter(t)

	b.c.Send(encodeRequestMagicSkillUse(bowQueueSelfSkillID, false, false))
	readUntilSlow(t, b.c, serverpackets.OpcodeActionFailed, "queued skill ActionFailed")
	readUntilSlow(t, b.c, serverpackets.OpcodeMagicSkillUse, "queued skill MagicSkillUse")
	b.assertBeforeReuseEnd(t, b.c.Now(), "queued skill MagicSkillUse")
}

// TestBowMidShotPickupRunsAtShotEnd is the same for a ground-item pickup
// clicked mid-shot: the item is picked up when the shot ends. The pickup
// replaced the attack (PlayableAI.onEvtFinishedAttack runs the next
// intention through doIntention), so once the reuse ends the bow does not
// fire again (PlayerAI.onEvtBowAttackReuse finds no ATTACK intention).
func TestBowMidShotPickupRunsAtShotEnd(t *testing.T) {
	t.Parallel()
	b := bootBowShooter(t)
	x, y, z := int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z)

	b.c.Send(encodeAction(b.groundID, x, y, z, false))
	readUntilSlow(t, b.c, serverpackets.OpcodeActionFailed, "queued pickup ActionFailed")
	readUntilSlow(t, b.c, serverpackets.OpcodeGetItem, "queued pickup GetItem")
	b.assertBeforeReuseEnd(t, b.c.Now(), "queued pickup GetItem")
	assertNoSwingBy(t, b.c, b.objID, 2*b.period, "after the queued pickup")
}
