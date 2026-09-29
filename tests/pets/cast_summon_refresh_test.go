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
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// petBuffSkill is a single-target buff that changes nothing about the pet,
// so the only pet status a cast of it sends is the hit's pre-skill refresh.
const petBuffSkill = 1068

// petBuffHitAndFinish lets petBuffSkill's 500ms hit time and its finish pass.
const petBuffHitAndFinish = time.Second

func petBuffDefinition() modelskill.Definition {
	return modelskill.Definition{
		ID: petBuffSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 600,
		StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 0,
	}
}

// bootOwnerBuffer brings the owner in knowing petBuffSkill, calls out the
// wolf and targets it.
func bootOwnerBuffer(t *testing.T) (*petWorld, *summon.Actor) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		petBuffDefinition(),
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t, gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, petBuffSkill, 1); err != nil {
		t.Fatalf("seed known skill %d: %v", petBuffSkill, err)
	}
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	pet, _ := h.spawnWolf(t)
	selectPet(t, h.client, pet)
	return h, pet
}

// skillLaunchedAt is the index in frames of casterID's MagicSkillLaunched
// for skillID, or -1.
func skillLaunchedAt(frames [][]byte, casterID, skillID int32) int {
	for i, frame := range frames {
		if len(frame) < 9 || frame[0] != serverpackets.OpcodeMagicSkillLaunched {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() == casterID && r.ReadInt32() == skillID {
			return i
		}
	}
	return -1
}

// TestPlayerBuffOnPetRefreshesItsStatus has the owner buff its wolf. A
// player's hit republishes every summon among its targets after the charge
// step and before the skill lands (CreatureCast.onMagicHitTimer,
// CreatureCast.java:274-288 -> Summon.updateAndBroadcastStatus,
// Summon.java:606-612): the owner gets one PetStatusUpdate and a watching
// player one NpcInfo, both after the launch. The buff itself changes no
// vitals, so nothing else refreshes the pet.
func TestPlayerBuffOnPetRefreshesItsStatus(t *testing.T) {
	t.Parallel()
	h, pet := bootOwnerBuffer(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	drainUntilQuiet(t, h.client)
	want := currentPetVitals(pet)

	h.client.Send(encodeRequestMagicSkillUse(petBuffSkill))
	h.srv.Advance(t, petBuffHitAndFinish)
	ownerFrames := drainFrames(t, h.client)

	launched := skillLaunchedAt(ownerFrames, h.ownerID, petBuffSkill)
	var updates []petVitals
	for i, frame := range ownerFrames {
		if frame[0] != serverpackets.OpcodePetStatusUpdate {
			continue
		}
		if launched < 0 || i < launched {
			t.Fatalf("owner PetStatusUpdate at %d, MagicSkillLaunched at %d; want the refresh at the hit, after the launch", i, launched)
		}
		updates = append(updates, readPetStatusVitals(t, frame))
	}
	if len(updates) != 1 || updates[0] != want {
		t.Fatalf("owner PetStatusUpdates = %+v, want one carrying %+v", updates, want)
	}
	if n := countNPCInfoFor(drainFrames(t, watcher.client), pet.ObjectID()); n != 1 {
		t.Fatalf("watcher got %d pet NpcInfo, want 1", n)
	}
}

// TestNPCBuffOnPetSendsNoRefresh has a monster cast the same buff on the
// wolf. Only a player caster's hit republishes its summon targets
// (CreatureCast.java:274 is inside `_actor instanceof Player`), so the
// owner gets no PetStatusUpdate and a watcher no NpcInfo. The AI cast path
// this drives is the one a summon's own casts use too.
func TestNPCBuffOnPetSendsNoRefresh(t *testing.T) {
	t.Parallel()
	h, pet := bootOwnerBuffer(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	caster, aiCtl := h.srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1_000_000, PAtk: 10,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{petBuffDefinition()}))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher.client)

	if !caster.Queue().Post(func() { aiCtl.Cast(pet, modelskill.Ref{ID: petBuffSkill, Level: 1}) }) {
		t.Fatal("post monster cast: queue closed")
	}
	h.srv.Advance(t, petBuffHitAndFinish)
	if caster.CastingNow() {
		t.Fatal("monster CastingNow() = true once its buff was due to finish")
	}
	ownerFrames := drainFrames(t, h.client)

	if skillLaunchedAt(ownerFrames, caster.ObjectID(), petBuffSkill) < 0 {
		t.Fatalf("owner never saw the monster launch its buff; opcodes %x", frameOpcodes(ownerFrames))
	}
	if n := countOpcode(ownerFrames, serverpackets.OpcodePetStatusUpdate); n != 0 {
		t.Fatalf("owner got %d PetStatusUpdate from a monster's buff, want none", n)
	}
	if n := countNPCInfoFor(drainFrames(t, watcher.client), pet.ObjectID()); n != 0 {
		t.Fatalf("watcher got %d pet NpcInfo from a monster's buff, want none", n)
	}
}
