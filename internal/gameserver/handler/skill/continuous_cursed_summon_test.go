package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// anyOwner names summon.Owner for embedding: its method set holds an Owner
// method, which a field named Owner would shadow.
type anyOwner = summon.Owner

// cursedOwner is a summon owner as the BUFF gate reads it: a player and
// its cursed weapon.
type cursedOwner struct {
	anyOwner
	neutralPlayer
	cursed bool
}

func (cursedOwner) Kind() actor.Kind             { return actor.KindPlayer }
func (cursedOwner) InCombat() bool               { return false }
func (cursedOwner) SetHeading(int)               {}
func (o cursedOwner) CursedWeaponEquipped() bool { return o.cursed }
func (o cursedOwner) OlympiadMode() bool         { return o.neutralPlayer.OlympiadMode() }

// summonCaster is a summon casting for owner.
type summonCaster struct {
	*continuousFake
	owner summon.Owner
}

func (summonCaster) Kind() actor.Kind                { return actor.KindSummon }
func (summonCaster) SiegeSummon() bool               { return false }
func (s summonCaster) SummonOwner() summon.Owner     { return s.owner }
func (summonCaster) UnSummon(summon.Owner)           {}
func (summonCaster) IsPet() bool                     { return false }
func (summonCaster) AddExpAndSp(int64, int)          {}
func (summonCaster) ReviveRestoringExp(float64) bool { return false }
func (summonCaster) ResurrectOutright(float64)       {}

// TestContinuousBuffFromCursedHoldersSummon follows the BUFF gate's acting
// player: a summon whose owner holds a cursed weapon buffs no one else, as
// the holder itself would not.
func TestContinuousBuffFromCursedHoldersSummon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		cursed bool
		want   int
	}{
		{"owner holds a cursed weapon", true, 0},
		{"owner holds none", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			caster := summonCaster{continuousFake: newContinuousFake(10), owner: cursedOwner{cursed: tc.cursed}}
			target := &skillTarget{
				fakeActor: fakeActor{objectID: 2}, hp: 100, maxHP: 100, isPlayer: true,
				name: "Other", effects: newTestList(nil), skillSuccessOK: true,
			}
			continuousHandler{}.UseResult(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{ID: 1040, Level: 1, SkillType: "BUFF", Effects: buffEffect()},
				Targets: []Actor{target},
			})
			if got := len(target.effects.All()); got != tc.want {
				t.Fatalf("buffs landed = %d, want %d", got, tc.want)
			}
		})
	}
}
