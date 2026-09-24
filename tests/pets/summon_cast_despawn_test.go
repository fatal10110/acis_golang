package pets

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// wolfStrikeAction is the pet special-skill shortcut that casts
	// wolfStrikeSkill on the owner's target.
	wolfStrikeAction  = int32(36)
	wolfStrikeSkill   = 4259
	wolfStrikeHitTime = 1000
	// wolfStrikeLaunch is when the launch phase comes due after cast start.
	wolfStrikeLaunch = (wolfStrikeHitTime - 400) * time.Millisecond
)

// wolfStrike is the lethal single-target strike bootWolfStriker teaches.
func wolfStrike() modelskill.Definition {
	return modelskill.Definition{
		ID: wolfStrikeSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, SkillType: "PDAM",
		CastRange: 900, HitTime: wolfStrikeHitTime, ReuseDelay: 60_000,
		StaticHitTime: true, StaticReuse: true, Power: 1_000_000,
	}
}

// bootWolfStriker brings the owner in with a wolf that knows wolfStrike,
// summons it, and targets the fixture monster.
func bootWolfStriker(t *testing.T) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	return bootWolfStrikerWith(t, wolfStrike())
}

// bootWolfStrikerWith is bootWolfStriker with a caller-tuned strike.
func bootWolfStrikerWith(t *testing.T, strike modelskill.Definition) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{wolfStrikeSkill: 1}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		strike,
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
	})
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	return h, petActor, hostile
}

// startWolfStrike presses the strike shortcut. The ActionFailed that
// releases the click is sent once the cast has started and its launch is
// armed.
func startWolfStrike(t *testing.T, h *petWorld) {
	t.Helper()
	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
}

// TestPetStrikeLandsOnTarget is the control for the despawn scenario below:
// left alone, the same strike reaches its hit and drops the monster's HP.
func TestPetStrikeLandsOnTarget(t *testing.T) {
	h, _, hostile := bootWolfStriker(t)
	startWolfStrike(t, h)
	h.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
	drainUntilQuiet(t, h.client)
}

// TestPetDespawnedBetweenLaunchAndHitLandsNothing unsummons the pet after
// its strike has launched but before the hit comes due. The despawn aborts
// the cast, so the monster keeps its HP and observers see no hit.
//
// On a driven clock the clock stops midway between the launch and hit
// deadlines. On the real pool the owner's queue is parked across the launch
// deadline instead, so the launch task and the despawn run back to back, in
// that order, before the hit is ever armed. Parking would block the driven
// clock's single runner, hence the two paths.
func TestPetDespawnedBetweenLaunchAndHitLandsNothing(t *testing.T) {
	h, petActor, hostile := bootWolfStriker(t)
	q := petActor.Queue()
	if q == nil {
		t.Fatal("pet has no actor queue")
	}
	startWolfStrike(t, h)

	const toHit = wolfStrikeHitTime*time.Millisecond - wolfStrikeLaunch
	if h.srv.DrivesClock() {
		h.srv.Advance(t, wolfStrikeLaunch+toHit/2)
		if !q.Post(petActor.Unsummon) {
			t.Fatal("post unsummon: queue closed")
		}
	} else {
		castStarted := time.Now()
		release := make(chan struct{})
		if !q.Post(func() { <-release }) {
			t.Fatal("park owner queue: queue closed")
		}
		// Let the launch deadline pass so its task queues behind the park.
		time.Sleep(time.Until(castStarted.Add(wolfStrikeLaunch + 300*time.Millisecond)))
		if !q.Post(petActor.Unsummon) {
			close(release)
			t.Fatal("post unsummon: queue closed")
		}
		close(release)
	}
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	// Let the hit the launch armed come due.
	h.srv.Advance(t, toHit)

	if hp, full := hostile.HP(), float64(hostile.MaxHP()); hp != full {
		t.Fatalf("monster HP = %v after despawn between launch and hit, want untouched %v", hp, full)
	}
	for _, frame := range drainFrames(t, h.client) {
		switch frame[0] {
		case serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeDie:
			t.Fatalf("owner received opcode %#x after despawn; the aborted strike must not reach observers", frame[0])
		}
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("pet still active in world after despawn")
	}
}
