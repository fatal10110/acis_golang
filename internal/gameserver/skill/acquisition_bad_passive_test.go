package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestLearnGeneralBadPassiveStillCompletes pins #2370: a passive definition
// whose stat functions fail to build cannot undo a learn the reference
// always completes (RequestAcquireSkill: removeExpAndSp, then
// Player.addSkill). LearnGeneral reports the bad definition as an error
// alongside LearnDone, with the skill known and its SP paid.
func TestLearnGeneralBadPassiveStillCompletes(t *testing.T) {
	const skillID = 9001
	p := NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
		ID: skillID, Level: 1, Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "noSuchStat", Value: 1}},
	}}))
	tmpl := &player.Template{Skills: []player.SkillGrant{{SkillID: skillID, Level: 1, MinLevel: 1, Cost: 50}}}
	ch := &player.Character{ID: 1, CharLevel: 5, SP: 80, Exp: 1000}

	result, status, err := LearnGeneral(ch, tmpl, p, modelskill.BookPolicy{}, skillID, 1)
	if err == nil {
		t.Fatal("LearnGeneral() error = nil, want the bad passive definition reported")
	}
	if status != LearnDone {
		t.Fatalf("LearnGeneral() status = %v, want LearnDone", status)
	}
	if result.SkillID != skillID || result.Level != 1 || result.Cost != 50 {
		t.Fatalf("LearnGeneral() result = %+v, want skill %d level 1 cost 50", result, skillID)
	}
	if got := ch.SkillLevel(skillID); got != 1 {
		t.Fatalf("known level = %d, want 1", got)
	}
	if got := ch.ProgressionValues().SP; got != 30 {
		t.Fatalf("SP after learn = %d, want 30", got)
	}
}
