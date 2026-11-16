package clan

import (
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: a hit that is no damage-over-time tick ends the target's
// sleep and hold-until-attacked, and its stun when Rnd.get(10) is 0
// (CreatureStatus.reduceHp). While stunned, asleep or held the NPC takes
// no AI action (Creature.denyAiAction), so a queued social desire waits.

const (
	towerStunSkillID = 9304
	towerHoldSkillID = 9305
)

// towerWakeSkills adds a stun and a hold-until-attacked, each cast on one
// target with no cost, to the tower effect skills.
func towerWakeSkills() []modelskill.Definition {
	def := func(id int, skillType string, effect modelskill.EffectTemplate) modelskill.Definition {
		return modelskill.Definition{
			ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, SkillType: skillType, Power: 100, CastRange: 900, HitTime: 500,
			StaticHitTime: true, StaticReuse: true, Effects: []modelskill.EffectTemplate{effect},
		}
	}
	return append(towerEffectSkills(),
		def(towerStunSkillID, "STUN", modelskill.EffectTemplate{
			Name: "Stun", Count: 1, Time: 60, StackType: "stun", StackOrder: 1, Icon: true,
		}),
		def(towerHoldSkillID, "ROOT", modelskill.EffectTemplate{
			Name: "ImmobileUntilAttacked", Count: 1, Time: 60, StackType: "hold", StackOrder: 1, Icon: true,
		}),
	)
}

// TestControlTowerWakesOnHit stuns a life tower during the siege: a poison
// tick on a roll of 0 and a hit on a roll of 1 leave the stun on, a hit on
// a roll of 0 ends it. A hold-until-attacked and a sleep each end on the
// next hit. While any of the three holds the tower plays no social desire
// a script queues; its awake twin, and the tower once woken, do.
func TestControlTowerWakesOnHit(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	w := towerWorld(t, gameservertest.WithSkills(skillstate.NewPersistence(
		gamesql.NewSkillSaveStore(db), modelskill.NewTable(towerWakeSkills()), gamesql.NewCharacterSkillStore(db))))
	if !w.srv.DrivesClock() {
		t.Skip("the effects' ticks need the driven clock")
	}
	gludio, tower, twin, _ := towers(t, w)
	for _, id := range []int{towerStunSkillID, towerHoldSkillID, towerSleepSkillID} {
		if err := w.srv.KnownSkills.SetKnownSkill(t.Context(), w.leaderID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	// The tower's roll is set before anything can draw from it.
	var roll atomic.Int32
	roll.Store(1)
	tower.SetRollSource(func(n int) int { return min(int(roll.Load()), n-1) })
	startInWorld(t, w.leader)
	player, ok := w.srv.State.Player(w.leaderID)
	if !ok {
		t.Fatal("founder not in the world")
	}
	// Every landing roll succeeds.
	player.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return 0 })
	gludio.Start()
	drainFrames(t, w.leader)

	w.leader.Send(encodeAction(tower.ObjectID(), w.at))
	drainFrames(t, w.leader)
	// One AI tick has run on each tower, so their desires are taken up.
	for _, f := range []*npc.Folk{tower, twin} {
		if err := f.TickThink(); err != nil {
			t.Fatalf("first TickThink() error: %v", err)
		}
	}
	drainFrames(t, w.leader)
	// social queues a script's social desire on f, runs one AI tick and
	// counts the animations f plays.
	social := func(f *npc.Folk) int {
		t.Helper()
		f.AddSocialDesire(2, 0, 50)
		if err := f.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
		n := 0
		for _, fr := range drainFrames(t, w.leader) {
			if len(fr) >= 5 && fr[0] == serverpackets.OpcodeSocialAction && int32(binary.LittleEndian.Uint32(fr[1:5])) == f.ObjectID() {
				n++
			}
		}
		return n
	}
	cast := func(skillID int32) {
		t.Helper()
		w.leader.Send(encodeRequestMagicSkillUse(skillID))
		if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeMagicSkillUse); !ok {
			t.Fatalf("skill %d on the tower did not start", skillID)
		}
		w.srv.AdvanceWithEffects(t, time.Second)
		drainFrames(t, w.leader)
		if _, held := tower.EffectList().ActiveBySkillID(int(skillID)); !held {
			t.Fatalf("skill %d landed nothing on the tower", skillID)
		}
		if n := social(tower); n != 0 {
			t.Fatalf("the tower held by skill %d played %d social animations, want none", skillID, n)
		}
	}
	held := func(skillID int) bool {
		_, ok := tower.EffectList().ActiveBySkillID(skillID)
		return ok
	}
	if n := social(twin); n != 1 {
		t.Fatalf("the awake tower played %d social animations, want 1", n)
	}

	cast(towerStunSkillID)
	roll.Store(0)
	tower.ReduceHPByDOT(1, nil, true)
	if !held(towerStunSkillID) {
		t.Fatal("a poison tick on a roll of 0 ended the stun")
	}
	roll.Store(1)
	tower.TakeDamage(1, nil)
	if !held(towerStunSkillID) {
		t.Fatal("a hit on a roll of 1 ended the stun")
	}
	roll.Store(0)
	tower.TakeDamage(1, nil)
	if held(towerStunSkillID) {
		t.Fatal("a hit on a roll of 0 left the stun on")
	}

	cast(towerHoldSkillID)
	tower.ReduceHPByDOT(1, nil, true)
	if !held(towerHoldSkillID) {
		t.Fatal("a poison tick ended the hold")
	}
	tower.TakeDamage(1, nil)
	if held(towerHoldSkillID) {
		t.Fatal("a hit left the hold on")
	}

	cast(towerSleepSkillID)
	tower.TakeDamage(1, nil)
	if held(towerSleepSkillID) {
		t.Fatal("a hit left the tower asleep")
	}
	if n := social(tower); n != 1 {
		t.Fatalf("the woken tower played %d social animations, want 1", n)
	}
}
