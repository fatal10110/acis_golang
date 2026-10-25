package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestFrintezzaAwakens runs the tomb's entry from a sealed spawn, every
// draw 0. Arg1 1 sets his db value 1 and tells his follower 1; arg1 2 sets
// 2, tells the follower 2 and runs maker event 1000 on frintessa_2515_m01.
// FrintezzaWaitTime (10 minutes) later he tells himself and the follower 3:
// db value 3. 4 s later he leaves his organ dummy at the hall; 4 s after
// that he jumps to his hall facing 16384 and closes every door; 8 s later
// he wants social 1 (7000 ms, 1000); 8.5 s later he runs maker event 1001
// on the first evil spirit maker, 0.5 s later on the second, and 0.5 s
// later wants social 3 (6600 ms, 1000); 7.6 s later he plays the stage's
// song, the plain music on five missed draws; 13.5 s later he wakes his
// follower (arg1 4) at db value 4, and a second later plays again. Arg1 0
// then seals the tomb at db value 0, sends him home facing 16384 and runs
// maker event 1001 on frintessa_2515_m01, which Shutdown saves.
func TestFrintezzaAwakens(t *testing.T) {
	t.Parallel()
	w := bootFrintezza(t, nil)
	f := w.frintezza(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	w.makers.take()
	w.heard.take()
	heard := func(when string, arg1 int32) {
		t.Helper()
		if got, want := w.heard.take(), []string{fmt.Sprintf("29996 event=%d arg1=%d arg2=0", id, arg1)}; !slices.Equal(got, want) {
			t.Fatalf("heard %s = %q, want %q", when, got, want)
		}
	}
	makers := func(when string, want ...string) {
		t.Helper()
		if got := w.makers.take(); !slices.Equal(got, want) {
			t.Fatalf("maker events %s = %q, want %q", when, got, want)
		}
	}

	w.send(t, f, 1, 1)
	if got := n.DBValue(); got != 1 {
		t.Fatalf("db value after arg1 1 = %d, want 1", got)
	}
	heard("on arg1 1", 1)
	w.send(t, f, 1, 2)
	if got := n.DBValue(); got != 2 {
		t.Fatalf("db value after arg1 2 = %d, want 2", got)
	}
	heard("on arg1 2", 2)
	makers("on arg1 2", "frintessa_2515_m01 1000")

	w.srv.Advance(t, 10*time.Minute-time.Millisecond)
	if got := w.heard.take(); len(got) != 0 || n.DBValue() != 2 {
		t.Fatalf("before the wait: heard %q at db value %d, want nothing at 2", got, n.DBValue())
	}
	w.srv.Advance(t, time.Millisecond)
	heard("after the wait", 3)
	if got := n.DBValue(); got != 3 {
		t.Fatalf("db value after the wait = %d, want 3", got)
	}

	w.srv.Advance(t, 4*time.Second-time.Millisecond)
	if got := w.liveNPCs(fzOrganID); len(got) != 0 {
		t.Fatalf("organ dummies before 4 s = %d, want none", len(got))
	}
	w.srv.Advance(t, time.Millisecond)
	if organs := w.liveNPCs(fzOrganID); len(organs) != 1 || frAt(organs[0]) != [4]int{174239, -89809, -5016, 16384} {
		t.Fatalf("organ dummies at 4 s = %d, want one at the hall facing 16384", len(organs))
	}
	w.srv.Advance(t, 4*time.Second-time.Millisecond)
	w.wantDoors(t, "before the jump", fzSealed)
	w.srv.Advance(t, time.Millisecond)
	if got, want := frAt(f), [4]int{174239, -89808, -5016, 16384}; got != want {
		t.Fatalf("Frintezza at %v, want his hall %v", got, want)
	}
	w.wantDoors(t, "after the jump", fzShut)

	w.srv.Advance(t, 8*time.Second-time.Millisecond)
	if got := fzSocials(f); len(got) != 0 {
		t.Fatalf("socials before 8 s = %q, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got, want := fzSocials(f), []string{"1/7000=1000"}; !slices.Equal(got, want) {
		t.Fatalf("socials at 8 s = %q, want %q", got, want)
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.srv.Advance(t, 8500*time.Millisecond)
	makers("8.5 s later", "frintessa_evilate_maker1 1001")
	w.srv.Advance(t, 500*time.Millisecond)
	makers("0.5 s later", "frintessa_evilate_maker2 1001")
	w.srv.Advance(t, 500*time.Millisecond)
	if got, want := fzSocials(f), []string{"3/6600=1000"}; !slices.Equal(got, want) {
		t.Fatalf("socials 0.5 s later = %q, want %q", got, want)
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.srv.Advance(t, 7600*time.Millisecond-time.Millisecond)
	if got := frCastDesires(f); len(got) != 0 {
		t.Fatalf("cast desires before the song = %q, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got, want := frCastDesires(f), []string{fzSong(f, "5006-1")}; !slices.Equal(got, want) {
		t.Fatalf("cast desires at the song = %q, want %q", got, want)
	}

	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.srv.Advance(t, 13500*time.Millisecond-time.Millisecond)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard before the follower's call = %q, want nothing", got)
	}
	w.srv.Advance(t, time.Millisecond)
	heard("at the follower's call", 4)
	if got := n.DBValue(); got != 4 {
		t.Fatalf("db value at the follower's call = %d, want 4", got)
	}
	w.srv.Advance(t, time.Second)
	if got, want := frCastDesires(f), []string{fzSong(f, "5006-1")}; !slices.Equal(got, want) {
		t.Fatalf("cast desires a second later = %q, want %q", got, want)
	}

	w.send(t, f, 1, 0)
	w.wantDoors(t, "after arg1 0", fzSealed)
	if got := n.DBValue(); got != 0 {
		t.Fatalf("db value after arg1 0 = %d, want 0", got)
	}
	if got, want := frAt(f), [4]int{-105200, -253104, -15264, 16384}; got != want {
		t.Fatalf("Frintezza at %v after arg1 0, want home %v", got, want)
	}
	makers("on arg1 0", "frintessa_2515_m01 1001")
	w.srv.Shutdown(t)
	if got := fzRowValue(t, w.srv); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestFrintezzaHearsHisFollowers runs the script-event hook on the stages
// his follower and its transform report. From the follower (key 5, its
// object id as the event id) arg1 5 and 6 set the db value and i_ai0 and
// clear i_ai1, and 30000 and 30010 set i_ai1; its arg1 7 does nothing.
// From anyone else, arg1 7 sets the db value 7, i_ai0 7, clears i_ai1,
// takes the game tick in i_quest2 and wants social 3 (100 ms, 50) a second
// later, a timer that clears i_ai1 again; 30021 to 30025 set i_ai1, and
// 30000 does nothing. Arg1 0 with the transform's key gone does nothing.
func TestFrintezzaHearsHisFollowers(t *testing.T) {
	t.Parallel()
	w := bootFrintezza(t, nil)
	f := w.frintezza(t)
	n := script.NPCOf(f)
	follower := w.listener(t, fzFollowerID).ObjectID()
	transform := w.listener(t, fzTransformID).ObjectID()
	want := func(when string, value int, ai0, ai1 int32) {
		t.Helper()
		if n.DBValue() != value || n.Int(script.SlotIntAI0) != ai0 || n.Int(script.SlotIntAI1) != ai1 {
			t.Fatalf("%s: db value %d i_ai0 %d i_ai1 %d, want %d %d %d", when, n.DBValue(), n.Int(script.SlotIntAI0), n.Int(script.SlotIntAI1), value, ai0, ai1)
		}
	}
	w.send(t, f, follower, 30000)
	want("follower 30000", 0, 0, 30000)
	w.send(t, f, follower, 5)
	want("follower 5", 5, 5, 0)
	w.send(t, f, follower, 6)
	want("follower 6", 6, 6, 0)
	w.send(t, f, follower, 30010)
	want("follower 30010", 6, 6, 30010)
	w.send(t, f, follower, 7)
	want("follower 7", 6, 6, 30010)
	w.send(t, f, transform, 30000)
	want("transform 30000", 6, 6, 30010)
	w.send(t, f, transform, 30024)
	want("transform 30024", 6, 6, 30024)

	var before, after int32
	w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) { before = s.CurrentTick() })
	w.send(t, f, transform, 7)
	w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) { after = s.CurrentTick() })
	want("transform 7", 7, 7, 0)
	if got := n.Int(npc.IntQuest2); got < before || got > after {
		t.Fatalf("i_quest2 after transform 7 = %d, want the game tick %d to %d", got, before, after)
	}
	for _, arg1 := range []int32{30021, 30022, 30023, 30025} {
		w.send(t, f, transform, arg1)
		want(fmt.Sprintf("transform %d", arg1), 7, 7, arg1)
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.srv.Advance(t, time.Second-time.Millisecond)
	if got := fzSocials(f); len(got) != 0 {
		t.Fatalf("socials before a second = %q, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got, want := fzSocials(f), []string{"3/100=50"}; !slices.Equal(got, want) {
		t.Fatalf("socials a second later = %q, want %q", got, want)
	}
	want("after timer 5000", 7, 7, 0)

	w.makers.take()
	w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) { s.GlobalMemo().Remove("6") })
	w.send(t, f, 1, 0)
	want("arg1 0 with no transform", 7, 7, 0)
	w.wantDoors(t, "arg1 0 with no transform", fzSealed)
	if got := w.makers.take(); len(got) != 0 {
		t.Fatalf("maker events on arg1 0 with no transform = %q, want none", got)
	}
	if x, _, _ := f.Position(); x != 200 {
		t.Fatalf("Frintezza moved on arg1 0 with no transform: at x %d", x)
	}
}

// bootFrintezzaFighting boots Frintezza restored at stage 5 beside the
// character, past his first timer 3000, with no desire queued and every
// draw 9999.
func bootFrintezzaFighting(t *testing.T) (*frWorld, *npc.Hostile) {
	t.Helper()
	w := bootFrintezza(t, fzLiveRow(5))
	f := w.frintezza(t)
	w.draws.set(9999)
	w.srv.Advance(t, time.Second)
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.srv.ReadQueued(t, w.srv.Client)
	return w, f
}

// fzHit lands a hit of 1 on f by the character with def.
func (w *frWorld) fzHit(t *testing.T, f *npc.Hostile, def modelskill.Definition) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		f.ReduceHP(1, w.player, def)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// TestFrintezzaIsSilenced runs the attacked hook at stage 5: a hit with
// skill 2234 sets i_ai4 1, drops every desire and, 30 s later, his timer
// 3000 clears i_ai4 and plays the stage's fight song, the plain music
// (5006 level 1, screen text 1000527) on draws of 9999. A hit with no
// skill does nothing.
func TestFrintezzaIsSilenced(t *testing.T) {
	t.Parallel()
	w, f := bootFrintezzaFighting(t)
	n := script.NPCOf(f)
	w.onQueue(t, f, func() { n.AddDoNothingDesire(60, 50) })
	w.fzHit(t, f, modelskill.Definition{})
	if got := n.Int(script.SlotIntAI4); got != 0 || len(f.AI().Desires().Snapshot()) != 1 {
		t.Fatalf("after a plain hit: i_ai4 %d with %d desires, want 0 with 1", got, len(f.AI().Desires().Snapshot()))
	}
	w.fzHit(t, f, modelskill.Definition{ID: 2234, Level: 1})
	if got := n.Int(script.SlotIntAI4); got != 1 || len(f.AI().Desires().Snapshot()) != 0 {
		t.Fatalf("after a 2234 hit: i_ai4 %d with %d desires, want 1 with none", got, len(f.AI().Desires().Snapshot()))
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 30*time.Second-time.Millisecond)
	if got := n.Int(script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 before 30 s = %d, want 1", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := n.Int(script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 at 30 s = %d, want 0", got)
	}
	if got, want := frCastDesires(f), []string{fzSong(f, "5006-1")}; !slices.Equal(got, want) {
		t.Fatalf("cast desires at 30 s = %q, want %q", got, want)
	}
	if got, want := screenTexts(w.srv.ReadQueued(t, w.srv.Client)), []string{fzText(t, 1000527)}; !slices.Equal(got, want) {
		t.Fatalf("screen texts at 30 s = %q, want %q", got, want)
	}
}

