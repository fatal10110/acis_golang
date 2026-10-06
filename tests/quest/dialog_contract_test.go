package quest

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// q001Pages are Q001's page files the dialog goldens show.
func q001Pages(pages contractPages) map[string]string {
	return map[string]string{
		"script/quest/Q001_LettersOfLove/30048-02.htm": pages.darinStart,
		"script/quest/Q001_LettersOfLove/30048-07.htm": pages.darinStarted,
	}
}

// bootWindowRow boots a fresh player with the row's seeds beside fresh
// NPCs, Darin dx east of it.
func bootWindowRow(t *testing.T, r scriptcontract.Row, dx int) *dialogWorld {
	t.Helper()
	seeds := r.List(t, "seed")
	var opts []gameservertest.Option
	if slices.Contains(seeds, "overweight") {
		opts = append(opts, gameservertest.WithWeightLimitMultiplier(1))
	}
	w := bootDialog(t, q001Pages(loadContractPages(t)), func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, seeds)
	}, opts...)
	w.spawnGoldenNPCs(t, dx)
	return w
}

// TestQuestWindowsMatchReferenceGoldens runs the general and single quest
// window goldens: a fresh player beside fresh NPCs, with the row's quest
// states, sends the npc_ Quest command a page offered, and receives the
// reference's packets in order, leaving the quest states and the last
// quest NPC the reference leaves.
func TestQuestWindowsMatchReferenceGoldens(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"dialog.general_window", "dialog.single_window"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			scriptcontract.Run(t, table, func(t *testing.T, r scriptcontract.Row) {
				t.Parallel()
				w := bootWindowRow(t, r, 0)
				w.offerAll(t)
				got := w.bypass(t, w.expand(r.Str(t, "bypass")))
				if !slices.Equal(got, r.Lines) {
					t.Fatalf("packets:\n got %q\nwant %q", got, r.Lines)
				}
				if got, want := w.states(t), r.Str(t, "after"); got != want {
					t.Errorf("states after = %s, want %s", got, want)
				}
				if got, want := w.lastRole(t), r.Str(t, "last"); got != want {
					t.Errorf("last quest NPC = %s, want %s", got, want)
				}
			})
		})
	}
}

// TestQuestEventLinkMatchesReferenceGolden runs the quest event link
// golden: with Darin dx east of the player and the row's NPC set silently
// as the last quest NPC, the Quest link a page offered runs Q001's event
// only from strictly within 150 of a live NPC talking through Q001, and
// every refusal is silent.
func TestQuestEventLinkMatchesReferenceGolden(t *testing.T) {
	t.Parallel()
	scriptcontract.Run(t, "dialog.quest_bypass", func(t *testing.T, r scriptcontract.Row) {
		t.Parallel()
		w := bootDialog(t, q001Pages(loadContractPages(t)), nil)
		npcs := w.spawnGoldenNPCs(t, int(r.Int(t, "dx")))
		if last := r.Str(t, "last_npc"); last != "-" {
			w.onCharacter(t, func(c *player.Character) { c.SetLastQuestNPC(w.npcs[last]) })
			if r.Bool(t, "gone") {
				npcs[last].Decay(w.srv.State, nil)
				w.srv.ReadQueued(t, w.srv.Client)
			}
		}
		if r.Bool(t, "offered") {
			w.offerAll(t)
		}
		got := w.bypass(t, r.Str(t, "bypass"))
		if !slices.Equal(got, r.Lines) {
			t.Fatalf("packets:\n got %q\nwant %q", got, r.Lines)
		}
		if got, want := w.states(t), r.Str(t, "after"); got != want {
			t.Errorf("states after = %s, want %s", got, want)
		}
	})
}

// TestNpcBypassMatchesReferenceGolden runs the npc_ handler golden: one
// ActionFailed after any command whose id parses, nothing when the id
// does not parse or the bypass was not offered.
func TestNpcBypassMatchesReferenceGolden(t *testing.T) {
	t.Parallel()
	scriptcontract.Run(t, "dialog.npc_bypass", func(t *testing.T, r scriptcontract.Row) {
		t.Parallel()
		w := bootDialog(t, q001Pages(loadContractPages(t)), nil)
		w.spawnGoldenNPCs(t, int(r.Int(t, "dx")))
		if r.Bool(t, "offered") {
			w.offerAll(t)
		}
		got := w.bypass(t, w.expand(r.Str(t, "bypass")))
		if !slices.Equal(got, r.Lines) {
			t.Fatalf("packets:\n got %q\nwant %q", got, r.Lines)
		}
		if got, want := w.states(t), r.Str(t, "after"); got != want {
			t.Errorf("states after = %s, want %s", got, want)
		}
	})
}

