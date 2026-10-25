package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The fixture monster of level 1 the scenarios of the second individual
// behavior batch's seventh PR spawn (#187 PR 7), and its skills.
const (
	ind2gSaintID     = int32(21534) // WarriorSaintTransform
	ind2gBlastSkill  = modelskill.ID(9671)
	ind2gDispelSkill = modelskill.ID(9672)
)

// bootIndividual2g boots WarriorSaintTransform after the Warrior chain
// beside templates, every draw 0 until a scenario queues others, every NPC
// knowing the given skills, and the director script the shared helpers
// send through.
func bootIndividual2g(t *testing.T, skills []modelskill.Definition, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	const saint = "script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorSaintTransform"
	const director = "test.Ind1Director"
	catalog[saint] = warrior.SaintTransform
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: saint}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(skills)),
		gameservertest.WithScriptSkills(skills...),
	}
	w := bootRaid(t, raidBoot{opts: opts}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind2gSaint boots a world with the saint's template, which lists its
// blast and its dispel, each costing mp, and returns it.
func ind2gSaint(t *testing.T, blastMP, dispelMP int) *ind1World {
	t.Helper()
	blast := ind1Skill(ind2gBlastSkill, 0, true)
	blast.MPConsume = blastMP
	dispel := ind1Skill(ind2gDispelSkill, 2000, false)
	dispel.MPConsume = dispelMP
	tmpl := ind2fTemplate(ind2gSaintID, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfRangeDDMagic: ind2gBlastSkill, npc.SkillTypeDispell: ind2gDispelSkill,
	})
	return bootIndividual2g(t, []modelskill.Definition{blast, dispel}, tmpl)
}

// TestWarriorSaintTransformDispelsOnce ports WarriorSaintTransform's
// created and attacked hooks for a character in no party, with hits of no
// damage. Created, i_ai0 is 0. Hit above 90 percent of its HP on a draw
// under 90, it wants its dispel on the character, weight 1000000, and sets
// i_ai0 to 1; hit strictly between 40 and 50 percent on a draw under 80,
// it does the same and sets i_ai0 to 2. Anywhere else, or on a higher
// draw, it wants nothing and i_ai0 stays 0.
func TestWarriorSaintTransformDispelsOnce(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		share  float64
		draw   int
		dispel bool
		ai0    int32
	}{
		{"unhurt", 1, 89, true, 1},
		{"unhurt draw 90", 1, 90, false, 0},
		{"89 percent", 0.89, 0, false, 0},
		{"below half", 0.45, 79, true, 2},
		{"below half draw 80", 0.45, 80, false, 0},
		{"half", 0.5, 0, false, 0},
		{"39 percent", 0.39, 0, false, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := ind2gSaint(t, 0, 0)
			n := w.spawn(t, ind2gSaintID, 1500, 0)
			if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
				t.Fatalf("created i_ai0 = %d, want 0", got)
			}
			w.setHPShare(t, n, row.share)
			w.draws.set(row.draw)
			w.zeroHit(t, n)
			want := map[modelskill.ID]ind1Cast{}
			if row.dispel {
				want[ind2gDispelSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := ind1Slot(n, script.SlotIntAI0); got != row.ai0 {
				t.Fatalf("i_ai0 after the hit = %d, want %d", got, row.ai0)
			}
		})
	}
	// The first try sets i_ai0 to 1, so a hit below half no longer tries:
	// i_ai0 would turn 2.
	t.Run("only one try", func(t *testing.T) {
		t.Parallel()
		w := ind2gSaint(t, 0, 0)
		n := w.spawn(t, ind2gSaintID, 1500, 0)
		w.zeroHit(t, n)
		w.setHPShare(t, n, 0.45)
		w.zeroHit(t, n)
		if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
			t.Fatalf("i_ai0 after the second hit = %d, want 1", got)
		}
	})
}

// TestWarriorSaintTransformDispelsWhomItHatesMost ports the dispel's
// target: the creature it hates most, the attacker only when it hates no
// one. A second character's hit on a draw of 90 adds hate but no dispel;
// the character's next hit then dispels that second character.
func TestWarriorSaintTransformDispelsWhomItHatesMost(t *testing.T) {
	t.Parallel()
	w := ind2gSaint(t, 0, 0)
	other := w.joinCharacter(t, "other", "Other")
	n := w.spawn(t, ind2gSaintID, 1500, 0)
	w.draws.set(90)
	w.hitBy(t, n, other, 1)
	w.wantCasts(t, "after the second character's hit", n, map[modelskill.ID]ind1Cast{})
	w.draws.set(0)
	w.zeroHit(t, n)
	w.wantCasts(t, "after the character's hit", n, map[modelskill.ID]ind1Cast{ind2gDispelSkill: {other.ObjectID(), 1000000}})
}

// TestWarriorSaintTransformAttacksWhenItCannotPay ports the fallback: with
// a dispel costing more MP than it has, the try wants to attack the
// character with weight 1000, which adds 1000 to its hate of the character
// on top of the hate its parent adds, and still sets i_ai0 to 1. A twin
// hit the same way on a draw of 90 shows the parent's share.
func TestWarriorSaintTransformAttacksWhenItCannotPay(t *testing.T) {
	t.Parallel()
	w := ind2gSaint(t, 0, 100000)
	plain := w.spawn(t, ind2gSaintID, 1500, 0)
	// Out of each other's clan range, so neither hit calls the other.
	n := w.spawn(t, ind2gSaintID, 1500, 1200)
	w.draws.set(90)
	w.hit(t, plain, 10)
	w.draws.set(0)
	w.hit(t, n, 10)
	w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{})
	if got, want := n.AggroHate(w.player), plain.AggroHate(w.player)+1000; got != want {
		t.Fatalf("hate of the character = %v, want %v", got, want)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the hit = %d, want 1", got)
	}
}

// TestWarriorSaintTransformBlastsAParty ports the party branch, with hits
// of no damage while unhurt. A character in a party, on a first draw under
// 33, makes it want its self range DD magic on itself, weight 1000000,
// before the dispel try. When it cannot pay for the blast it sets i_ai0 to
// 1 instead, which skips the dispel try. On a first draw of 33 it does not
// blast and the next draw still decides the dispel.
func TestWarriorSaintTransformBlastsAParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		blastMP int
		draws   []int
		blast   bool
		dispel  bool
	}{
		{"blast", 0, []int{32, 89}, true, true},
		{"cannot pay", 100000, []int{0}, false, false},
		{"draw 33", 0, []int{33, 89}, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := ind2gSaint(t, row.blastMP, 0)
			w.joinMate(t)
			n := w.spawn(t, ind2gSaintID, 1500, 0)
			w.draws.set(99, row.draws...)
			w.zeroHit(t, n)
			want := map[modelskill.ID]ind1Cast{}
			if row.blast {
				want[ind2gBlastSkill] = ind1Cast{n.ObjectID(), 1000000}
			}
			if row.dispel {
				want[ind2gDispelSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
				t.Fatalf("i_ai0 after the hit = %d, want 1", got)
			}
		})
	}
}
