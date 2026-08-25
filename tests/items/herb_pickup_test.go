package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// herbSkills builds the skill table the Herb of Life's carried skill
// resolves against: an instant, self-targeting heal-over-time potion.
func herbSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	store := gamesql.NewSkillSaveStore(db)
	known := gamesql.NewCharacterSkillStore(db)
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2278, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 5, Time: 3, Value: 12, Icon: true}},
		},
	}), known)
}

// bootHerbField boots a character and places one Herb of Life on the ground
// at the shared spawn point, owned by that character.
func bootHerbField(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(herbSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	startInWorld(t, c)

	srv.SeedGroundItem(t, objID, 8600, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)
	return srv, objID, groundID
}

// TestHerbPickupConsumesWithoutStoring pins the herb contract: picking a
// herb up uses it on the spot — its carried skill lands on the picker — and
// the herb never reaches the inventory, the packet stream as an item row, or
// the persisted items table.
func TestHerbPickupConsumesWithoutStoring(t *testing.T) {
	srv, objID, groundID := bootHerbField(t)
	c := srv.Client

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")

	frames := collectUntilQuiet(t, c)
	var sawCast bool
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeMagicSkillUse {
			assertMagicSkillUseSelf(t, f, objID, 2278, 1, 0, 0)
			sawCast = true
		}
		if f[0] == serverpackets.OpcodeInventoryUpdate {
			for _, e := range readInventoryUpdateEntries(t, f) {
				if e.itemID == 8600 {
					t.Fatalf("herb reached the inventory stream: entry %+v", e)
				}
			}
		}
	}
	if !sawCast {
		t.Fatalf("herb pickup produced no carried-skill cast across %d frames", len(frames))
	}

	if _, ok := srv.State.Object(groundID); ok {
		t.Fatal("ground herb still present after pickup")
	}
	srv.FlushItems(t)
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == 8600 {
			t.Fatalf("herb persisted to the items table: %+v", inst)
		}
	}
}

// TestHerbPickupMirrorsOntoServitor pins issue #1246's gate: an active
// servitor receives a mirrored copy of the herb's cast; a pet would not.
func TestHerbPickupMirrorsOntoServitor(t *testing.T) {
	srv, objID, groundID := bootHerbField(t)
	c := srv.Client

	servitor := summon.NewServitor(summon.ServitorConfig{
		ObjectID: srv.NewObjectID(),
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200},
	})
	srv.State.AddSummon(objID, servitor)
	drainUntilQuiet(t, c)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")

	frames := collectUntilQuiet(t, c)
	playerCasts, servitorCasts := 0, 0
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		r := wire.NewReader(f[1:])
		caster, target, sid := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		switch {
		case caster == objID && target == objID && sid == 2278:
			playerCasts++
		case caster == servitor.ObjectID() && target == servitor.ObjectID() && sid == 2278:
			servitorCasts++
		}
	}
	if playerCasts != 1 || servitorCasts != 1 {
		t.Fatalf("herb casts = player %d servitor %d, want 1/1 across %d frames", playerCasts, servitorCasts, len(frames))
	}
}
