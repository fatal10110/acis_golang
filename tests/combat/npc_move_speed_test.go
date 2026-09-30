package combat

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// speedTestTemplate is a moving monster whose DEX 30 gives the RUN_SPEED
// stat its 1.1 bonus: it walks at 66 and runs at 132 units per second.
func speedTestTemplate() *npc.Template {
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.DEX = 30
	return tmpl
}

const (
	speedTestWalk = 66.0
	speedTestRun  = 132.0
)

// coveredInOneSecond lets one second pass in position-update ticks and
// returns how far the server moved hostile.
func coveredInOneSecond(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile) float64 {
	t.Helper()
	from := hostile.Move().Position()
	for range 10 {
		srv.Advance(t, 100*time.Millisecond)
		srv.TickPositions()
	}
	return from.Distance2D(hostile.Move().Position())
}

// npcInfoSpeeds reads frames until the NpcInfo for objectID and returns
// the base run and walk speeds each scaled by its movement speed
// multiplier: the paces the client animates for the two stances.
func npcInfoSpeeds(t *testing.T, c *scriptedClient, objectID int32) (run, walk float64) {
	t.Helper()
	for range 100 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatalf("NpcInfo for %d never arrived", objectID)
		}
		if frame[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		for range 9 { // template, attackable, x, y, z, heading, 0, cast and attack speed
			r.ReadInt32()
		}
		baseRun, baseWalk := r.ReadInt32(), r.ReadInt32()
		for range 6 {
			r.ReadInt32()
		}
		multiplier := r.ReadFloat64()
		if err := r.Err(); err != nil {
			t.Fatalf("read NpcInfo: %v", err)
		}
		return float64(baseRun) * multiplier, float64(baseWalk) * multiplier
	}
	t.Fatalf("NpcInfo for %d not found within 100 frames", objectID)
	return 0, 0
}

func requireClientPace(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("%s: client animates %.2f per second, want %.1f", what, got, want)
	}
}

func requireCovered(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 2 {
		t.Fatalf("%s: moved %.1f in one second, want %.1f", what, got, want)
	}
}

// TestWalkingNPCMovesAtWalkSpeed pins the server-side pace of an NPC in walk
// stance: CreatureMove.updatePosition advances getMoveSpeed()/10 per tick,
// and the move speed is RUN_SPEED over the base the stance picks
// (CreatureStatus.getBaseMoveSpeed/getMoveSpeed). A monster walking home
// covers its walk speed per second, and switching it to run mid-leg speeds
// the same leg up to its run speed. The NpcInfo the client got carries the
// base speeds and the movement speed multiplier
// (AbstractNpcInfo.NpcInfo: getMovementSpeedMultiplier), and scaled by it
// both stances match the server's pace.
func TestWalkingNPCMovesAtWalkSpeed(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	away := location.Location{X: hostileX, Y: hostileY + 1000, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, speedTestTemplate(), home, away)
	clientRun, clientWalk := npcInfoSpeeds(t, c, hostile.ObjectID())
	requireClientPace(t, "walk", clientWalk, speedTestWalk)
	requireClientPace(t, "run", clientRun, speedTestRun)
	drainUntilQuiet(t, c)

	onHostileQueue(t, hostile, func() {
		if !hostile.ReturnHome() {
			t.Error("ReturnHome() = false, want a walk home from outside the drift range")
		}
	})
	if hostile.Running() || !hostile.Move().Moving() {
		t.Fatalf("Running, Moving = %v, %v after ReturnHome; want a walk", hostile.Running(), hostile.Move().Moving())
	}
	requireCovered(t, "walking home", coveredInOneSecond(t, srv, hostile), speedTestWalk)

	onHostileQueue(t, hostile, hostile.ForceRunStance)
	requireCovered(t, "same leg after switching to run", coveredInOneSecond(t, srv, hostile), speedTestRun)
}

// TestRunSpeedDebuffSlowsChasingNPC lands a RUN_SPEED debuff on a monster
// chasing a player. The debuff's stat funcs change getMoveSpeed, which every
// position update reads, so the server-side approach slows at once and
// regains its pace when the debuff is removed. Each change resends NpcInfo,
// whose multiplier-scaled run speed matches the server's pace.
func TestRunSpeedDebuffSlowsChasingNPC(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	player := liveCombatant(t, srv)

	px, py, pz := player.Position()
	at := location.Location{X: px + 1500, Y: py, Z: pz}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, speedTestTemplate(), at, at)
	drainUntilQuiet(t, c)

	hostile.AddCombatDamageHate(player, 50)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionAttack)
	}
	requireCovered(t, "chasing", coveredInOneSecond(t, srv, hostile), speedTestRun)

	slow, err := effect.New(effect.Skill{ID: 102, Level: 1, Debuff: true}, modelskill.EffectTemplate{
		Name: "Debuff", Time: 30, StackType: "speed_down", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "runSpd", Value: 0.5}},
	})
	if err != nil {
		t.Fatalf("effect.New(slow): %v", err)
	}
	slow.Effector, slow.Effected = hostile, hostile
	drainUntilQuiet(t, c)
	onHostileQueue(t, hostile, func() { hostile.EffectList().Add(slow) })
	clientRun, _ := npcInfoSpeeds(t, c, hostile.ObjectID())
	requireClientPace(t, "run under the slow", clientRun, speedTestRun/2)
	requireCovered(t, "chasing under the slow", coveredInOneSecond(t, srv, hostile), speedTestRun/2)

	drainUntilQuiet(t, c)
	onHostileQueue(t, hostile, func() { hostile.EffectList().Remove(slow) })
	clientRun, _ = npcInfoSpeeds(t, c, hostile.ObjectID())
	requireClientPace(t, "run after the slow ends", clientRun, speedTestRun)
	requireCovered(t, "chasing after the slow ends", coveredInOneSecond(t, srv, hostile), speedTestRun)
}