// TestFrintezzaSingsWithNothingToDo runs the no-desire hook: at stage 5
// with i_ai4 0, nothing to do plays the stage's fight song, the song of
// power (5007 level 3, 1000523) on draws of 2 then 1999; at i_ai4 1 it
// plays nothing. At stage 7 his transform's last report picks the odds:
// at 30022, within 15 minutes of the transform's start, a draw of 0 is the
// healing song (5007 level 1, 1000522); past 15 minutes the healing odds
// are 0, and draws of 0, 9999, 9999, 9999, 1499 play the song of plague (5007 level
// 4, 1000526) only after the rampage's 3000 is missed. Below stage 4 he
// plays nothing.
func TestFrintezzaSingsWithNothingToDo(t *testing.T) {
	t.Parallel()
	t.Run("stage 5", func(t *testing.T) {
		t.Parallel()
		w, f := bootFrintezzaFighting(t)
		n := script.NPCOf(f)
		w.onQueue(t, f, func() { n.SetInt(script.SlotIntAI4, 1) })
		if got := screenTexts(w.ticks(t, 3)); len(got) != 0 {
			t.Fatalf("screen texts at i_ai4 1 = %q, want none", got)
		}
		w.onQueue(t, f, func() { n.SetInt(script.SlotIntAI4, 0) })
		w.draws.set(9999, 2, 1999)
		if got, want := screenTexts(w.ticks(t, 3)), []string{fzText(t, 1000523)}; !slices.Equal(got, want) {
			t.Fatalf("screen texts with nothing to do = %q, want %q", got, want)
		}
	})
	for _, tc := range []struct {
		name  string
		late  bool
		draws []int
		text  int32
	}{
		{"stage 7 within 15 minutes", false, []int{0}, 1000522},
		{"stage 7 past 15 minutes", true, []int{0, 9999, 9999, 9999, 1499}, 1000526},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootFrintezza(t, fzLiveRow(7))
			f := w.frintezza(t)
			n := script.NPCOf(f)
			transform := w.listener(t, fzTransformID).ObjectID()
			w.draws.set(9999)
			w.srv.Advance(t, time.Second)
			w.send(t, f, transform, 7)
			w.srv.Advance(t, time.Second)
			w.send(t, f, transform, 30022)
			if tc.late {
				w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) {
					n.SetInt(npc.IntQuest2, s.CurrentTick()-15*60-1)
				})
			}
			w.onQueue(t, f, func() { f.AI().Desires().Clear() })
			w.srv.ReadQueued(t, w.srv.Client)
			w.draws.set(9999, tc.draws...)
			if got, want := screenTexts(w.ticks(t, 3)), []string{fzText(t, tc.text)}; !slices.Equal(got, want) {
				t.Fatalf("screen texts = %q, want %q", got, want)
			}
		})
	}
	t.Run("stage 3", func(t *testing.T) {
		t.Parallel()
		w := bootFrintezza(t, fzLiveRow(3))
		f := w.frintezza(t)
		w.srv.Advance(t, time.Second)
		w.onQueue(t, f, func() { f.AI().Desires().Clear() })
		w.srv.ReadQueued(t, w.srv.Client)
		if got := screenTexts(w.ticks(t, 3)); len(got) != 0 {
			t.Fatalf("screen texts below stage 4 = %q, want none", got)
		}
	})
}

