package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// petMoveDest returns the destination of a MoveToLocation frame when it
// moves objectID.
func petMoveDest(frames [][]byte, objectID int32) (location.Location, bool) {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}, true
	}
	return location.Location{}, false
}

// landPetFear applies a real Fear effect from the monster to the pet on the
// pet's queue and waits for its start hook to finish.
func landPetFear(t *testing.T, petActor *summon.Actor, effector effect.Actor, skillID modelskill.ID, count, period int) *effect.Effect {
	t.Helper()
	return landPetEffect(t, petActor, effector, "Fear", skillID, count, period)
}

// landPetEffect applies the named real effect from effector to the summon on
// the summon's queue and waits for its start hook to finish.
func landPetEffect(t *testing.T, petActor *summon.Actor, effector effect.Actor, name string, skillID modelskill.ID, count, period int) *effect.Effect {
	t.Helper()
	e, err := effect.New(
		effect.Skill{ID: skillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: name, Count: count, Time: period, Icon: true},
	)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = effector, petActor
	done := make(chan struct{})
	if !petActor.Queue().Post(func() { petActor.EffectList().Add(e); close(done) }) {
		t.Fatalf("post %s: queue closed", name)
	}
	<-done
	return e
}

// TestFearedPetFleesOnceThenStaysPut pins fear on a pet. Fear (1092) runs at
// half its count on a playable. The landing walks the pet 500 units away from
// the monster; later ticks find it already afraid and leave it in place.
func TestFearedPetFleesOnceThenStaysPut(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	origin := petActor.Move().Position()
	fear := landPetFear(t, petActor, hostile, 1092, 6, 2)

	if !petActor.Afraid() {
		t.Fatal("Afraid() = false after fear landed, want true")
	}
	if got := fear.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d for Fear on a pet, want the halved 3", got)
	}
	hx, hy, _ := hostile.Position()
	dest, ok := petMoveDest(drainFrames(t, h.client), petActor.ObjectID())
	if !ok {
		t.Fatal("fear landing sent no flee MoveToLocation for the pet")
	}
	if want := origin.FleeFrom(hx, hy, 500); dest.X != want.X || dest.Y != want.Y {
		t.Fatalf("pet flee dest = %+v from %+v, want %+v", dest, origin, want)
	}

	h.srv.Advance(t, 2*time.Second)
	h.srv.TickEffects()
	if _, moved := petMoveDest(drainFrames(t, h.client), petActor.ObjectID()); moved {
		t.Fatal("tick flee moved an already-afraid pet, want it left in place")
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent while afraid = %v, want follow-owner", got)
	}
}

// TestPetFollowsOwnerOnceFleeEndsAfterFear pins the end of a flee walk that
// outlasts its fear: the walk's arrival sends the pet idle, and an idle pet
// that follows its owner walks back to it.
func TestPetFollowsOwnerOnceFleeEndsAfterFear(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	landPetFear(t, petActor, hostile, 101, 1, 1)
	if _, ok := petMoveDest(drainFrames(t, h.client), petActor.ObjectID()); !ok {
		t.Fatal("fear landing sent no flee MoveToLocation for the pet")
	}

	h.srv.Advance(t, time.Second)
	h.srv.TickEffects()
	if petActor.Afraid() {
		t.Fatal("Afraid() = true after the single tick, want the fear gone")
	}
	if !petActor.IsMoving() {
		t.Fatal("IsMoving() = false once the fear ended, want the flee walk still running")
	}
	drainFrames(t, h.client)

	h.srv.Advance(t, 5*time.Second)
	if !petMovesTo(drainFrames(t, h.client), petActor.ObjectID()) {
		t.Fatal("pet stayed where its flee ended, want it walking back to its owner")
	}
}

// petMovesTo reports whether frames start a walk for objectID, toward a pawn
// or a location.
func petMovesTo(frames [][]byte, objectID int32) bool {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToPawn && frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		if wire.NewReader(frame[1:]).ReadInt32() == objectID {
			return true
		}
	}
	return false
}

// TestRootedPetFearedStaysPutAndStaysAfraid pins fear landing on a rooted
// pet: the flee request finds it unable to move, so it goes idle instead of
// walking, and the fear is still held and counting.
func TestRootedPetFearedStaysPutAndStaysAfraid(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	landPetEffect(t, petActor, petActor, "Root", 102, 30, 1)
	if !petActor.MovementDisabled() {
		t.Fatal("MovementDisabled() = false after Root landed, want true")
	}
	drainFrames(t, h.client)
	origin := petActor.Move().Position()

	fear := landPetFear(t, petActor, hostile, 1092, 6, 2)
	if petMovesTo(drainFrames(t, h.client), petActor.ObjectID()) {
		t.Fatal("fear landing walked a rooted pet, want it left in place")
	}
	if got := petActor.Move().Position(); got != origin {
		t.Fatalf("rooted pet position = %+v after fear, want %+v", got, origin)
	}
	if !petActor.Afraid() || !fear.InUse() {
		t.Fatalf("Afraid() = %v, fear in use = %v on a rooted pet, want the fear held", petActor.Afraid(), fear.InUse())
	}

	h.srv.Advance(t, 2*time.Second)
	h.srv.TickEffects()
	if petMovesTo(drainFrames(t, h.client), petActor.ObjectID()) {
		t.Fatal("tick flee walked a rooted pet, want it left in place")
	}
	if got := fear.Remaining(); got != 2 {
		t.Fatalf("Remaining() = %d after one tick, want 2", got)
	}
}

