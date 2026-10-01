package combat

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// setPlayerRoll pins the live player's rolls to roll, on its own queue.
func setPlayerRoll(t *testing.T, srv *gameservertest.Server, objID int32, roll int) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return roll })
	}) {
		t.Fatal("player queue closed")
	}
	<-done
}

// TestMissedSwingsEnterNoAttackStance pins the attacker side of
// CreatureAttack.doHit (CreatureAttack.java:236-238): the attacker enters
// its stance only when a hit lands with damage, never at swing start. Swings
// that all miss broadcast no AutoAttackStart and leave restart open; the
// first landed hit starts the attacker's stance, and the monster it hit
// enters its own (AttackableAI.onEvtAttacked -> startAttackStance).
func TestMissedSwingsEnterNoAttackStance(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
	)
	if !srv.DrivesClock() {
		t.Skip("counting swings needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	setPlayerRoll(t, srv, objID, 999)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, objID)
	srv.Advance(t, 5*time.Second)
	frames := readQuiet(c)
	if n := countByID(frames, serverpackets.OpcodeAttack, objID); n < 2 {
		t.Fatalf("missed swings = %d, want at least 2 more after the first", n)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 0 {
		t.Fatalf("attacker AutoAttackStart after missed swings = %d, want 0", n)
	}
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("missed swings put the attacker in the stance tracker")
	}
	if got := hostile.CurrentHP(); got != hostile.MaxHP() {
		t.Fatalf("hostile HP after missed swings = %d, want full %d", got, hostile.MaxHP())
	}

	// A hit that lands starts both stances.
	setPlayerRoll(t, srv, objID, 0)
	srv.AdvanceUntil(t, "landed swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	frames = readQuiet(c)
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 1 {
		t.Fatalf("attacker AutoAttackStart after the landed hit = %d, want 1", n)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, hostile.ObjectID()); n != 1 {
		t.Fatalf("monster AutoAttackStart after the landed hit = %d, want 1", n)
	}
	if !srv.AttackStance.InAttackStance(worldActor{id: hostile.ObjectID()}) {
		t.Fatal("the hit monster is not in the stance tracker")
	}
	if !hostile.InCombat() {
		t.Fatal("the hit monster does not report combat for NpcInfo")
	}
}

// TestMissingPlayerCanRestart pins the exit side of the same rule: a player
// whose swings all missed holds no stance, so restart goes through once the
// swing loop is left.
func TestMissingPlayerCanRestart(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
	)
	if !srv.DrivesClock() {
		t.Skip("counting swings needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	setPlayerRoll(t, srv, objID, 999)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, objID)
	srv.Advance(t, 2*time.Second)
	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	drainUntilQuiet(t, c)
	srv.Advance(t, 2*time.Second)
	drainUntilQuiet(t, c)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	for {
		reply := mustRead(t, c, "RestartResponse")
		if reply[0] != serverpackets.OpcodeRestartResponse {
			continue
		}
		if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 1 {
			t.Fatalf("RestartResponse result = %d, want 1 (no stance after missed swings)", ok)
		}
		return
	}
}

const (
	stanceNukeID   = 42
	stanceBuffID   = 1068
	stanceLethalID = 43
)

func stanceCastSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: stanceNukeID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, CoolTime: 200, StaticHitTime: true, StaticReuse: true,
			SkillType: "PDAM", Power: 1, Offensive: true,
		},
		{
			ID: stanceLethalID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			SkillType: "PDAM", Power: 1_000_000, Offensive: true,
		},
		{
			ID: stanceBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, StaticHitTime: true, StaticReuse: true, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		},
	}
}

