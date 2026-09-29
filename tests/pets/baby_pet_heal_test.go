package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: BabyPet (aCis_gameserver/java/net/sf/l2j/gameserver/model/
// actor/instance/BabyPet.java): onSpawn/doRevive start the heal task
// (:35-40, :62-67), doDie/unSummon stop it (:43-59), startCastTask schedules
// castSkill at a fixed rate of 1000 ms after 3000 ms (:78-82), getSkillLevel
// is level/10 below level 70 (:70-76), and castSkill (:93-121) rolls
// Rnd.get(100) <= 25 for Heal Trick (4717) below 80% owner HP, then
// Rnd.get(100) <= 75 for Greater Heal Trick (4718) below 15%, each gated on
// reuse and on the pet's MP covering mpConsume, and sends the owner
// PET_USES_S1 naming the skill after handing the cast to the AI.
//
// The heal definitions are level 4 of skills 4717 and 4718 as
// aCis_datapack/data/xml/skills/4700-4799.xml defines them; a level-45 baby
// pet casts level 45/10 = 4.

const (
	babyLevel      = 45
	babyHealLevel  = 4
	babyMaxHP      = 1000
	babyMaxMP      = 300
	babyLevelExp   = int64(100_000)
	babyNextExp    = int64(200_000)
	babySeededExp  = int64(150_000)
	babyWeakHeal   = 4717
	babyStrongHeal = 4718
	// babyStrongHealMP is Greater Heal Trick's level-4 MP cost.
	babyStrongHealMP = 122
)

// babyPetTemplate is the wolf fixture as a baby pet at level 45.
func babyPetTemplate() *npc.Template {
	baby := wolfTemplate()
	baby.Type = "BabyPet"
	baby.Name = "Baby Buffalo"
	stats := func(maxExp int64) npc.PetLevelStats {
		s := wolfLevelStats(maxExp)
		s.MaxHP, s.MaxMP = babyMaxHP, babyMaxMP
		return s
	}
	baby.Pet.Levels = map[int]npc.PetLevelStats{
		babyLevel:     stats(babyLevelExp),
		babyLevel + 1: stats(babyNextExp),
	}
	return baby
}

func babyHealSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	heal := func(id modelskill.ID, mp int, power float32) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: babyHealLevel, Activation: modelskill.ActivationActive, Magic: true,
			Target: modelskill.TargetOwnerPet, SkillType: "HEAL", MPConsume: mp, Power: power,
			MagicLevel: 40, CastRange: 600, EffectRange: 1100, HitTime: 4000, ReuseDelay: 8000,
		}
	}
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{
			ID: petResurrectSkillID, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
			CastRange: 400, HitTime: 500, StaticHitTime: true, Power: 100,
		},
		heal(babyWeakHeal, 17, 37),
		heal(babyStrongHeal, babyStrongHealMP, 272),
	}), gamesql.NewCharacterSkillStore(db))
}

// babyScene is an owner with its baby pet out, and when the pet's heal task
// last started on the driven clock (to within one AdvanceUntil step).
type babyScene struct {
	*petWorld
	baby    *summon.Actor
	started time.Time
}

// bootBabyPet brings an owner who knows a resurrection in the world and
// calls out its level-45 baby pet with every heal roll fixed to roll.
func bootBabyPet(t *testing.T, roll int) *babyScene {
	t.Helper()
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{babyPetTemplate(), treeTemplate()})),
		gameservertest.WithSkills(babyHealSkills(t)),
	)
	if !srv.DrivesClock() {
		t.Skip("timing one-second heal ticks needs the driven clock")
	}
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.Pets.Save(petCtx(), collarID, pet.State{
		Level: babyLevel, Exp: babySeededExp, CurHP: babyMaxHP, CurMP: babyMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)

	h.client.Send(encodeUseItem(collarID, false))
	var baby *summon.Actor
	srv.AdvanceUntil(t, "baby pet in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if ok {
			baby, ok = obj.(*summon.Actor)
		}
		return ok
	})
	s := &babyScene{petWorld: h, baby: baby, started: baby.Now()}
	if !baby.IsBabyPet() {
		t.Fatal("the BabyPet template spawned an ordinary pet")
	}
	runOnPetQueue(t, baby, func() { baby.SetRollSource(func(n int) int { return min(roll, n-1) }) })
	drainUntilQuiet(t, h.client)
	return s
}

// woundOwner leaves the owner at share of its max HP.
func (s *babyScene) woundOwner(t *testing.T, share float64) {
	t.Helper()
	maxHP := s.srv.PlayerMaxHP(t, s.ownerID)
	s.srv.DamagePlayerHP(t, s.ownerID, s.srv.PlayerCurrentHP(t, s.ownerID)-int(float64(maxHP)*share))
	drainUntilQuiet(t, s.client)
}

// framesUntil reads the owner's frames until the driven clock reaches
// deadline, with the clock reading each one arrived at.
func (s *babyScene) framesUntil(t *testing.T, deadline time.Time) (frames [][]byte, at []time.Time) {
	t.Helper()
	for now := s.baby.Now(); now.Before(deadline); now = s.baby.Now() {
		if frame := s.client.ReadWithTimeout(deadline.Sub(now)); frame != nil {
			frames = append(frames, frame)
			at = append(at, s.baby.Now())
		}
	}
	return frames, at
}

// petUsesAt returns the index of the PET_USES_S1 message among frames, or -1.
func petUsesAt(frames [][]byte) int {
	return frameIndex(frames, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessagePetUsesS1)
}