// castThrough queues f's cast of ref on itself, runs the AI until the cast
// is under way, calls during on f's queue, and runs the clock until a
// screen text shows; it returns the frames seen.
func (w *frWorld) castThrough(t *testing.T, f *npc.Hostile, ref modelskill.Ref, during func()) [][]byte {
	t.Helper()
	n := script.NPCOf(f)
	w.onQueue(t, f, func() { n.AddCastDesireUnchecked(n, ref, 10000) })
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the cast", func() bool {
		frames = append(frames, w.ticks(t, 1)...)
		return f.CastingNow()
	})
	if during != nil {
		w.onQueue(t, f, during)
	}
	w.srv.AdvanceUntil(t, "a screen text", func() bool {
		frames = append(frames, w.ticks(t, 1)...)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return len(screenTexts(frames)) > 0
	})
	return frames
}

// TestFrintezzaSingsOn runs the skill-finished hook at stage 5. A finished
// song plays the stage's fight song, the plain music on draws of 9999. A
// song that ends while i_ai4 is set plays the rampage (5007 level 2,
// 1000524) and clears i_ai4. An aborted song sets i_ai4 1, drops every
// desire and, 30 s later, his timer 3001 clears i_ai4 and plays the
// stage's fight song.
func TestFrintezzaSingsOn(t *testing.T) {
	t.Parallel()
	power := modelskill.Ref{ID: 5007, Level: 3}
	t.Run("finished", func(t *testing.T) {
		t.Parallel()
		w, f := bootFrintezzaFighting(t)
		frames := w.castThrough(t, f, power, nil)
		if got, want := frCasts(frames, f), []string{fmt.Sprintf("5007-3>%d", f.ObjectID())}; len(got) == 0 || got[0] != want[0] {
			t.Fatalf("casts = %q, want %q first", got, want)
		}
		if got := screenTexts(frames); got[0] != fzText(t, 1000527) {
			t.Fatalf("screen texts = %q, want the plain music first", got)
		}
	})
	t.Run("silenced", func(t *testing.T) {
		t.Parallel()
		w, f := bootFrintezzaFighting(t)
		n := script.NPCOf(f)
		frames := w.castThrough(t, f, power, func() { n.SetInt(script.SlotIntAI4, 1) })
		if got := screenTexts(frames); got[0] != fzText(t, 1000524) {
			t.Fatalf("screen texts = %q, want the rampage first", got)
		}
		if got := n.Int(script.SlotIntAI4); got != 0 {
			t.Fatalf("i_ai4 after the song = %d, want 0", got)
		}
	})
	t.Run("aborted", func(t *testing.T) {
		t.Parallel()
		w, f := bootFrintezzaFighting(t)
		n := script.NPCOf(f)
		w.onQueue(t, f, func() { n.AddCastDesireUnchecked(n, power, 10000) })
		w.srv.AdvanceUntil(t, "the cast", func() bool {
			w.ticks(t, 1)
			return f.CastingNow()
		})
		w.onQueue(t, f, func() { n.AddDoNothingDesire(60, 50); f.StopCast() })
		w.srv.Settle(t)
		if got := n.Int(script.SlotIntAI4); got != 1 || len(f.AI().Desires().Snapshot()) != 0 {
			t.Fatalf("after the abort: i_ai4 %d with %d desires, want 1 with none", got, len(f.AI().Desires().Snapshot()))
		}
		w.srv.ReadQueued(t, w.srv.Client)
		w.srv.Advance(t, 30*time.Second-time.Millisecond)
		if got := n.Int(script.SlotIntAI4); got != 1 {
			t.Fatalf("i_ai4 before 30 s = %d, want 1", got)
		}
		w.srv.Advance(t, time.Millisecond)
		if got := n.Int(script.SlotIntAI4); got != 0 {
			t.Fatalf("i_ai4 at 30 s = %d, want 0", got)
		}
		if got, want := screenTexts(w.srv.ReadQueued(t, w.srv.Client)), []string{fzText(t, 1000527)}; !slices.Equal(got, want) {
			t.Fatalf("screen texts at 30 s = %q, want %q", got, want)
		}
	})
}