// TestOffensiveCastPutsCasterInAttackStance pins CreatureCast.onMagicFinalizer
// (CreatureCast.java:308-309): an offensive skill whose launch resolved a
// target enters the caster's stance when the cast finishes, after its
// launch; a buff never does.
func TestOffensiveCastPutsCasterInAttackStance(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
		gameservertest.WithSkills(combatPersistence(t, stanceCastSkills())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, stanceNukeID, 1)
	seedKnownSkill(t, srv, objID, stanceBuffID, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(stanceBuffID, false, false))
	srv.AdvanceUntil(t, "buff cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
	srv.Advance(t, time.Second)
	if n := countByID(readQuiet(c), serverpackets.OpcodeAutoAttackStart, objID); n != 0 {
		t.Fatalf("caster AutoAttackStart after a buff = %d, want 0", n)
	}
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("a buff put the caster in the stance tracker")
	}

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeRequestMagicSkillUse(stanceNukeID, false, false))
	srv.AdvanceUntil(t, "nuke cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
	frames := readQuiet(c)
	launched := indexOf(frames, 0, serverpackets.OpcodeMagicSkillLaunched, objID)
	start := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStart, objID)
	if launched < 0 || start < launched {
		t.Fatalf("caster AutoAttackStart at frame %d, MagicSkillLaunched at %d; want the stance after the launch", start, launched)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 1 {
		t.Fatalf("caster AutoAttackStart after the nuke = %d, want 1", n)
	}
	if !srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("the nuke left the caster out of the stance tracker")
	}
}

// TestKilledMonsterEndsItsStance pins CreatureAI.onEvtDead
// (CreatureAI.java:78-87): a monster that entered its stance when the skill
// reached it ends it as it dies, AutoAttackStop right after its Die.
func TestKilledMonsterEndsItsStance(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
		gameservertest.WithSkills(combatPersistence(t, stanceCastSkills())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, stanceLethalID, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeRequestMagicSkillUse(stanceLethalID, false, false))
	srv.AdvanceUntil(t, "monster death", hostile.Dead)
	srv.Settle(t)
	frames := readQuiet(c)
	start := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStart, hostile.ObjectID())
	die := indexOf(frames, 0, serverpackets.OpcodeDie, hostile.ObjectID())
	stop := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStop, hostile.ObjectID())
	if start < 0 || die < start || stop < die {
		t.Fatalf("monster frames: AutoAttackStart at %d, Die at %d, AutoAttackStop at %d; want them in that order", start, die, stop)
	}
	if srv.AttackStance.InAttackStance(worldActor{id: hostile.ObjectID()}) || hostile.InCombat() {
		t.Fatal("dead monster still in its stance")
	}
}

// TestMonsterStanceEndsFifteenSecondsAfterItsLastHit pins the NPC attacker's
// stance (AbstractAI.startAttackStance, AttackStanceTaskManager): its first
// landed hit broadcasts its AutoAttackStart once, a later hit only refreshes
// it, and AutoAttackStop follows 15s after the last hit.
func TestMonsterStanceEndsFifteenSecondsAfterItsLastHit(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	monsterID := attacker.ObjectID()
	drainUntilQuiet(t, c)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	frames := readQuiet(c)
	swing := indexOf(frames, 0, serverpackets.OpcodeAttack, monsterID)
	start := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStart, monsterID)
	if swing < 0 || start < swing {
		t.Fatalf("monster AutoAttackStart at frame %d, Attack at %d; want the stance at the landed hit", start, swing)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, monsterID); n != 1 {
		t.Fatalf("monster AutoAttackStart after one hit = %d, want 1", n)
	}

	nowMS.Add(10_000)
	srv.AddPlayerHP(t, objID, 1000)
	attacker.DoAttack(t, victim.(attackable.Combatant))
	if n := countByID(readQuiet(c), serverpackets.OpcodeAutoAttackStart, monsterID); n != 0 {
		t.Fatalf("monster AutoAttackStart after a hit inside its stance = %d, want 0", n)
	}

	nowMS.Add(10_000)
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	if n := countByID(readQuiet(c), serverpackets.OpcodeAutoAttackStop, monsterID); n != 0 {
		t.Fatal("monster stance expired 10s after its last hit, want it held for 15s")
	}

	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	srv.Settle(t)
	if n := countByID(readQuiet(c), serverpackets.OpcodeAutoAttackStop, monsterID); n != 1 {
		t.Fatalf("monster AutoAttackStop after its stance expired = %d, want 1", n)
	}
	if srv.AttackStance.InAttackStance(worldActor{id: monsterID}) || attacker.InCombat() {
		t.Fatal("expired monster stance still tracked")
	}
}

