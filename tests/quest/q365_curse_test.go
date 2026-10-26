package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q365"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The curse of Collob's rare finds in Devil's Legacy
// (Q365_DevilsLegacy onAdvEvent, Creature.getFirstEffect,
// L2Skill.getEffects): a rare find casts Poison of Death (4082) on the
// character unless it already carries it; a common find casts nothing. The
// scenario runner holds no skill table for the scripts, so a scenario cannot
// play the rare finds; the scenarios play the common ones.
const (
	q365Name   = "Q365_DevilsLegacy"
	q365Path   = "quest.Q365_DevilsLegacy"
	q365Collob = 30092
	q365Chest  = 5873
	q365Curse  = 4082
)

func TestQ365RareFindsCurseOnce(t *testing.T) {
	t.Parallel()
	curse, ok := shippedSkillDefs(t).Definition(skill.Ref{ID: q365Curse, Level: 1})
	if !ok {
		t.Fatal("the datapack holds no skill 4082")
	}
	for _, tc := range []struct {
		name   string
		draws  []int
		cursed bool
	}{
		{"common find", []int{79, 0}, false},
		{"rare find", []int{80, 0}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			draws.queue(tc.draws...)
			draws.queue(tc.draws...)
			w := bootSeamQuest(t, 39, chatPages(questPages(q365Name, map[string][]string{
				"30092-01.htm": {"30092-05.htm"}, "30092-05.htm": nil, "30092-06.htm": nil,
			}), q365Collob), func(srv *gameservertest.Server, objID int32) {
				seedJournal(t, srv, objID, []string{q365Name + ":STARTED"})
				srv.GiveItem(t, objID, q365Chest, 2)
				srv.GiveItem(t, objID, 57, 1200)
			},
				x1Items([]int32{q365Chest, 951, 955}),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", q365Collob)})),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(
					map[int32]script.NPCKind{q365Collob: script.KindFolk},
					[]script.Listing{{Path: q365Path}},
					script.Catalog{q365Path: q365.New},
				),
				gameservertest.WithScriptRand(draws.rand),
				gameservertest.WithScriptSkills(curse),
			)
			w.roles[w.player] = "player"
			w.spawnFolk(t, "collob", q365Collob, 40, 0, 0)

			open := func(page string) {
				t.Helper()
				if got := w.talk(t, "collob"); !shows(got, "30092-01.htm") {
					t.Fatalf("Collob's window: got %q, want 30092-01.htm", got)
				}
				if got := w.send(t, encodeBypass("Quest "+q365Name+" 30092-05.htm")); !shows(got, page) {
					t.Fatalf("Collob's event: got %q, want %s", got, page)
				}
			}
			page := "30092-05.htm"
			if tc.cursed {
				page = "30092-06.htm"
			}
			open(page)
			first := q365Effects(t, w)
			if got := len(first) > 0; got != tc.cursed {
				t.Fatalf("the character carries %d effects of the curse after the first opening, want cursed %v", len(first), tc.cursed)
			}
			open(page)
			second := q365Effects(t, w)
			if len(second) != len(first) {
				t.Fatalf("%d effects after the second opening, want the same %d", len(second), len(first))
			}
			for i := range first {
				if first[i] != second[i] {
					t.Fatalf("the second opening cast the curse again: effect %d was replaced", i)
				}
			}
		})
	}
}

// q365Effects returns the effects of the curse the character carries.
func q365Effects(t *testing.T, w *dialogWorld) []*effect.Effect {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatalf("player %d is not in the world", w.player)
	}
	holder, ok := obj.(interface{ EffectList() *effect.List })
	if !ok {
		t.Fatalf("player %T holds no effect list", obj)
	}
	var out []*effect.Effect
	for _, e := range holder.EffectList().All() {
		if e.Skill.ID == q365Curse {
			out = append(out, e)
		}
	}
	return out
}
