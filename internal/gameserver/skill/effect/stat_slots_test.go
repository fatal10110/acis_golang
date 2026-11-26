package effect

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// TestStatSlots pins the slot set every creature keeps: a slot is created
// once, on first use, by the creature's constructor; removing an owner
// reports each dropped Mod's stat, in stat order, once per Mod.
func TestStatSlots(t *testing.T) {
	t.Parallel()
	var ss StatSlots
	if ss.Lookup(stat.PowerAttack) != nil {
		t.Fatal("Lookup on empty slots returned a Calculator")
	}
	created := 0
	newCalc := func(stat.Stat) Calculator {
		created++
		return NewCalculator(nil)
	}
	e := &Effect{}
	owner := ModOwnerEffect(e)
	e.Funcs = []Mod{
		{Stat: stat.MagicDefence, Owner: owner},
		{Stat: stat.PowerAttack, Owner: owner},
		{Stat: stat.MagicDefence, Owner: owner},
	}
	other := ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1})
	ss.Attach(append(slices.Clone(e.Funcs), Mod{Stat: stat.PowerAttack, Owner: other}), newCalc)
	if created != 2 {
		t.Fatalf("created %d slots, want 2", created)
	}
	if got := ss.Get(stat.PowerAttack, newCalc); got != ss.Lookup(stat.PowerAttack) || created != 2 {
		t.Fatalf("Get on an existing slot created another (created %d)", created)
	}
	if !ss.Owns(owner) || !ss.Owns(other) {
		t.Fatal("Owns = false for an attached owner")
	}
	want := []stat.Stat{stat.PowerAttack, stat.MagicDefence, stat.MagicDefence}
	slices.Sort(want)
	if got := ss.RemoveOwner(owner); !slices.Equal(got, want) {
		t.Fatalf("RemoveOwner = %v, want %v", got, want)
	}
	if ss.Owns(owner) || !ss.Owns(other) {
		t.Fatal("RemoveOwner dropped the wrong owner's Mods")
	}
	if got := ss.RemoveOwner(owner); got != nil {
		t.Fatalf("second RemoveOwner = %v, want none", got)
	}
}