// sitDown seats the client's player and waits out the sit-down, so the
// player counts as sitting (Player.isSitting) rather than sitting down.
func sitDown(t *testing.T, srv *gameservertest.Server, c *scriptedClient, objID int32) {
	t.Helper()
	c.Send(encodeRequestChangeWaitType(false))
	assertFrameOpcode(t, mustRead(t, c, "sit ChangeWaitType"), serverpackets.OpcodeChangeWaitType, "sit ChangeWaitType")
	srv.AdvanceUntil(t, "seated", func() bool { return onlinePlayer(t, srv, objID).Seated() })
	drainUntilQuiet(t, c)
}

// standingWaitTypes counts the ChangeWaitType frames for id that stand it
// up.
func standingWaitTypes(frames [][]byte, id int32) int {
	n := 0
	for i := indexOf(frames, 0, serverpackets.OpcodeChangeWaitType, id); i >= 0; i = indexOf(frames, i+1, serverpackets.OpcodeChangeWaitType, id) {
		r := wireReader(frames[i][5:])
		if r.ReadInt32() == int32(serverpackets.WaitStanding) {
			n++
		}
	}
	return n
}

// TestInvulnerableSeatedPlayerStandsWhenHit pins PlayerAI.onEvtAttacked
// (PlayerAI.java:148-158): a landed hit on a seated player runs the stand
// intention, even when the player is invulnerable and the damage itself
// stands nobody up.
func TestInvulnerableSeatedPlayerStandsWhenHit(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	sitDown(t, srv, c, objID)
	victim.SetInvul(true)
	beforeHP := srv.PlayerCurrentHP(t, objID)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	srv.Settle(t)
	frames := readQuiet(c)
	if n := standingWaitTypes(frames, objID); n != 1 {
		t.Fatalf("standing ChangeWaitType after the hit = %d, want 1", n)
	}
	if !onlinePlayer(t, srv, objID).Standing() {
		t.Fatal("invulnerable player still seated after the hit")
	}
	if got := srv.PlayerCurrentHP(t, objID); got != beforeHP {
		t.Fatalf("invulnerable player HP = %d, want unchanged %d", got, beforeHP)
	}
}

// TestSeatedStoringPlayerRefusesToStandWhenHit pins thinkStand's rejection
// (PlayerAI.java:490-504): a storing player's AI is denied, so the stand
// intention answers ActionFailed and the player stays seated.
func TestSeatedStoringPlayerRefusesToStandWhenHit(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	sitDown(t, srv, c, objID)
	srv.SetPlayerOperating(t, objID, true)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	srv.Settle(t)
	frames := readQuiet(c)
	if n := standingWaitTypes(frames, objID); n != 0 {
		t.Fatalf("standing ChangeWaitType for a storing player = %d, want 0", n)
	}
	if indexOf(frames, 0, serverpackets.OpcodeActionFailed, -1) < 0 {
		t.Fatal("storing player's refused stand sent no ActionFailed")
	}
	if !onlinePlayer(t, srv, objID).Seated() {
		t.Fatal("storing player stood up after the hit")
	}
}

// TestSeatedSleepingPlayerRefusesStandBeforeHitStandsIt pins the order of a
// hit on a seated, sleeping player: ATTACKED fires before the damage
// (CreatureAttack.java:240 then :263), so thinkStand's rejection
// (PlayerAI.java:490-504) answers ActionFailed first; the damage then
// stops the sleep and stands the player up (PlayerStatus.java:118-125).
func TestSeatedSleepingPlayerRefusesStandBeforeHitStandsIt(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	sitDown(t, srv, c, objID)
	obj, _ := srv.State.Player(objID)
	landEffect(t, obj.(effectHolder), "Sleep")
	drainUntilQuiet(t, c)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	srv.Settle(t)
	frames := readQuiet(c)
	if n := standingWaitTypes(frames, objID); n != 1 {
		t.Fatalf("standing ChangeWaitType after the hit = %d, want 1", n)
	}
	refused := indexOf(frames, 0, serverpackets.OpcodeActionFailed, -1)
	stood := indexOf(frames, 0, serverpackets.OpcodeChangeWaitType, objID)
	if refused < 0 || refused > stood {
		t.Fatalf("ActionFailed at %d, standing ChangeWaitType at %d, want the refused stand first (opcodes %v)", refused, stood, opcodes(frames))
	}
}