// TestDialogRangeMatchesReferenceGoldens runs the range goldens: the
// quest event link runs, and the npc_ Quest command reaches the NPC, only
// strictly within 150 of it, centre to centre in 3D.
func TestDialogRangeMatchesReferenceGoldens(t *testing.T) {
	t.Parallel()
	t.Run("range.quest_event", func(t *testing.T) {
		t.Parallel()
		scriptcontract.Run(t, "range.quest_event", func(t *testing.T, r scriptcontract.Row) {
			t.Parallel()
			w := bootDialog(t, q001Pages(loadContractPages(t)), nil)
			w.spawnFolk(t, "darin", darinID, int(r.Int(t, "dx")), int(r.Int(t, "dy")), int(r.Int(t, "dz")))
			w.onCharacter(t, func(c *player.Character) { c.SetLastQuestNPC(w.npcs["darin"]) })
			w.offerAll(t)
			got := w.bypass(t, "Quest Q001_LettersOfLove 30048-02.htm")
			ran := len(got) > 0 && strings.HasPrefix(got[0], "S NpcHtmlMessage")
			if want := r.Bool(t, "ran"); ran != want {
				t.Fatalf("event ran = %v (%q), want %v", ran, got, want)
			}
		})
	})
	t.Run("range.interact", func(t *testing.T) {
		t.Parallel()
		scriptcontract.Run(t, "range.interact", func(t *testing.T, r scriptcontract.Row) {
			t.Parallel()
			w := bootDialog(t, q001Pages(loadContractPages(t)), nil)
			w.spawnFolk(t, "darin", darinID, int(r.Int(t, "dx")), int(r.Int(t, "dy")), int(r.Int(t, "dz")))
			w.offerAll(t)
			got := w.bypass(t, w.expand("npc_{darin}_Quest"))
			reached := len(got) > 0 && strings.HasPrefix(got[0], "S NpcHtmlMessage")
			if want := r.Bool(t, "can_interact"); reached != want {
				t.Fatalf("command reached the NPC = %v (%q), want %v", reached, got, want)
			}
		})
	})
}

// TestLastQuestNPCMatchesReferenceGolden runs the last quest NPC golden's
// steps: an interact sets it, before a first talk too; a quest's own
// window sets it; the no-quest page and the choice list leave it.
func TestLastQuestNPCMatchesReferenceGolden(t *testing.T) {
	t.Parallel()
	scriptcontract.Run(t, "dialog.last_quest_npc", func(t *testing.T, r scriptcontract.Row) {
		t.Parallel()
		w := bootDialog(t, q001Pages(loadContractPages(t)), nil)
		w.spawnGoldenNPCs(t, 20)
		w.spawnFolk(t, "black_judge", judgeID, 0, -10, 0)
		for _, step := range strings.Split(r.Str(t, "steps"), ";") {
			step = strings.TrimSpace(step)
			if i := strings.Index(step, " ("); i >= 0 {
				step = step[:i]
			}
			switch {
			case strings.HasPrefix(step, "interact "):
				w.interact(t, w.npcs[strings.TrimPrefix(step, "interact ")])
			case step == "seed Q001 started":
				seedStarted(t, w, q001)
			case strings.HasPrefix(step, "npc_"):
				w.offerAll(t)
				w.bypass(t, w.expand(step))
			default:
				t.Fatalf("unknown step %q", step)
			}
		}
		if got, want := w.lastRole(t), r.Str(t, "last"); got != want {
			t.Fatalf("last quest NPC = %s, want %s", got, want)
		}
	})
}

// seedStarted starts the player's state in quest name at condition 1, as
// a script would, and discards what that sends.
func seedStarted(t *testing.T, w *dialogWorld, name string) {
	t.Helper()
	w.srv.RunQuest(t, w.player, name, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.State(c, sc)
		if st == nil {
			st = q.NewState(c, sc)
		}
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
	})
	w.srv.ReadQueued(t, w.srv.Client)
}
