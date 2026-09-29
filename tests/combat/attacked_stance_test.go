package combat

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// countByID counts the frames with opcode op whose leading object id is id.
func countByID(frames [][]byte, op byte, id int32) int {
	n := 0
	for i := indexOf(frames, 0, op, id); i >= 0; i = indexOf(frames, i+1, op, id) {
		n++
	}
	return n
}

// onHostileQueue runs fn on hostile's own queue and waits for it.
func onHostileQueue(t *testing.T, hostile *npc.Hostile, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !hostile.Queue().Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("hostile queue closed")
	}
	<-done
}

// TestMonsterHitPutsPlayerInAttackStance pins the ATTACKED event for a
// player target (CreatureAttack.java:240 -> CreatureAI.onEvtAttacked ->
// PlayerAI.startAttackStance): the first landed hit broadcasts the player's
// AutoAttackStart, before the hit's HP update, and every later hit only
// refreshes the stance's 15s inactivity window.
func TestMonsterHitPutsPlayerInAttackStance(t *testing.T) {
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
	drainUntilQuiet(t, c)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	frames := readQuiet(c)
	start := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStart, objID)
	if start < 0 {
		t.Fatal("hit player never got its own AutoAttackStart")
	}
	if status := indexOf(frames, 0, serverpackets.OpcodeStatusUpdate, objID); status >= 0 && status < start {
		t.Fatalf("player StatusUpdate at frame %d before its AutoAttackStart at %d, want the stance first", status, start)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 1 {
		t.Fatalf("player AutoAttackStart count after one hit = %d, want 1", n)
	}

	// A second hit 10s later refreshes the stance without a new broadcast.
	nowMS.Add(10_000)
	srv.AddPlayerHP(t, objID, 1000)
	attacker.DoAttack(t, victim.(attackable.Combatant))
	frames = readQuiet(c)
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 0 {
		t.Fatalf("player AutoAttackStart count after a hit inside the stance = %d, want 0", n)
	}

	// 20s after the first hit, 10s after the second: still in stance.
	nowMS.Add(10_000)
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	if n := countByID(readQuiet(c), serverpackets.OpcodeAutoAttackStop, objID); n != 0 {
		t.Fatal("stance expired 10s after the refreshing hit, want it held for 15s")
	}
	if !srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("player left the stance tracker 10s after the refreshing hit")
	}

	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	stop := readSkippingCombat(t, c, serverpackets.OpcodeAutoAttackStop, "stance expiry")
	if got := wireReader(stop[1:]).ReadInt32(); got != objID {
		t.Fatalf("AutoAttackStop object id = %d, want %d", got, objID)
	}
}

// TestMonsterMissTellsPlayerItAvoidedTheAttack pins CreatureAttack.doHit's
// miss branch for a player target (CreatureAttack.java:226-230): the player
// reads AVOIDED_S1_ATTACK naming the attacker, and a miss is no ATTACKED
// event, so it enters no stance.
func TestMonsterMissTellsPlayerItAvoidedTheAttack(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	victim := livePlayer(t, srv, objID)
	tmpl := gameservertest.AttackingHostileTemplate()
	tmpl.Name = "Grim Wolf"
	attacker := srv.SpawnAttackingHostileNPCTemplate(t, tmpl, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	onHostileQueue(t, attacker.Hostile, func() { attacker.SetRollSource(func(int) int { return 999 }) })
	drainUntilQuiet(t, c)
	beforeHP := srv.PlayerCurrentHP(t, objID)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	frames := readQuiet(c)
	swing := indexOf(frames, 0, serverpackets.OpcodeAttack, attacker.ObjectID())
	if swing < 0 {
		t.Fatal("monster Attack frame missing")
	}
	r := wireReader(frames[swing][1:])
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	if flags := r.ReadUint8(); flags&attack.HitMiss == 0 {
		t.Fatalf("Attack flags = %#x, want a miss", flags)
	}
	var avoided []byte
	for _, f := range frames[swing:] {
		if f[0] == serverpackets.OpcodeSystemMessage && wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageAvoidedS1Attack {
			if avoided != nil {
				t.Fatal("AVOIDED_S1_ATTACK sent twice for one missed hit")
			}
			avoided = f
		}
	}
	if avoided == nil {
		t.Fatal("player never read AVOIDED_S1_ATTACK for the missed hit")
	}
	assertSystemMessageString(t, avoided, serverpackets.SystemMessageAvoidedS1Attack, "Grim Wolf")
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 0 {
		t.Fatalf("player AutoAttackStart after a miss = %d, want 0", n)
	}
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("a missed hit put the player in the stance tracker")
	}
	if got := srv.PlayerCurrentHP(t, objID); got != beforeHP {
		t.Fatalf("player HP after a miss = %d, want %d", got, beforeHP)
	}
}