// awaitFirstHeal reads the owner's frames until 3.5 s after the heal task
// started and checks the one heal skill it holds: the pet's cast on its
// owner, then the owner's PET_USES_S1 naming it, both on the first tick, 3 s
// after the start. skill 0 wants no heal at all.
func (s *babyScene) awaitFirstHeal(t *testing.T, skill int32) {
	t.Helper()
	frames, at := s.framesUntil(t, s.started.Add(3500*time.Millisecond))
	cast := frameIndex(frames, serverpackets.OpcodeMagicSkillUse, s.baby.ObjectID())
	uses := petUsesAt(frames)
	if skill == 0 {
		if cast >= 0 || uses >= 0 {
			t.Fatalf("the baby pet healed its owner: frames %x", frameOpcodes(frames))
		}
		return
	}
	if cast < 0 || uses < 0 || cast > uses {
		t.Fatalf("owner frames %x: pet MagicSkillUse at %d, PET_USES_S1 at %d; want both, the cast first", frameOpcodes(frames), cast, uses)
	}
	// The start is read up to one 10 ms AdvanceUntil step after it happened.
	if elapsed := at[uses].Sub(s.started); elapsed < 2990*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("first heal %v after the task started, want 3s", elapsed)
	}
	r := wire.NewReader(frames[cast][1:])
	if caster, target, id, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != s.baby.ObjectID() || target != s.ownerID || id != skill || level != babyHealLevel {
		t.Fatalf("MagicSkillUse = %d onto %d skill %d/%d, want pet %d onto owner %d skill %d/%d",
			caster, target, id, level, s.baby.ObjectID(), s.ownerID, skill, babyHealLevel)
	}
	assertSystemMessageSkill(t, frames[uses], serverpackets.SystemMessagePetUsesS1, skill, babyHealLevel)
}

// assertNoHealFor reads the owner's frames for d and checks the pet neither
// cast nor told its owner it did.
func (s *babyScene) assertNoHealFor(t *testing.T, d time.Duration, what string) {
	t.Helper()
	frames, _ := s.framesUntil(t, s.baby.Now().Add(d))
	if petUsesAt(frames) >= 0 || frameIndex(frames, serverpackets.OpcodeMagicSkillUse, s.baby.ObjectID()) >= 0 {
		t.Fatalf("%s: the baby pet healed its owner (frames %x)", what, frameOpcodes(frames))
	}
}

// TestBabyPetHealsWoundedOwner has a freshly called baby pet consider its
// wounded owner on its first tick, 3 s after it came out: the roll and the
// owner's HP share pick the heal. A roll of 25 still passes the weak heal's
// "<= 25"; 26 fails it and passes the strong heal's "<= 75", which then only
// fires below 15%.
func TestBabyPetHealsWoundedOwner(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		roll  int
		share float64
		skill int32 // 0: no heal
	}{
		{name: "weak heal below 80%", roll: 25, share: 0.5, skill: babyWeakHeal},
		{name: "weak heal wins below 15% too", roll: 0, share: 0.1, skill: babyWeakHeal},
		{name: "strong heal below 15%", roll: 26, share: 0.1, skill: babyStrongHeal},
		{name: "strong roll above 15% heals nothing", roll: 26, share: 0.5},
		{name: "strong roll missed", roll: 76, share: 0.1},
		{name: "owner at 90% is left alone", roll: 0, share: 0.9},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootBabyPet(t, tt.roll)
			s.woundOwner(t, tt.share)
			s.awaitFirstHeal(t, tt.skill)
			if tt.skill == 0 {
				return
			}
			before := s.srv.PlayerCurrentHP(t, s.ownerID)
			s.srv.AdvanceUntil(t, "the heal landing on the owner", func() bool {
				return s.srv.PlayerCurrentHP(t, s.ownerID) > before
			})
		})
	}
}

// TestBabyPetStrongHealNeedsMP: a pet short of Greater Heal Trick's MP cost
// does not cast it, and its owner reads nothing.
func TestBabyPetStrongHealNeedsMP(t *testing.T) {
	t.Parallel()
	s := bootBabyPet(t, 26)
	runOnPetQueue(t, s.baby, func() { s.baby.ReduceMP(s.baby.MPValue() - (babyStrongHealMP - 1)) })
	s.woundOwner(t, 0.1)
	s.awaitFirstHeal(t, 0)
}

// TestBabyPetHealStopsOnDeathAndResumesOnRevive: a dead baby pet casts
// nothing on its wounded owner. Revived by its owner, it waits 3 s again,
// then heals.
func TestBabyPetHealStopsOnDeathAndResumesOnRevive(t *testing.T) {
	t.Parallel()
	s := bootBabyPet(t, 0)
	killPet(t, s.petWorld, s.baby)
	s.woundOwner(t, 0.5)
	s.assertNoHealFor(t, 5*time.Second, "dead")

	x, y, z := s.baby.Position()
	s.client.Send(encodeAction(s.baby.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, s.client)
	s.client.Send(encodeRequestMagicSkillUse(petResurrectSkillID))
	s.srv.AdvanceUntil(t, "the baby pet revived", func() bool { return !s.baby.Dead() })
	s.started = s.baby.Now()
	s.awaitFirstHeal(t, babyWeakHeal)
}

// TestReturnedBabyPetStopsHealing: once the owner sends its baby pet back,
// its heal task is gone with it, though the owner's queue it ran on stays
// open.
func TestReturnedBabyPetStopsHealing(t *testing.T) {
	t.Parallel()
	s := bootBabyPet(t, 0)
	s.returnPet(t)
	s.woundOwner(t, 0.5)
	s.assertNoHealFor(t, 5*time.Second, "returned")
}
