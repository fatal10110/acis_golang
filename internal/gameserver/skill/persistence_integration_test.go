//go:build integration

package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestSaveExcludesToggleHerbContinuousAndHealOverTimeEffects_RealStore is the
// real-database counterpart of persistence_test.go's
// TestSaveExcludesToggleHerbContinuousAndHealOverTimeEffects: it proves the
// same exclusion rule (Player.java: effect.getEffectType() ==
// EffectType.HEAL_OVER_TIME, effect.isHerbEffect(), skill.isToggle(),
// skill.getSkillType() == SkillType.CONT) survives a real round trip through
// character_skills_save, not just a Replace() call captured in memory.
func TestSaveExcludesToggleHerbContinuousAndHealOverTimeEffects_RealStore(t *testing.T) {
	plainDef := modelskill.Definition{ID: 1, Level: 1, Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}
	toggleDef := modelskill.Definition{ID: 2, Level: 1, Activation: modelskill.ActivationToggle, Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}
	continuousDef := modelskill.Definition{ID: 3, Level: 1, SkillType: "CONT", Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}
	hotDef := modelskill.Definition{ID: 4, Level: 1, Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 1, Time: 30}}}
	herbDef := modelskill.Definition{ID: 5, Level: 1, Name: "Herb of Life", Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}

	table := modelskill.NewTable([]modelskill.Definition{plainDef, toggleDef, continuousDef, hotDef, herbDef})
	store := sql.NewSkillSaveStore(sqltest.NewDB(t))
	p := NewPersistence(store, table)

	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 0, persistenceTestGeo{}, ch)
	if err != nil {
		t.Fatalf("creature.NewLive() error: %v", err)
	}
	ch.Live = live

	for _, def := range []modelskill.Definition{plainDef, toggleDef, continuousDef, hotDef, herbDef} {
		e, err := effect.New(effect.SkillFromDefinition(def), def.Effects[0])
		if err != nil {
			t.Fatalf("effect.New(%d) error: %v", def.ID, err)
		}
		e.Effector, e.Effected = ch, ch
		ch.EffectList().Add(e)
	}

	if err := p.Save(context.Background(), ch, true); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := store.ListByCharacter(context.Background(), ch.ID, ch.SkillSaveClassIndex())
	if err != nil {
		t.Fatalf("ListByCharacter() error: %v", err)
	}
	if len(got) != 1 || got[0].Skill.ID != plainDef.ID {
		t.Fatalf("saved rows read back from the database = %+v, want exactly the plain buff (skill 1)", got)
	}
}

// TestRestoreKnownSkillsAttachesPassiveStats_RealStore is the real-database
// counterpart of persistence_test.go's TestRestoreKnownSkillsAttachesPassiveStats:
// it seeds character_skills via the store's own SetKnownSkill write path
// instead of a literal fakeSkillLevelStore map, proving Restore reads real
// persisted skill levels, not just whatever a fake was told to return.
func TestRestoreKnownSkillsAttachesPassiveStats_RealStore(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
	})
	ch := &player.Character{ID: 1}
	levels := sql.NewCharacterSkillStore(sqltest.NewDB(t))
	if err := levels.SetKnownSkill(context.Background(), ch.ID, ch.SkillSaveClassIndex(), 134, 1); err != nil {
		t.Fatalf("SetKnownSkill(134) error: %v", err)
	}
	if err := levels.SetKnownSkill(context.Background(), ch.ID, ch.SkillSaveClassIndex(), 9999, 1); err != nil {
		t.Fatalf("SetKnownSkill(9999) error: %v", err)
	}

	p := NewPersistence(nil, table, levels)
	base := ch.PAtk()

	if err := p.Restore(context.Background(), ch); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if got, want := ch.PAtk(), base+7; got != want {
		t.Fatalf("PAtk() after restoring a passive skill = %v, want %v", got, want)
	}
	if ch.SkillLevel(9999) != 0 {
		t.Fatalf("stale unloaded skill level = %d, want 0 (not restored)", ch.SkillLevel(9999))
	}
}