// TestFrintezzaPlaysHisLastSong runs arg1 50000 at stage 5: 0.5 s later he
// drops every desire and wants social 4 (7000 ms, 10000000); 8 s later he
// drops every desire again and plays the rampage (5007 level 2, screen
// text 1000524) on draws of 9999.
func TestFrintezzaPlaysHisLastSong(t *testing.T) {
	t.Parallel()
	w, f := bootFrintezzaFighting(t)
	n := script.NPCOf(f)
	w.send(t, f, 1, 50000)
	w.onQueue(t, f, func() { n.AddDoNothingDesire(60, 50) })
	w.srv.Advance(t, 500*time.Millisecond)
	if got := f.AI().Desires().Snapshot(); len(got) != 1 {
		t.Fatalf("desires at 0.5 s = %d, want the social alone", len(got))
	}
	if got, want := fzSocials(f), []string{"4/7000=1e+07"}; !slices.Equal(got, want) {
		t.Fatalf("socials at 0.5 s = %q, want %q", got, want)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 8*time.Second-time.Millisecond)
	if got := frCastDesires(f); len(got) != 0 {
		t.Fatalf("cast desires before 8 s = %q, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := fzSocials(f); len(got) != 0 {
		t.Fatalf("socials at 8 s = %q, want none", got)
	}
	if got, want := frCastDesires(f), []string{fzSong(f, "5007-2")}; !slices.Equal(got, want) {
		t.Fatalf("cast desires at 8 s = %q, want %q", got, want)
	}
	if got, want := screenTexts(w.srv.ReadQueued(t, w.srv.Client)), []string{fzText(t, 1000524)}; !slices.Equal(got, want) {
		t.Fatalf("screen texts at 8 s = %q, want %q", got, want)
	}
}

// TestFrintezzaDies runs arg1 8 at stage 5: 7.5 s later he dies by his own
// hand. Dying (the hook runs 3 s after the death), he gives up memo key 4, seals the tomb, sets his db value 0
// and runs maker event 1001 on frintessa_teleportcuve.
func TestFrintezzaDies(t *testing.T) {
	t.Parallel()
	w, f := bootFrintezzaFighting(t)
	n := script.NPCOf(f)
	w.makers.take()
	w.send(t, f, 1, 8)
	w.srv.Advance(t, 7500*time.Millisecond-time.Millisecond)
	if f.Dead() {
		t.Fatal("Frintezza died before 7.5 s")
	}
	w.srv.Advance(t, time.Millisecond)
	if !f.Dead() {
		t.Fatal("Frintezza alive at 7.5 s")
	}
	// The dying hooks run 3 s after the death.
	w.srv.Advance(t, 3*time.Second-time.Millisecond)
	if got := w.memo(t, "4"); got != int(f.ObjectID()) {
		t.Fatalf("memo 4 before the dying hook = %d, want Frintezza %d", got, f.ObjectID())
	}
	w.srv.Advance(t, time.Millisecond)
	if got := w.memo(t, "4"); got != 0 {
		t.Fatalf("memo 4 after the death = %d, want none", got)
	}
	w.wantDoors(t, "after the death", fzSealed)
	if got := n.DBValue(); got != 0 {
		t.Fatalf("db value after the death = %d, want 0", got)
	}
	if got, want := w.makers.take(), []string{"frintessa_teleportcuve 1001"}; !slices.Equal(got, want) {
		t.Fatalf("maker events after the death = %q, want %q", got, want)
	}
}
