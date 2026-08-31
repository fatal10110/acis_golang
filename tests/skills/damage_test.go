package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
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

// TestResistedSkillReportsResistanceToCaster drives an MDAM cast whose
// effect-landing roll can never succeed (IgnoreResists returns BaseLandRate
// verbatim, and a zero rate never beats the roll) at another player: the
// damage itself lands, and the caster's own client receives the
// resisted-your-skill system message naming the target and the skill.
func TestResistedSkillReportsResistanceToCaster(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 44, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "MDAM", Power: 1_000_000,
				IgnoreResists: true, BaseLandRate: 0,
				Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Value: 100, Count: 5, Time: 1}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 44, 1)
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 1, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorldAmongPlayers(t, vc)
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	before := srv.PlayerCurrentHP(t, victim.ID)

	c.Send(encodeAction(victim.ID, hostileX, hostileY, hostileZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeValidateLocation, "click ValidateLocation")
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	if got := wireReader(frame[1:]).ReadInt32(); got != victim.ID {
		t.Fatalf("MyTargetSelected object id = %d, want %d", got, victim.ID)
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStatusUpdate, "target StatusUpdate")
	drainUntilQuiet(t, c)

	// An unflagged innocent is only attackable with force (ctrl), matching
	// the reference's isAttackableWithoutForce gate.
	c.Send(encodeRequestMagicSkillUse(44, true, false))
	readCastStartFrames(t, c, objID, 44, 1, 500, 60_000, victim.ID)

	waitFor(t, "MDAM damage on the victim", func() bool {
		return srv.PlayerCurrentHP(t, victim.ID) < before
	})
	drainUntilQuiet(t, vc)

	found := false
	for i := 0; i < 50 && !found; i++ {
		frame = c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if id := r.ReadInt32(); id != int32(serverpackets.SystemMessageS1ResistedYourS2) {
			continue
		}
		found = true
		if params := r.ReadInt32(); params != 2 {
			t.Fatalf("resisted message params = %d, want 2", params)
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText || r.ReadString() != "Victim" {
			t.Fatalf("resisted message first parameter = text Victim")
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName || r.ReadInt32() != 44 || r.ReadInt32() != 1 {
			t.Fatalf("resisted message second parameter = skill 44 level 1")
		}
	}
	if !found {
		t.Fatalf("resisted-your-skill message never arrived")
	}
	drainUntilQuiet(t, c)
}

// TestMagicDamageHalfFailureSendsAttackFailed forces the two independent
// magic-success rolls to fail then succeed so a same-level MDAM cast
// follows the half-damage branch: the caster receives ATTACK_FAILED and
// the monster still loses HP.
func TestMagicDamageHalfFailureSendsAttackFailed(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 45, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "MDAM", Power: 1_000_000,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 45, 1)
	startInWorld(t, c)

	worldObj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	caster, ok := worldObj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world player %d = %T, want SetRollSource", objID, worldObj)
	}
	magicRolls := 0
	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			magicRolls++
			if magicRolls == 1 {
				return 0
			}
			return 500
		}
		if n <= 0 {
			return 0
		}
		return n - 1
	})

	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(45, false, false))
	readCastStartFrames(t, c, objID, 45, 1, 500, 60_000, hostile.ObjectID())
	waitFor(t, "MDAM half-fail drain", func() bool { return hostile.CurrentHP() < maxHP })

	found := false
	for i := 0; i < 50 && !found; i++ {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		id := wireReader(frame[1:]).ReadInt32()
		if id == int32(serverpackets.SystemMessageAttackFailed) {
			found = true
		}
	}
	if !found {
		t.Fatalf("ATTACK_FAILED message never arrived")
	}
	drainUntilQuiet(t, c)
}

func signetMDamSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: 1419, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "SIGNET_CASTTIME", EffectNpcID: 13018, Radius: 180, MPConsume: 1,
		Power: 1_000_000, Magic: true,
		SelfEffects: []modelskill.EffectTemplate{{Name: "SignetMDam", Self: true, Count: 3, Time: 1}},
	}
}

