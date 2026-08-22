package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// hostileX/Y/Z mirrors the fixture NPC spawn point the suite targets.
const (
	hostileX = 60
	hostileY = 20
	hostileZ = 30
)

// targetHostile clicks the fixture monster and consumes the click's reply
// sequence: the position validation, MyTargetSelected, and the monster's
// full-health StatusUpdate. It returns the monster's max HP as reported.
func targetHostile(t *testing.T, c *testsupport.ScriptedClient, hostileID int32) int {
	t.Helper()
	c.Send(encodeAction(hostileID, hostileX, hostileY, hostileZ, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeValidateLocation, "click ValidateLocation")
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	if got := wireReader(frame[1:]).ReadInt32(); got != hostileID {
		t.Fatalf("MyTargetSelected object id = %d, want %d", got, hostileID)
	}
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "target StatusUpdate")
	r := wireReader(frame[1:])
	if got := r.ReadInt32(); got != hostileID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, hostileID)
	}
	if count := r.ReadInt32(); count != 2 {
		t.Fatalf("StatusUpdate count = %d, want 2", count)
	}
	maxHP := 0
	for range 2 {
		typ, val := r.ReadInt32(), r.ReadInt32()
		switch typ {
		case int32(serverpackets.StatusMaxHP):
			maxHP = int(val)
		case int32(serverpackets.StatusCurrentHP):
			if int(val) != maxHP && maxHP != 0 {
				t.Fatalf("StatusUpdate CUR_HP = %d before any damage, want MAX_HP %d", val, maxHP)
			}
		default:
			t.Fatalf("StatusUpdate type = %d, want MAX_HP/CUR_HP", typ)
		}
	}
	return maxHP
}

// TestOffensiveSkillDrainsNPCHealth targets the fixture monster through the
// click flow and hits it with a physical skill: the cast reports the NPC as
// its target, and the monster's health drops. Exact damage numbers stay
// with the formula core tests; here only the drain is pinned.
func TestOffensiveSkillDrainsNPCHealth(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 42, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "PDAM", Power: 1_000_000,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)

	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(42, false, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, hostile.ObjectID())
	drainUntilQuiet(t, c)

	waitFor(t, "PDAM drain", func() bool { return hostile.CurrentHP() < maxHP })
	if hp := hostile.CurrentHP(); hp >= maxHP {
		t.Fatalf("monster HP after PDAM = %d, want drained below %d", hp, maxHP)
	}
}

// TestDamageOverTimeTicksDrainNPCHealth lands a damage-over-time debuff on
// the fixture monster and verifies each production effect sweep drains more
// health until the effect's count runs out.
func TestDamageOverTimeTicksDrainNPCHealth(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 43, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 0, StaticHitTime: true,
				SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
				BaseLandRate: 100, IgnoreResists: true,
				Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Value: 100, Count: 5, Time: 1}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 43, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(43, false, false))
	drainUntilQuiet(t, c)

	before := hostile.CurrentHP()
	time.Sleep(1100 * time.Millisecond)
	srv.TickEffects()
	afterFirst := hostile.CurrentHP()
	time.Sleep(1100 * time.Millisecond)
	srv.TickEffects()
	afterSecond := hostile.CurrentHP()

	if afterFirst >= before {
		t.Fatalf("monster HP after first DoT tick = %d, want drained below %d", afterFirst, before)
	}
	if afterSecond >= afterFirst {
		t.Fatalf("monster HP after second DoT tick = %d, want drained below %d", afterSecond, afterFirst)
	}
}