// TestPlayerHitPutsVictimInAttackStance drives the PvP leg: a landed hit
// shows the victim's own AutoAttackStart to the victim once; a missed hit
// tells the victim AVOIDED_S1_ATTACK with the attacker's name and no stance.
func TestPlayerHitPutsVictimInAttackStance(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		roll int
		hit  bool
	}{
		{name: "landed hit", roll: 0, hit: true},
		{name: "miss", roll: 999},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Attacker", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, attackerID := srv.Client, srv.SoleObjectID(t)
			victim := srv.SeedCharacterFor(t, "victim", "Victim", 5, 0)
			vc := srv.DialClient(t, "victim", 1)
			startInWorld(t, vc)
			startInWorld(t, c)
			drainUntilQuiet(t, vc)
			drainUntilQuiet(t, c)

			obj, ok := srv.State.Player(attackerID)
			if !ok {
				t.Fatal("attacker missing from world state")
			}
			done := make(chan struct{})
			if !srv.PlayerQueue(t, attackerID).Post(func() {
				defer close(done)
				obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return tt.roll })
			}) {
				t.Fatal("attacker queue closed")
			}
			<-done

			selectPlayerTarget(t, c, victim.ID)
			c.Send(encodeAttackRequest(victim.ID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			var feedback []byte
			for feedback == nil {
				f := mustRead(t, c, "attacker hit feedback")
				if f[0] != serverpackets.OpcodeSystemMessage {
					continue
				}
				switch wireReader(f[1:]).ReadInt32() {
				case serverpackets.SystemMessageMissedTarget, serverpackets.SystemMessageYouDidS1Dmg:
					feedback = f
				}
			}
			// Stop swinging, so the victim's reads cover exactly one swing.
			c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
			srv.Settle(t)

			frames := readQuiet(vc)
			starts := countByID(frames, serverpackets.OpcodeAutoAttackStart, victim.ID)
			avoided := 0
			for _, f := range frames {
				if f[0] != serverpackets.OpcodeSystemMessage || wireReader(f[1:]).ReadInt32() != serverpackets.SystemMessageAvoidedS1Attack {
					continue
				}
				assertSystemMessageString(t, f, serverpackets.SystemMessageAvoidedS1Attack, "Attacker")
				avoided++
			}
			if tt.hit {
				if starts != 1 || avoided != 0 {
					t.Fatalf("victim after a landed hit: own AutoAttackStart = %d, AVOIDED_S1_ATTACK = %d; want 1, 0", starts, avoided)
				}
				return
			}
			if starts != 0 || avoided != 1 {
				t.Fatalf("victim after a miss: own AutoAttackStart = %d, AVOIDED_S1_ATTACK = %d; want 0, 1", starts, avoided)
			}
		})
	}
}

// monsterNukeSkill is a casting monster's quick physical skill strike.
const monsterNukeSkill = modelskill.ID(9105)

// TestMonsterSkillPutsPlayerInAttackStance pins the offensive-skill leg of
// ATTACKED (CreatureCast.java:480-490): a monster's damage skill landing on
// a player shows the player's AutoAttackStart.
func TestMonsterSkillPutsPlayerInAttackStance(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	victim := livePlayer(t, srv, objID)
	caster, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000, PAtk: 1,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{{
		ID: monsterNukeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		SkillType: "PDAM", Power: 1,
	}}))
	// The PvP flag keeps the monster's launch-time target re-check from
	// dropping the unflagged player (#2685); the reference resolves an NPC's
	// ONE target list with no conditions.
	victim.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	drainUntilQuiet(t, c)

	onHostileQueue(t, caster, func() {
		aiCtl.Cast(victim.(attackable.Combatant), modelskill.Ref{ID: monsterNukeSkill, Level: 1})
	})
	srv.AdvanceUntil(t, "monster skill landing", func() bool { return srv.AttackStance.InAttackStance(worldActor{id: objID}) })
	if n := countByID(readQuiet(c), serverpackets.OpcodeAutoAttackStart, objID); n != 1 {
		t.Fatalf("player AutoAttackStart after the monster's skill = %d, want 1", n)
	}
}

// assertSystemMessageString asserts a SystemMessage whose only parameter is
// one text.
func assertSystemMessageString(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wireReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("SystemMessage param type = %d, want text", typ)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("SystemMessage text = %q, want %q", got, text)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}