// Siege-summon fixture: a servitor skill that calls the Siege Golem
// template, whose npc id marks it a siege summon.
const (
	siegeGolemNPCID    = 14737
	summonGolemSkillID = 13
)

// bootServitorOwner brings the owner in with one servitor skill that calls
// tmpl, casts it, waits for the servitor to reach world state, and spawns
// the fixture monster.
func bootServitorOwner(t *testing.T, tmpl *npc.Template, skillID int) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", NpcID: tmpl.ID, SummonTotalLifeTime: 1_200_000,
			StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), tmpl})),
		gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, skillID, 1); err != nil {
		t.Fatalf("seed known skill %d: %v", skillID, err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	h.client.Send(encodeRequestMagicSkillUse(int32(skillID)))
	var actor *summon.Actor
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if !ok {
			return false
		}
		actor, ok = obj.(*summon.Actor)
		return ok
	})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)
	return h, actor, hostile
}

// siegeGolemTemplate is a servitor template carrying the Siege Golem id.
func siegeGolemTemplate() *npc.Template {
	return &npc.Template{
		ID: siegeGolemNPCID, TemplateID: siegeGolemNPCID, Type: "SiegeSummon", Name: "Siege Golem", Level: 60,
		HPMax: 5000, MPMax: 100, AtkSpd: 300, RunSpeed: 60, WalkSpeed: 30,
		CollisionRadius: 60, CollisionHeight: 60,
	}
}

// catTemplate is an ordinary servitor template.
func catTemplate() *npc.Template {
	return &npc.Template{
		ID: catNPCID, TemplateID: catNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}
}

// TestSiegeSummonRejectsFearAndBluff pins a siege summon's immunity: a Fear
// landing on a Siege Golem is refused at start, so it is not afraid, holds
// no effect and runs nowhere; a Bluff is refused the same way.
func TestSiegeSummonRejectsFearAndBluff(t *testing.T) {
	t.Parallel()
	h, golem, hostile := bootServitorOwner(t, siegeGolemTemplate(), summonGolemSkillID)
	fear := landPetFear(t, golem, hostile, 1092, 6, 2)
	if golem.Afraid() || fear.InUse() {
		t.Errorf("siege golem Afraid() = %v, fear in use = %v, want the fear refused", golem.Afraid(), fear.InUse())
	}
	if _, moved := petMoveDest(drainFrames(t, h.client), golem.ObjectID()); moved {
		t.Error("fear sent a flee MoveToLocation for a siege golem, want none")
	}

	bluff := landPetEffect(t, golem, hostile, "Bluff", 358, 1, 3)
	if bluff.InUse() {
		t.Error("siege golem holds the bluff, want it refused")
	}
	if got := len(golem.EffectList().All()); got != 0 {
		t.Errorf("siege golem holds %d effects, want none", got)
	}
	if !golem.SiegeSummon() || !golem.FearImmune() || !golem.BluffExempt() {
		t.Errorf("siege golem SiegeSummon/FearImmune/BluffExempt = %v/%v/%v, want all true",
			golem.SiegeSummon(), golem.FearImmune(), golem.BluffExempt())
	}
}

// TestOrdinaryServitorTakesBluff is the control for the siege-summon case: an
// ordinary servitor is neither fear-immune nor bluff-exempt, and a Bluff
// landing on it is held.
func TestOrdinaryServitorTakesBluff(t *testing.T) {
	t.Parallel()
	_, cat, hostile := bootServitorOwner(t, catTemplate(), summonCatSkillID)
	if cat.SiegeSummon() || cat.FearImmune() || cat.BluffExempt() {
		t.Fatalf("servitor SiegeSummon/FearImmune/BluffExempt = %v/%v/%v, want all false",
			cat.SiegeSummon(), cat.FearImmune(), cat.BluffExempt())
	}
	bluff := landPetEffect(t, cat, hostile, "Bluff", 358, 1, 3)
	if !bluff.InUse() {
		t.Fatal("servitor refused bluff, want it held")
	}
}

// TestPetIsNeitherFearImmuneNorBluffExempt is the pet side of the control:
// only siege servitors are exempt.
func TestPetIsNeitherFearImmuneNorBluffExempt(t *testing.T) {
	t.Parallel()
	_, wolf, _ := bootWolfStriker(t)
	if wolf.FearImmune() || wolf.BluffExempt() {
		t.Fatalf("pet FearImmune/BluffExempt = %v/%v, want false", wolf.FearImmune(), wolf.BluffExempt())
	}
}