func bootSignetMDamPair(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, *testsupport.ScriptedClient, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{ID: 13018, Type: "EffectPoint", CollisionRadius: 8}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{signetMDamSkill()})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1419, 1)
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 5, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorldAmongPlayers(t, vc)
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	return srv, c, vc, objID, victim.ID
}

func setCasterMagicRolls(t *testing.T, srv *gameservertest.Server, objID int32, roll10000 func() int) {
	t.Helper()
	worldObj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	caster, ok := worldObj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world player %d = %T, want SetRollSource", objID, worldObj)
	}
	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			return roll10000()
		}
		if n <= 0 {
			return 0
		}
		return n - 1
	})
}

func tickSignetMDamLive(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	for range 3 {
		time.Sleep(1100 * time.Millisecond)
		srv.TickEffects()
	}
}

func findSystemMessage(t *testing.T, c *testsupport.ScriptedClient, wantID int32) *wire.Reader {
	t.Helper()
	for range 50 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() == wantID {
			return r
		}
	}
	t.Fatalf("system message %d never arrived", wantID)
	return nil
}

// TestSignetMDamHalfFailureSendsResistMessages drives a SignetMDam tick
// against another player with a scripted fail-then-succeed magic-success
// pair: the caster gets ATTACK_FAILED, the target gets RESISTED_S1_MAGIC,
// and HP still drops by the failed amount.
func TestSignetMDamHalfFailureSendsResistMessages(t *testing.T) {
	srv, c, vc, objID, victimID := bootSignetMDamPair(t)
	magicRolls := 0
	setCasterMagicRolls(t, srv, objID, func() int {
		magicRolls++
		if magicRolls%2 == 1 {
			return 0
		}
		return 500
	})

	before := srv.PlayerCurrentHP(t, victimID)
	c.Send(encodeRequestMagicSkillUse(1419, false, false))
	readCastStartFrames(t, c, objID, 1419, 1, 500, 60_000, objID)
	tickSignetMDamLive(t, srv)
	if srv.PlayerCurrentHP(t, victimID) >= before {
		t.Fatalf("victim HP after signet tick = %d, want below %d", srv.PlayerCurrentHP(t, victimID), before)
	}

	findSystemMessage(t, c, int32(serverpackets.SystemMessageAttackFailed))
	r := findSystemMessage(t, vc, int32(serverpackets.SystemMessageResistedS1Magic))
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("RESISTED_S1_MAGIC params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText || r.ReadString() != "Mage" {
		t.Fatalf("RESISTED_S1_MAGIC parameter = text Mage")
	}
}

// TestSignetMDamFullFailureSendsResistedSkill drives the same tick with both
// magic-success rolls forced to fail: the caster gets S1_RESISTED_YOUR_S2
// naming the victim, and HP still drops.
func TestSignetMDamFullFailureSendsResistedSkill(t *testing.T) {
	srv, c, vc, objID, victimID := bootSignetMDamPair(t)
	setCasterMagicRolls(t, srv, objID, func() int { return 0 })

	before := srv.PlayerCurrentHP(t, victimID)
	c.Send(encodeRequestMagicSkillUse(1419, false, false))
	readCastStartFrames(t, c, objID, 1419, 1, 500, 60_000, objID)
	tickSignetMDamLive(t, srv)
	if srv.PlayerCurrentHP(t, victimID) >= before {
		t.Fatalf("victim HP after signet tick = %d, want below %d", srv.PlayerCurrentHP(t, victimID), before)
	}

	var found bool
	for range 50 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != int32(serverpackets.SystemMessageS1ResistedYourS2) {
			continue
		}
		if params := r.ReadInt32(); params != 2 {
			t.Fatalf("resisted message params = %d, want 2", params)
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText || r.ReadString() != "Victim" {
			continue
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName || r.ReadInt32() != 1419 || r.ReadInt32() != 1 {
			t.Fatalf("resisted message second parameter = skill 1419 level 1")
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("S1_RESISTED_YOUR_S2 naming Victim never arrived")
	}
	findSystemMessage(t, vc, int32(serverpackets.SystemMessageResistedS1Magic))
}
