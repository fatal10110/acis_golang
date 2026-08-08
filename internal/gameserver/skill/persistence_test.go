package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

type persistenceTestGeo struct{}

func (persistenceTestGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (persistenceTestGeo) Height(_, _, _ int) int16          { return 0 }
func (persistenceTestGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (persistenceTestGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func TestSetKnownSkillAttachesPassiveStatsAndReplacesOnRelearn(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
		{ID: 134, Level: 2, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 15},
		}},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.SetKnownSkill(context.Background(), ch, 134, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got, want := ch.PAtk(), base+7; got != want {
		t.Fatalf("PAtk() after learning level 1 = %v, want %v", got, want)
	}

	if err := p.SetKnownSkill(context.Background(), ch, 134, 2); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got, want := ch.PAtk(), base+15; got != want {
		t.Fatalf("PAtk() after relearning at level 2 = %v, want %v (level 1's bonus must be dropped)", got, want)
	}
}

func TestSetKnownSkillDoesNotApplyStatsForNonPassiveOrUnloadedSkill(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 60, Level: 1, Activation: modelskill.ActivationToggle},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.SetKnownSkill(context.Background(), ch, 60, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after learning a toggle skill = %v, want unchanged %v", got, base)
	}
	if ch.SkillLevel(60) != 1 {
		t.Fatalf("SkillLevel(60) = %d, want 1", ch.SkillLevel(60))
	}

	if err := p.SetKnownSkill(context.Background(), ch, 999, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after learning an unloaded skill = %v, want unchanged %v", got, base)
	}
	if ch.SkillLevel(999) != 1 {
		t.Fatalf("SkillLevel(999) = %d, want 1", ch.SkillLevel(999))
	}
}

func TestSetKnownSkillDropsPassiveStatsWhenSkillIsRemoved(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
	})
	p := NewPersistence(nil, table)
	ch := &player.Character{ID: 1}
	base := ch.PAtk()

	if err := p.SetKnownSkill(context.Background(), ch, 134, 1); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}
	if err := p.SetKnownSkill(context.Background(), ch, 134, 0); err != nil {
		t.Fatalf("SetKnownSkill() error: %v", err)
	}

	if got := ch.PAtk(); got != base {
		t.Fatalf("PAtk() after removing the passive skill = %v, want unchanged %v", got, base)
	}
	if ch.SkillLevel(134) != 0 {
		t.Fatalf("SkillLevel(134) = %d, want 0", ch.SkillLevel(134))
	}
}

func TestRestoreKnownSkillsAttachesPassiveStats(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 134, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7},
		}},
	})
	p := NewPersistence(nil, table, fakeSkillLevelStore{levels: player.SkillLevels{134: 1, 9999: 1}})
	ch := &player.Character{ID: 1}
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

// fakeSkillSaveStore captures whatever rows Save last replaced, standing in
// for the real character_skills_save table.
type fakeSkillSaveStore struct {
	rows []effect.SaveRow
}

func (s *fakeSkillSaveStore) Replace(_ context.Context, _ int32, _ int32, rows []effect.SaveRow) error {
	s.rows = append([]effect.SaveRow(nil), rows...)
	return nil
}
func (s *fakeSkillSaveStore) ListByCharacter(context.Context, int32, int32) ([]effect.SaveRow, error) {
	return nil, nil
}
func (s *fakeSkillSaveStore) DeleteByCharacter(context.Context, int32, int32) (int64, error) {
	return 0, nil
}

// TestSaveExcludesToggleHerbContinuousAndHealOverTimeEffects proves Save's
// live-effect-list snapshot applies the same exclusions
// Player.storeEffect() does (Player.java: effect.getEffectType() ==
// EffectType.HEAL_OVER_TIME, effect.isHerbEffect(), skill.isToggle(),
// skill.getSkillType() == SkillType.CONT): only a plain buff among these
// five survives into a saved row.
func TestSaveExcludesToggleHerbContinuousAndHealOverTimeEffects(t *testing.T) {
	plainDef := modelskill.Definition{ID: 1, Level: 1, Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}
	toggleDef := modelskill.Definition{ID: 2, Level: 1, Activation: modelskill.ActivationToggle, Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}
	continuousDef := modelskill.Definition{ID: 3, Level: 1, SkillType: "CONT", Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}
	hotDef := modelskill.Definition{ID: 4, Level: 1, Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 1, Time: 30}}}
	// Name (not an explicit flag) is what marks a herb effect, mirroring
	// AbstractEffect._isHerbEffect = _skill.getName().contains("Herb") — the
	// same derivation effect.New applies on every real herb-item cast.
	herbDef := modelskill.Definition{ID: 5, Level: 1, Name: "Herb of Life", Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 30}}}

	table := modelskill.NewTable([]modelskill.Definition{plainDef, toggleDef, continuousDef, hotDef, herbDef})
	store := &fakeSkillSaveStore{}
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
	if len(store.rows) != 1 || store.rows[0].Skill.ID != plainDef.ID {
		t.Fatalf("saved rows = %+v, want exactly the plain buff (skill 1)", store.rows)
	}
}

type fakeSkillLevelStore struct {
	levels player.SkillLevels
}

func (s fakeSkillLevelStore) ListKnownSkills(context.Context, int32, int32) (player.SkillLevels, error) {
	return s.levels, nil
}
